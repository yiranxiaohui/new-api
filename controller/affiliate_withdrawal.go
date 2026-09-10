package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/unipay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func withdrawalError(c *gin.Context, err error) {
	// Never serialize DB/transport errors: they may contain encrypted records or payee data.
	message := "Please try again later."
	for _, known := range []error{model.ErrWithdrawalUnavailable, model.ErrWithdrawalInvalid, model.ErrWithdrawalFunds, model.ErrWithdrawalState, model.ErrWithdrawalMismatch, model.ErrWithdrawalStorageKey} {
		if errors.Is(err, known) {
			message = known.Error()
			break
		}
	}
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": message})
}

// Decode the complete bounded body: a JSON prefix must not bypass size limits.
func decodeWithdrawalRequest(c *gin.Context, limit int64, value any) error {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, limit+1))
	if err != nil || int64(len(body)) > limit || common.Unmarshal(body, value) != nil {
		return model.ErrWithdrawalInvalid
	}
	return nil
}

func GetWithdrawalConfig(c *gin.Context) {
	secrets, err := model.ReadWithdrawalSecrets(model.DB)
	if err != nil {
		withdrawalError(c, err)
		return
	}
	config := secrets.Config
	configured := config.APIKey != ""
	config.APIKey = ""
	common.ApiSuccess(c, gin.H{"config": config, "key_configured": configured})
}
func PutWithdrawalConfig(c *gin.Context) {
	var config model.WithdrawalConfig
	if err := decodeWithdrawalRequest(c, unipay.MaxBody, &config); err != nil {
		withdrawalError(c, model.ErrWithdrawalInvalid)
		return
	}
	digest, err := service.WithdrawalConfigDigest(config)
	if err != nil {
		withdrawalError(c, err)
		return
	}
	context, _ := common.Marshal(service.WithdrawalConfigContext{Digest: digest})
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeWithdrawalConfigure, Context: context}) == nil {
		return
	}
	if err := model.SaveWithdrawalConfig(config); err != nil {
		withdrawalError(c, err)
		return
	}
	recordManageAudit(c, "withdrawal.configure", map[string]interface{}{"enabled": config.Enabled})
	common.ApiSuccess(c, gin.H{})
}
func GetWithdrawalPolicy(c *gin.Context) {
	secrets, err := model.ReadWithdrawalSecrets(model.DB)
	if err != nil {
		withdrawalError(c, err)
		return
	}
	config := secrets.Config
	common.ApiSuccess(c, gin.H{"enabled": config.Enabled && operation_setting.IsPaymentComplianceConfirmed(), "min_cents": config.MinCents, "max_cents": config.MaxCents, "cny_per_unit": config.CNYPerUnit})
}
func QuoteWithdrawal(c *gin.Context) {
	cents, err := strconv.ParseInt(c.Query("amount_cents"), 10, 64)
	if err != nil {
		withdrawalError(c, model.ErrWithdrawalInvalid)
		return
	}
	secrets, err := model.ReadWithdrawalSecrets(model.DB)
	if err != nil {
		withdrawalError(c, err)
		return
	}
	quota, err := model.QuoteWithdrawal(secrets.Config, cents)
	if err != nil {
		withdrawalError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"quota": quota, "amount_cents": cents})
}
func CreateWithdrawal(c *gin.Context) {
	var input model.WithdrawalRequest
	if err := decodeWithdrawalRequest(c, 4096, &input); err != nil {
		withdrawalError(c, model.ErrWithdrawalInvalid)
		return
	}
	context, _ := common.Marshal(input)
	success := false
	defer func() {
		recordUserSecurityAudit(c, c.GetInt("id"), "withdrawal.create", map[string]interface{}{"success": success})
	}()
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeWithdrawalCreate, Context: context}) == nil {
		return
	}
	withdrawal, err := model.CreateAffiliateWithdrawal(c.GetInt("id"), input)
	if err != nil {
		withdrawalError(c, err)
		return
	}
	success = true
	common.ApiSuccess(c, withdrawal)
}
func GetWithdrawals(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 100000 {
		withdrawalError(c, model.ErrWithdrawalInvalid)
		return
	}
	userID := c.GetInt("id")
	if c.GetBool("withdrawal_admin") {
		userID = 0
	}
	rows, more, err := model.ListAffiliateWithdrawals(userID, page)
	if err != nil {
		withdrawalError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows, "has_more": more})
}
func ReviewWithdrawal(c *gin.Context) {
	var input service.WithdrawalReviewContext
	if err := decodeWithdrawalRequest(c, 1024, &input); err != nil {
		withdrawalError(c, model.ErrWithdrawalInvalid)
		return
	}
	context, _ := common.Marshal(input)
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeWithdrawalReview, Context: context}) == nil {
		return
	}
	if err := model.ReviewAffiliateWithdrawal(input.ID, c.GetInt("id"), input.Approve); err != nil {
		withdrawalError(c, err)
		return
	}
	recordManageAudit(c, "withdrawal.review", map[string]interface{}{"withdrawal_id": input.ID, "approved": input.Approve})
	common.ApiSuccess(c, gin.H{})
}
func WithdrawalNotify(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, unipay.MaxBody+1))
	if err != nil || len(body) > unipay.MaxBody {
		c.Status(http.StatusBadRequest)
		return
	}
	secrets, err := model.ReadWithdrawalSecrets(model.DB)
	if err != nil || !unipay.VerifyNotification(c.Request.Header, body, secrets.Config.PID, secrets.Keys, time.Now()) {
		c.Status(http.StatusUnauthorized)
		return
	}
	var result unipay.Result
	if common.Unmarshal(body, &result) != nil || result.PID != secrets.Config.PID || (result.Status != model.WithdrawalSucceeded && result.Status != model.WithdrawalFailed) {
		c.Status(http.StatusBadRequest)
		return
	}
	if err := model.SettleAffiliateWithdrawal(result); err != nil {
		c.Status(http.StatusConflict)
		return
	}
	c.Data(http.StatusOK, "text/plain", []byte("success"))
}
