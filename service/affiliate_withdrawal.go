package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/unipay"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const VerificationScopeWithdrawalCreate = "withdrawal.create"
const VerificationScopeWithdrawalReview = "withdrawal.review"
const VerificationScopeWithdrawalConfigure = "withdrawal.configure"

type WithdrawalReviewContext struct {
	ID      string `json:"id"`
	Approve bool   `json:"approve"`
}

type WithdrawalConfigContext struct {
	Digest string `json:"digest"`
}

func WithdrawalConfigDigest(config model.WithdrawalConfig) (string, error) {
	body, err := common.Marshal(config)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// ReconcileAffiliateWithdrawal only releases funds on a verified terminal
// result. A 404 is a reason to retry the identical unaccepted request, never a
// reason to release frozen funds. Previously accepted payouts are query-only.
func ReconcileAffiliateWithdrawal(ctx context.Context, w model.AffiliateWithdrawal, client unipay.Client, enabled bool) error {
	// Re-read before any network request; callers cannot submit an unreviewed
	// order or retry a terminal order using a stale snapshot.
	if err := model.DB.Where("id = ?", w.ID).First(&w).Error; err != nil {
		return err
	}
	if w.Status != model.WithdrawalProcessing {
		return model.ErrWithdrawalState
	}
	snapshot, err := w.Snapshot()
	if err != nil {
		return err
	}
	if client.BaseURL != snapshot.Gateway || client.PID != snapshot.PID {
		return model.ErrWithdrawalMismatch
	}
	result, status, err := client.Call(ctx, unipay.QueryPath, map[string]string{"out_biz_no": w.ID})
	if err != nil && status == http.StatusNotFound && w.PayoutNo == "" && enabled {
		// Recheck after the query so disabling payouts takes effect for the
		// remainder of a running batch, while accepted orders still reconcile.
		current, configErr := model.ReadWithdrawalSecrets(model.DB)
		if configErr != nil {
			return configErr
		}
		if !current.Config.Enabled || !operation_setting.IsPaymentComplianceConfirmed() {
			return model.ErrWithdrawalUnavailable
		}
		client.Key = current.Config.APIKey
		result, _, err = client.Call(ctx, unipay.CreatePath, snapshot.Payout)
	}
	if err != nil {
		return err
	}
	if result.OutBizNo != w.ID || result.AmountCents != w.AmountCents {
		return model.ErrWithdrawalMismatch
	}
	return model.SettleAffiliateWithdrawal(*result)
}

type affiliateWithdrawalTask struct{}

func (affiliateWithdrawalTask) Type() string            { return "affiliate_withdrawal_reconcile" }
func (affiliateWithdrawalTask) Interval() time.Duration { return time.Minute }
func (affiliateWithdrawalTask) NewPayload() any         { return struct{}{} }
func (affiliateWithdrawalTask) Enabled() bool {
	var count int64
	return model.DB.Model(&model.AffiliateWithdrawal{}).Where("status = ?", model.WithdrawalProcessing).Limit(1).Count(&count).Error == nil && count > 0
}
func (affiliateWithdrawalTask) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	var workErr error
	defer func() {
		status := model.SystemTaskStatusSucceeded
		message := ""
		if workErr != nil {
			status = model.SystemTaskStatusFailed
			message = "Withdrawal reconciliation requires attention; funds remain frozen."
		}
		if err := model.FinishSystemTask(task.TaskID, runnerID, status, nil, message); err != nil {
			common.SysError("failed to finish withdrawal reconciliation task")
		}
	}()
	secrets, err := model.ReadWithdrawalSecrets(model.DB)
	if err != nil {
		workErr = err
		return
	}
	var records []model.AffiliateWithdrawal
	workErr = model.DB.Where("status = ? AND next_attempt_at <= ?", model.WithdrawalProcessing, time.Now().Unix()).Order("next_attempt_at").Order("id").Limit(10).Find(&records).Error
	if workErr != nil {
		return
	}
	client := unipay.Client{BaseURL: secrets.Config.Gateway, PID: secrets.Config.PID, Key: secrets.Config.APIKey}
	for _, w := range records {
		if ctx.Err() != nil {
			workErr = ctx.Err()
			return
		}
		// One shared system task lease across nodes, at most 20 gateway calls/minute.
		if err := model.DB.Model(&model.AffiliateWithdrawal{}).Where("id = ?", w.ID).Update("next_attempt_at", time.Now().Unix()+60).Error; err != nil {
			workErr = err
			continue
		}
		err := ReconcileAffiliateWithdrawal(ctx, w, client, secrets.Config.Enabled && operation_setting.IsPaymentComplianceConfirmed())
		if err != nil && !errors.Is(err, context.Canceled) {
			workErr = err
			common.SysError("withdrawal reconciliation pending: " + w.ID)
		}
	}
}
func init() { RegisterSystemTaskHandler(affiliateWithdrawalTask{}) }
