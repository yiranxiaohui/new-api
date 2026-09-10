// Package unipay implements the documented UniPay payout v1 protocol.
package unipay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const CreatePath = "/api/v1/payouts"
const QueryPath = "/api/v1/payouts/query"
const MaxAmountCents int64 = 100000000
const MaxBody = 64 * 1024

var Identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var KeyPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
var ErrUncertain = errors.New("Payout result is not confirmed. Funds remain frozen.")

type SceneInfo struct {
	InfoType    string `json:"info_type"`
	InfoContent string `json:"info_content"`
}
type Request struct {
	OutBizNo     string      `json:"out_biz_no"`
	AmountCents  int64       `json:"amount_cents"`
	PayeeAccount string      `json:"payee_account"`
	PayeeName    string      `json:"payee_name"`
	Title        string      `json:"title"`
	NotifyURL    string      `json:"notify_url"`
	Scene        string      `json:"transfer_scene_name"`
	SceneInfos   []SceneInfo `json:"transfer_scene_report_infos"`
}
type Result struct {
	PID         string `json:"pid,omitempty"`
	PayoutNo    string `json:"payout_no"`
	OutBizNo    string `json:"out_biz_no"`
	AmountCents int64  `json:"amount_cents"`
	Status      string `json:"status"`
	FailureCode string `json:"failure_code"`
}

func ValidText(value string, max int) bool {
	if !utf8.ValidString(value) || value == "" || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) > max {
		return false
	}
	for _, c := range value {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}

func ValidateHTTPS(raw string, base bool) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || (base && u.Path != "" && u.Path != "/") {
		return errors.New("Use an HTTPS URL without credentials, query parameters or fragments.")
	}
	return nil
}

func (r Request) Validate() error {
	if !Identifier.MatchString(r.OutBizNo) || r.AmountCents <= 0 || r.AmountCents > MaxAmountCents || !ValidText(r.PayeeAccount, 100) || !ValidText(r.PayeeName, 100) || !ValidText(r.Title, 100) || !ValidText(r.Scene, 64) || len(r.SceneInfos) > 10 {
		return errors.New("Invalid payout details.")
	}
	if err := ValidateHTTPS(r.NotifyURL, false); err != nil {
		return err
	}
	for _, s := range r.SceneInfos {
		if !ValidText(s.InfoType, 64) || !ValidText(s.InfoContent, 300) {
			return errors.New("Invalid payout scene information.")
		}
	}
	return nil
}

func KeyID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:12]
}

func Signature(key, prefix, pid, timestamp, nonce, path string, body []byte) string {
	sum := sha256.Sum256(body)
	lines := []string{prefix, pid, timestamp, nonce}
	if path != "" {
		lines = append(lines, "POST", path)
	}
	lines = append(lines, hex.EncodeToString(sum[:]))
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyNotification(headers http.Header, body []byte, pid string, keys map[string]string, now time.Time) bool {
	for _, name := range []string{"X-Unipay-Pid", "X-Unipay-Timestamp", "X-Unipay-Nonce", "X-Unipay-Signature", "X-Unipay-Key-Id"} {
		if len(headers.Values(name)) != 1 {
			return false
		}
	}
	stamp := headers.Get("X-Unipay-Timestamp")
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	nonce := headers.Get("X-Unipay-Nonce")
	key := keys[headers.Get("X-Unipay-Key-Id")]
	if err != nil || seconds < now.Unix()-300 || seconds > now.Unix()+300 || headers.Get("X-Unipay-Pid") != pid || len(nonce) < 16 || !Identifier.MatchString(nonce) || !KeyPattern.MatchString(key) || len(body) > MaxBody {
		return false
	}
	expected := Signature(key, "unipay-payout-notify-v1", pid, stamp, nonce, "", body)
	return hmac.Equal([]byte(expected), []byte(headers.Get("X-Unipay-Signature")))
}

type Client struct {
	BaseURL, PID, Key string
	HTTP              *http.Client
}

func (c Client) Call(ctx context.Context, path string, payload any) (*Result, int, error) {
	if path != CreatePath && path != QueryPath {
		return nil, 0, ErrUncertain
	}
	if err := ValidateHTTPS(c.BaseURL, true); err != nil {
		return nil, 0, err
	}
	if !ValidText(c.PID, 64) || !KeyPattern.MatchString(c.Key) {
		return nil, 0, ErrUncertain
	}
	body, err := common.Marshal(payload)
	if err != nil || len(body) > MaxBody {
		return nil, 0, ErrUncertain
	}
	nonceBytes := make([]byte, 16)
	if _, err = rand.Read(nonceBytes); err != nil {
		return nil, 0, ErrUncertain
	}
	nonce, stamp := hex.EncodeToString(nonceBytes), strconv.FormatInt(time.Now().Unix(), 10)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, ErrUncertain
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Unipay-Pid", c.PID)
	request.Header.Set("X-Unipay-Timestamp", stamp)
	request.Header.Set("X-Unipay-Nonce", nonce)
	request.Header.Set("X-Unipay-Signature", Signature(c.Key, "unipay-payout-v1", c.PID, stamp, nonce, path, body))
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	if c.HTTP != nil {
		copy := *c.HTTP
		client = &copy
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, ErrUncertain
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		return nil, response.StatusCode, fmt.Errorf("payout gateway HTTP %d; funds remain frozen", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxBody+1))
	var result Result
	if err != nil || len(raw) > MaxBody || common.Unmarshal(raw, &result) != nil {
		return nil, response.StatusCode, ErrUncertain
	}
	return &result, response.StatusCode, nil
}
