package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"math"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/unipay"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const WithdrawalConfigKey = "AffiliateWithdrawalConfigSecret"
const WithdrawalPending = "pending"
const WithdrawalProcessing = "processing"
const WithdrawalSucceeded = "succeeded"
const WithdrawalFailed = "failed"
const WithdrawalRejected = "rejected"

var ErrWithdrawalUnavailable = errors.New("Affiliate withdrawals are not configured or enabled.")
var ErrWithdrawalInvalid = errors.New("Invalid withdrawal details or amount.")
var ErrWithdrawalFunds = errors.New("Insufficient available referral rewards.")
var ErrWithdrawalState = errors.New("This withdrawal cannot be changed in its current state.")
var ErrWithdrawalMismatch = errors.New("Withdrawal details changed. Refresh and try again.")
var ErrWithdrawalStorageKey = errors.New("Configure a persistent CRYPTO_SECRET or SESSION_SECRET of at least 32 characters before enabling withdrawals.")

type WithdrawalConfig struct {
	Enabled    bool               `json:"enabled"`
	Gateway    string             `json:"gateway"`
	PID        string             `json:"pid"`
	APIKey     string             `json:"api_key"`
	NotifyURL  string             `json:"notify_url"`
	CNYPerUnit string             `json:"cny_per_unit"`
	MinCents   int64              `json:"min_cents"`
	MaxCents   int64              `json:"max_cents"`
	Scene      string             `json:"scene"`
	SceneInfos []unipay.SceneInfo `json:"scene_infos"`
}

type WithdrawalSecrets struct {
	Config WithdrawalConfig `json:"config"`
	// Keep previously used notification keys after an API key rotation.
	Keys map[string]string `json:"keys"`
}

type AffiliateWithdrawal struct {
	ID                string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	UserID            int    `json:"user_id" gorm:"index"`
	Quota             int    `json:"quota" gorm:"type:bigint"`
	AmountCents       int64  `json:"amount_cents" gorm:"type:bigint"`
	Status            string `json:"status" gorm:"type:varchar(20);index:idx_withdrawal_due,priority:1"`
	PayoutNo          string `json:"payout_no" gorm:"type:varchar(64)"`
	FailureCode       string `json:"failure_code" gorm:"type:varchar(64)"`
	RequestCiphertext string `json:"-" gorm:"type:text"`
	CreatedAt         int64  `json:"created_at"`
	CompletedAt       int64  `json:"completed_at"`
	ReviewedBy        int    `json:"reviewed_by"`
	NextAttemptAt     int64  `json:"-" gorm:"index:idx_withdrawal_due,priority:2"`
}

type WithdrawalRequest struct {
	ID           string `json:"id"`
	AmountCents  int64  `json:"amount_cents"`
	Quota        int    `json:"quota"`
	PayeeAccount string `json:"payee_account"`
	PayeeName    string `json:"payee_name"`
}

type WithdrawalSnapshot struct {
	Gateway string         `json:"gateway"`
	PID     string         `json:"pid"`
	Payout  unipay.Request `json:"payout"`
}

type WithdrawalView struct {
	AffiliateWithdrawal
	PayeeAccount string `json:"payee_account"`
	PayeeName    string `json:"payee_name"`
}

// withdrawalCipher uses a stable operator-managed secret, never the random
// per-process fallback. Ciphertext is bound to its order/config key with GCM AAD.
func withdrawalCipher() (cipher.AEAD, error) {
	secret := os.Getenv("CRYPTO_SECRET")
	if secret == "" {
		secret = os.Getenv("SESSION_SECRET")
	}
	if len(secret) < 32 {
		return nil, ErrWithdrawalStorageKey
	}
	key := sha256.Sum256([]byte("affiliate-withdrawal-v1:" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func encryptWithdrawal(value any, binding string) (string, error) {
	aead, err := withdrawalCipher()
	if err != nil {
		return "", err
	}
	body, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, body, []byte(binding))), nil
}
func decryptWithdrawal(raw, binding string, value any) error {
	aead, err := withdrawalCipher()
	if err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(data) < aead.NonceSize() {
		return ErrWithdrawalUnavailable
	}
	body, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(binding))
	if err != nil {
		return ErrWithdrawalUnavailable
	}
	return common.Unmarshal(body, value)
}

func DefaultWithdrawalConfig() WithdrawalConfig {
	return WithdrawalConfig{Gateway: "https://pay.yunnet.top", CNYPerUnit: "7", MinCents: 100, MaxCents: 10000, SceneInfos: []unipay.SceneInfo{}}
}
func ReadWithdrawalSecrets(tx *gorm.DB) (*WithdrawalSecrets, error) {
	var option Option
	err := tx.Where(clause.Eq{Column: "key", Value: WithdrawalConfigKey}).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &WithdrawalSecrets{Config: DefaultWithdrawalConfig(), Keys: map[string]string{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var secrets WithdrawalSecrets
	if err = decryptWithdrawal(option.Value, WithdrawalConfigKey, &secrets); err != nil {
		return nil, err
	}
	return &secrets, nil
}

func (config WithdrawalConfig) Validate() error {
	if !unipay.ValidText(config.PID, 64) || !unipay.KeyPattern.MatchString(config.APIKey) || config.MinCents < 1 || config.MaxCents < config.MinCents || config.MaxCents > unipay.MaxAmountCents {
		return ErrWithdrawalInvalid
	}
	if err := unipay.ValidateHTTPS(config.Gateway, true); err != nil {
		return err
	}
	rate, err := decimal.NewFromString(config.CNYPerUnit)
	if err != nil || rate.LessThanOrEqual(decimal.Zero) || rate.GreaterThan(decimal.NewFromInt(1000000)) || rate.Exponent() < -6 {
		return ErrWithdrawalInvalid
	}
	return (unipay.Request{OutBizNo: "validation", AmountCents: config.MinCents, PayeeAccount: "validation@example.com", PayeeName: "validation", Title: "Referral withdrawal", NotifyURL: config.NotifyURL, Scene: config.Scene, SceneInfos: config.SceneInfos}).Validate()
}

func SaveWithdrawalConfig(config WithdrawalConfig) error {
	if _, err := withdrawalCipher(); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		// Serialize saves across nodes and retain the complete notification keyring.
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: WithdrawalConfigKey, Value: ""}).Error; err != nil {
			return err
		}
		var option Option
		if err := lockForUpdate(tx).Where(clause.Eq{Column: "key", Value: WithdrawalConfigKey}).First(&option).Error; err != nil {
			return err
		}
		previous := WithdrawalSecrets{Keys: map[string]string{}}
		if option.Value != "" {
			if err := decryptWithdrawal(option.Value, WithdrawalConfigKey, &previous); err != nil {
				return err
			}
		}
		if config.APIKey == "" {
			config.APIKey = previous.Config.APIKey
		}
		if err := config.Validate(); err != nil {
			return err
		}
		if config.Enabled && !operation_setting.IsPaymentComplianceConfirmed() {
			return ErrWithdrawalUnavailable
		}
		if config.Gateway != previous.Config.Gateway || config.PID != previous.Config.PID {
			var outstanding int64
			if err := tx.Model(&AffiliateWithdrawal{}).Where("status IN ?", []string{WithdrawalPending, WithdrawalProcessing}).Count(&outstanding).Error; err != nil {
				return err
			}
			if outstanding > 0 {
				return errors.New("Resolve outstanding withdrawals before changing the gateway or merchant PID.")
			}
			previous.Keys = map[string]string{}
		}
		previous.Config = config
		if previous.Keys == nil {
			previous.Keys = map[string]string{}
		}
		previous.Keys[unipay.KeyID(config.APIKey)] = config.APIKey
		encrypted, err := encryptWithdrawal(previous, WithdrawalConfigKey)
		if err != nil {
			return err
		}
		return tx.Model(&option).Update("value", encrypted).Error
	})
}

func QuoteWithdrawal(config WithdrawalConfig, cents int64) (int, error) {
	if !config.Enabled || !operation_setting.IsPaymentComplianceConfirmed() {
		return 0, ErrWithdrawalUnavailable
	}
	if cents < config.MinCents || cents > config.MaxCents || cents <= 0 || cents > unipay.MaxAmountCents {
		return 0, ErrWithdrawalInvalid
	}
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return 0, ErrWithdrawalInvalid
	}
	rate, err := decimal.NewFromString(config.CNYPerUnit)
	if err != nil || rate.LessThanOrEqual(decimal.Zero) {
		return 0, ErrWithdrawalInvalid
	}
	// Always round the required quota up, so sub-quota fractions never mint cash.
	numerator := decimal.NewFromInt(cents).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	quotient, remainder := numerator.QuoRem(rate.Mul(decimal.NewFromInt(100)), 0)
	if !remainder.IsZero() {
		quotient = quotient.Add(decimal.NewFromInt(1))
	}
	quota, err := common.WalletQuotaFromDecimalStrict(quotient)
	if err != nil || quota <= 0 {
		return 0, ErrWithdrawalInvalid
	}
	return quota, nil
}

func (w *AffiliateWithdrawal) Snapshot() (*WithdrawalSnapshot, error) {
	var snapshot WithdrawalSnapshot
	if err := decryptWithdrawal(w.RequestCiphertext, w.ID, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func CreateAffiliateWithdrawal(userID int, input WithdrawalRequest) (*AffiliateWithdrawal, error) {
	if !unipay.Identifier.MatchString(input.ID) || !unipay.ValidText(input.PayeeAccount, 100) || !unipay.ValidText(input.PayeeName, 100) {
		return nil, ErrWithdrawalInvalid
	}
	var withdrawal AffiliateWithdrawal
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Settings lock keeps concurrent merchant changes outside order creation.
		var option Option
		if err := lockForUpdate(tx).Where(clause.Eq{Column: "key", Value: WithdrawalConfigKey}).First(&option).Error; err != nil {
			return ErrWithdrawalUnavailable
		}
		existingErr := tx.Where("id = ?", input.ID).First(&withdrawal).Error
		if existingErr == nil {
			snapshot, err := withdrawal.Snapshot()
			if err != nil {
				return err
			}
			if withdrawal.UserID != userID || withdrawal.AmountCents != input.AmountCents || withdrawal.Quota != input.Quota || snapshot.Payout.PayeeAccount != input.PayeeAccount || snapshot.Payout.PayeeName != input.PayeeName {
				return ErrWithdrawalMismatch
			}
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		secrets, err := ReadWithdrawalSecrets(tx)
		if err != nil {
			return err
		}
		quota, err := QuoteWithdrawal(secrets.Config, input.AmountCents)
		if err != nil {
			return err
		}
		if quota != input.Quota {
			return ErrWithdrawalMismatch
		}
		snapshot := WithdrawalSnapshot{Gateway: secrets.Config.Gateway, PID: secrets.Config.PID, Payout: unipay.Request{OutBizNo: input.ID, AmountCents: input.AmountCents, PayeeAccount: input.PayeeAccount, PayeeName: input.PayeeName, Title: "Referral withdrawal", NotifyURL: secrets.Config.NotifyURL, Scene: secrets.Config.Scene, SceneInfos: secrets.Config.SceneInfos}}
		if err := snapshot.Payout.Validate(); err != nil {
			return err
		}
		encrypted, err := encryptWithdrawal(snapshot, input.ID)
		if err != nil {
			return err
		}
		result := tx.Model(&User{}).Where("id = ? AND status = ? AND aff_quota >= ?", userID, common.UserStatusEnabled, quota).Update("aff_quota", gorm.Expr("aff_quota - ?", quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrWithdrawalFunds
		}
		withdrawal = AffiliateWithdrawal{ID: input.ID, UserID: userID, Quota: quota, AmountCents: input.AmountCents, Status: WithdrawalPending, RequestCiphertext: encrypted, CreatedAt: time.Now().Unix()}
		return tx.Create(&withdrawal).Error
	})
	return &withdrawal, err
}

func ReviewAffiliateWithdrawal(id string, reviewer int, approve bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var w AffiliateWithdrawal
		if err := lockForUpdate(tx).Where("id = ?", id).First(&w).Error; err != nil {
			return err
		}
		if w.Status != WithdrawalPending {
			return ErrWithdrawalState
		}
		if approve {
			secrets, err := ReadWithdrawalSecrets(tx)
			if err != nil {
				return err
			}
			if !secrets.Config.Enabled || !operation_setting.IsPaymentComplianceConfirmed() {
				return ErrWithdrawalUnavailable
			}
			var user User
			if err := tx.Select("status").First(&user, w.UserID).Error; err != nil {
				return err
			}
			if user.Status != common.UserStatusEnabled {
				return ErrWithdrawalUnavailable
			}
			w.Status = WithdrawalProcessing
		} else {
			if err := restoreWithdrawalQuota(tx, &w); err != nil {
				return err
			}
			w.Status = WithdrawalRejected
			w.CompletedAt = time.Now().Unix()
		}
		w.ReviewedBy = reviewer
		return tx.Save(&w).Error
	})
}

func restoreWithdrawalQuota(tx *gorm.DB, w *AffiliateWithdrawal) error {
	result := tx.Unscoped().Model(&User{}).Where("id = ? AND aff_quota <= ?", w.UserID, common.MaxWalletQuota-w.Quota).Update("aff_quota", gorm.Expr("aff_quota + ?", w.Quota))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrWalletQuotaLimitExceeded
	}
	return nil
}

func SettleAffiliateWithdrawal(result unipay.Result) error {
	if result.PayoutNo == "" || !unipay.Identifier.MatchString(result.PayoutNo) {
		return ErrWithdrawalMismatch
	}
	switch result.Status {
	case "queued", WithdrawalProcessing, WithdrawalSucceeded, WithdrawalFailed:
	default:
		return ErrWithdrawalMismatch
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var w AffiliateWithdrawal
		if err := lockForUpdate(tx).Where("id = ?", result.OutBizNo).First(&w).Error; err != nil {
			return err
		}
		if w.AmountCents != result.AmountCents || (w.PayoutNo != "" && w.PayoutNo != result.PayoutNo) {
			return ErrWithdrawalMismatch
		}
		if w.Status == WithdrawalSucceeded || w.Status == WithdrawalFailed {
			if w.Status != result.Status {
				return ErrWithdrawalState
			}
			return nil
		}
		if w.Status != WithdrawalProcessing {
			return ErrWithdrawalState
		}
		w.PayoutNo = result.PayoutNo
		if result.Status == WithdrawalFailed {
			if err := restoreWithdrawalQuota(tx, &w); err != nil {
				return err
			}
		}
		if result.Status == WithdrawalSucceeded || result.Status == WithdrawalFailed {
			w.Status = result.Status
			w.CompletedAt = time.Now().Unix()
		}
		if unipay.Identifier.MatchString(result.FailureCode) {
			w.FailureCode = result.FailureCode
		} else {
			w.FailureCode = ""
		}
		return tx.Save(&w).Error
	})
}

func ListAffiliateWithdrawals(userID int, page int) ([]WithdrawalView, bool, error) {
	query := DB.Model(&AffiliateWithdrawal{})
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	var records []AffiliateWithdrawal
	if err := query.Order("created_at desc").Order("id desc").Offset((page - 1) * 20).Limit(21).Find(&records).Error; err != nil {
		return nil, false, err
	}
	more := len(records) > 20
	if more {
		records = records[:20]
	}
	views := make([]WithdrawalView, 0, len(records))
	for _, w := range records {
		snapshot, err := w.Snapshot()
		if err != nil {
			return nil, false, err
		}
		views = append(views, WithdrawalView{AffiliateWithdrawal: w, PayeeAccount: snapshot.Payout.PayeeAccount, PayeeName: snapshot.Payout.PayeeName})
	}
	return views, more, nil
}
