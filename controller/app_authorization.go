package controller

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// App authorization lets a native app (for example a desktop client) obtain an
// API token without handling the user's account credentials. It follows the
// OAuth 2.0 authorization code flow for native apps (RFC 8252) with mandatory
// PKCE (RFC 7636, S256):
//
//  1. The app opens /app-auth in the system browser with a loopback
//     redirect_uri, a code_challenge and a state value.
//  2. The signed-in user approves on the consent page, which calls
//     POST /api/app-auth/authorize. A new API token is created and a one-time
//     authorization code bound to it is issued.
//  3. The browser is redirected to the loopback redirect_uri with the code.
//  4. The app calls POST /api/app-auth/token with the code and the
//     code_verifier and receives the token key.
//
// Codes are single use, expire after appAuthorizationCodeTTL, are stored only
// as HMAC digests in auth_flows, and stay bound to the approving login session.

const (
	appAuthorizationCodeTTL       = 5 * time.Minute
	appAuthorizationMaxClientName = 64
	appAuthorizationMaxState      = 512
	appAuthorizationMaxRedirect   = 512
)

type appAuthorizationRequest struct {
	ClientName          string       `json:"client_name"`
	RedirectURI         string       `json:"redirect_uri"`
	CodeChallenge       string       `json:"code_challenge"`
	CodeChallengeMethod string       `json:"code_challenge_method"`
	State               string       `json:"state"`
	Token               tokenRequest `json:"token"`
}

type appAuthorizationExchangeRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	RedirectURI  string `json:"redirect_uri"`
}

// appAuthorizationGrant is the server-side state behind an authorization code.
type appAuthorizationGrant struct {
	TokenID       int    `json:"token_id"`
	ClientName    string `json:"client_name"`
	RedirectURI   string `json:"redirect_uri"`
	CodeChallenge string `json:"code_challenge"`
}

// normalizeAppClientName returns the trimmed display name of the requesting app,
// or false when it is empty, too long, or contains control characters.
func normalizeAppClientName(value string) (string, bool) {
	name := strings.TrimSpace(value)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > appAuthorizationMaxClientName {
		return "", false
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return "", false
		}
	}
	return name, true
}

// isLoopbackRedirectURI accepts only RFC 8252 loopback redirects:
// http://127.0.0.1:<port>/<path> or http://[::1]:<port>/<path>, without user
// info, query or fragment. Codes can therefore only be delivered to a process
// on the user's own machine.
func isLoopbackRedirectURI(value string) bool {
	if value == "" || len(value) > appAuthorizationMaxRedirect {
		return false
	}
	redirect, err := url.Parse(value)
	if err != nil || redirect.Scheme != "http" || redirect.User != nil || redirect.Opaque != "" ||
		redirect.RawQuery != "" || redirect.ForceQuery || redirect.Fragment != "" || strings.Contains(value, "#") {
		return false
	}
	host, port, err := net.SplitHostPort(redirect.Host)
	if err != nil || (host != "127.0.0.1" && host != "::1") {
		return false
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 || strconv.Itoa(portNumber) != port {
		return false
	}
	return redirect.Path == "" || strings.HasPrefix(redirect.Path, "/")
}

// isPKCEVerifier checks the RFC 7636 code_verifier syntax: 43-128 characters
// from the unreserved set.
func isPKCEVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_' || r == '~') {
			return false
		}
	}
	return true
}

func pkceS256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizeApp is called by the consent page after the signed-in user approves
// an app. It creates the requested API token and returns the loopback redirect
// URL carrying a one-time authorization code.
func AuthorizeApp(c *gin.Context) {
	setAuthNoStore(c)
	if !common.AppAuthorizationEnabled {
		common.ApiErrorI18n(c, i18n.MsgAppAuthDisabled)
		return
	}
	// Consent must come from a browser login session, not a personal access token.
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": i18n.T(c, i18n.MsgAuthNotLoggedIn)})
		return
	}
	var request appAuthorizationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	clientName, validName := normalizeAppClientName(request.ClientName)
	challenge, err := base64.RawURLEncoding.DecodeString(request.CodeChallenge)
	if !validName || !isLoopbackRedirectURI(request.RedirectURI) || request.CodeChallengeMethod != "S256" ||
		err != nil || len(challenge) != sha256.Size || len(request.State) > appAuthorizationMaxState {
		common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidRequest)
		return
	}
	params := tokenAuditParams(c)
	params["client_name"] = clientName

	token, created := createUserToken(c, request.Token)
	if !created {
		return
	}
	payload, err := common.Marshal(appAuthorizationGrant{
		TokenID:       token.Id,
		ClientName:    clientName,
		RedirectURI:   request.RedirectURI,
		CodeChallenge: request.CodeChallenge,
	})
	var code string
	if err == nil {
		code, _, err = model.CreateAuthFlow(model.AuthFlowCreate{
			Purpose:   model.AuthFlowPurposeAppAuthorization,
			UserId:    identity.UserID,
			SessionId: identity.SessionID,
			Payload:   string(payload),
			ExpiresAt: time.Now().Add(appAuthorizationCodeTTL),
		})
	}
	if err != nil {
		if deleteErr := model.DeleteTokenById(token.Id, identity.UserID); deleteErr != nil {
			common.SysError("failed to remove token after app authorization failure: " + deleteErr.Error())
		}
		writeSecurityOperationError(c, err)
		return
	}

	redirect, _ := url.Parse(request.RedirectURI)
	query := url.Values{}
	query.Set("code", code)
	if request.State != "" {
		query.Set("state", request.State)
	}
	redirect.RawQuery = query.Encode()
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	common.ApiSuccess(c, gin.H{"redirect_url": redirect.String()})
}

// ExchangeAppAuthorization redeems an authorization code for the key of the
// API token the user approved. The code is consumed on every attempt that
// names a live code, including a wrong verifier, so it cannot be retried.
func ExchangeAppAuthorization(c *gin.Context) {
	setAuthNoStore(c)
	if !common.AppAuthorizationEnabled {
		common.ApiErrorI18n(c, i18n.MsgAppAuthDisabled)
		return
	}
	var request appAuthorizationExchangeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Code == "" || !isPKCEVerifier(request.CodeVerifier) {
		common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidRequest)
		return
	}
	match := model.AuthFlowMatch{Purpose: model.AuthFlowPurposeAppAuthorization}
	flow, err := model.GetAuthFlow(request.Code, match)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidGrant)
		return
	}
	match.UserId, match.SessionId = flow.UserId, flow.SessionId
	audit := map[string]any{"success": false}
	defer func() { recordUserSecurityAudit(c, flow.UserId, "token.app_key_exchange", audit) }()

	var grant appAuthorizationGrant
	grantErr := common.UnmarshalJsonStr(flow.Payload, &grant)
	if grantErr == nil {
		audit["id"], audit["client_name"] = grant.TokenID, grant.ClientName
		expected := []byte(grant.CodeChallenge)
		if subtle.ConstantTimeCompare([]byte(pkceS256Challenge(request.CodeVerifier)), expected) != 1 ||
			subtle.ConstantTimeCompare([]byte(request.RedirectURI), []byte(grant.RedirectURI)) != 1 {
			grantErr = errors.New("app authorization verifier or redirect mismatch")
		}
	}
	if grantErr == nil {
		_, grantErr = service.ValidateSessionReference(flow.UserId, flow.SessionId)
	}
	var token model.Token
	_, err = model.ConsumeAuthFlowWithAction(request.Code, match, func(tx *gorm.DB, _ *model.AuthFlow) error {
		if grantErr != nil {
			return nil
		}
		// A deleted token still burns the code; its zero status fails below.
		err := tx.Where("id = ? AND user_id = ?", grant.TokenID, flow.UserId).First(&token).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	})
	if err != nil || grantErr != nil || token.Status != common.TokenStatusEnabled {
		common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidGrant)
		return
	}

	data := gin.H{
		"key":   token.GetFullKey(),
		"token": gin.H{"id": token.Id, "name": token.Name, "group": token.Group},
	}
	if user, err := model.GetUserById(flow.UserId, false); err == nil {
		data["user"] = gin.H{"id": user.Id, "username": user.Username, "display_name": user.DisplayName}
	}
	audit["success"] = true
	common.ApiSuccess(c, data)
}
