package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/unipay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const withdrawalTestKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func setupWithdrawalTest(t *testing.T) (model.User, model.WithdrawalConfig) {
	t.Helper()
	db := setupManageUserTestDB(t)
	t.Setenv("CRYPTO_SECRET", "synthetic-withdrawal-encryption-key-for-tests")
	old := *operation_setting.GetPaymentSetting()
	oldUnit := common.QuotaPerUnit
	t.Cleanup(func() { *operation_setting.GetPaymentSetting() = old; common.QuotaPerUnit = oldUnit })
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	common.QuotaPerUnit = 500000
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.TopUp{}))
	// Representative pre-feature data must survive adding the new table twice.
	user := model.User{Username: "withdrawal-user", AffCode: "withdrawal-aff", Status: common.UserStatusEnabled, AffQuota: 500000, AffHistoryQuota: 500000}
	require.NoError(t, db.Create(&user).Error)
	for range 2 {
		require.NoError(t, db.AutoMigrate(&model.AffiliateWithdrawal{}))
	}
	var preserved model.User
	require.NoError(t, db.First(&preserved, user.Id).Error)
	assert.Equal(t, 500000, preserved.AffQuota)
	config := model.DefaultWithdrawalConfig()
	config.Enabled = true
	config.PID = "1001"
	config.APIKey = withdrawalTestKey
	config.NotifyURL = "https://business.example.com/api/user/withdrawals/notify"
	config.Scene = "Test scene"
	require.NoError(t, model.SaveWithdrawalConfig(config))
	return user, config
}

func createWithdrawalTestOrder(t *testing.T, user model.User, config model.WithdrawalConfig, id string, cents int64) *model.AffiliateWithdrawal {
	t.Helper()
	quota, err := model.QuoteWithdrawal(config, cents)
	require.NoError(t, err)
	w, err := model.CreateAffiliateWithdrawal(user.Id, model.WithdrawalRequest{ID: id, Quota: quota, AmountCents: cents, PayeeAccount: "test-payee@example.com", PayeeName: "Test Payee"})
	require.NoError(t, err)
	return w
}

func TestAffiliateWithdrawalLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		approve      bool
		remaining    int
	}{{"paid", model.WithdrawalSucceeded, true, 400000}, {"failed", model.WithdrawalFailed, true, 500000}, {"rejected", model.WithdrawalRejected, false, 500000}} {
		t.Run(tc.name, func(t *testing.T) {
			user, config := setupWithdrawalTest(t)
			w := createWithdrawalTestOrder(t, user, config, "wd_lifecycle", 140)
			assert.Equal(t, 100000, w.Quota)
			same := createWithdrawalTestOrder(t, user, config, w.ID, 140)
			assert.Equal(t, w.ID, same.ID)
			require.NoError(t, model.ReviewAffiliateWithdrawal(w.ID, 9999, tc.approve))
			if tc.approve {
				result := unipay.Result{OutBizNo: w.ID, AmountCents: w.AmountCents, PayoutNo: "PTEST1", Status: tc.status}
				require.NoError(t, model.SettleAffiliateWithdrawal(result))
				require.NoError(t, model.SettleAffiliateWithdrawal(result))
				wrong := result
				wrong.AmountCents++
				require.Error(t, model.SettleAffiliateWithdrawal(wrong))
				wrong = result
				wrong.PayoutNo = "OTHER"
				require.Error(t, model.SettleAffiliateWithdrawal(wrong))
				wrong = result
				wrong.Status = model.WithdrawalProcessing
				require.Error(t, model.SettleAffiliateWithdrawal(wrong))
			}
			require.NoError(t, model.DB.First(&user, user.Id).Error)
			assert.Equal(t, tc.remaining, user.AffQuota)
			assert.Equal(t, 500000, user.AffHistoryQuota)
			var persisted model.AffiliateWithdrawal
			require.NoError(t, model.DB.Where("id = ?", w.ID).First(&persisted).Error)
			assert.Equal(t, tc.status, persisted.Status)
			assert.NotContains(t, persisted.RequestCiphertext, "test-payee")
			require.Error(t, model.ReviewAffiliateWithdrawal(w.ID, 9999, false))
		})
	}
}

func TestAffiliateWithdrawalQuotaAndRollback(t *testing.T) {
	user, config := setupWithdrawalTest(t)
	config.MinCents = 1
	quota, err := model.QuoteWithdrawal(config, 1)
	require.NoError(t, err)
	assert.Equal(t, 715, quota)
	for _, cents := range []int64{-1, 0, 100000001} {
		_, err := model.QuoteWithdrawal(config, cents)
		require.Error(t, err)
	}
	input := model.WithdrawalRequest{ID: "wd_invalid", AmountCents: 140, Quota: 99999, PayeeAccount: "payee@example.com", PayeeName: "Payee"}
	_, err = model.CreateAffiliateWithdrawal(user.Id, input)
	require.ErrorIs(t, err, model.ErrWithdrawalMismatch)
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register("withdrawal:fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "affiliate_withdrawals" {
			tx.AddError(errors.New("synthetic failure"))
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Create().Remove("withdrawal:fail") })
	input.Quota = 100000
	_, err = model.CreateAffiliateWithdrawal(user.Id, input)
	require.Error(t, err)
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 500000, user.AffQuota)
	require.NoError(t, model.DB.Callback().Create().Remove("withdrawal:fail"))
	w := createWithdrawalTestOrder(t, user, config, "wd_capacity", 140)
	require.NoError(t, model.ReviewAffiliateWithdrawal(w.ID, 9999, true))
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("withdrawal:refund-fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(errors.New("synthetic refund failure"))
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Update().Remove("withdrawal:refund-fail") })
	result := unipay.Result{OutBizNo: w.ID, AmountCents: 140, PayoutNo: "PTEST2", Status: model.WithdrawalFailed}
	require.Error(t, model.SettleAffiliateWithdrawal(result))
	require.NoError(t, model.DB.Where("id = ?", w.ID).First(w).Error)
	assert.Equal(t, model.WithdrawalProcessing, w.Status)
	require.NoError(t, model.DB.Callback().Update().Remove("withdrawal:refund-fail"))
	require.NoError(t, model.SettleAffiliateWithdrawal(result))
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 500000, user.AffQuota)
}

func TestAffiliateWithdrawalConcurrentReservationAndTransfer(t *testing.T) {
	user, config := setupWithdrawalTest(t)
	var group sync.WaitGroup
	var successful atomic.Int32
	start := make(chan struct{})
	for _, id := range []string{"wd_concurrent_1", "wd_concurrent_2"} {
		group.Add(1)
		go func(id string) {
			defer group.Done()
			<-start
			_, err := model.CreateAffiliateWithdrawal(user.Id, model.WithdrawalRequest{ID: id, AmountCents: 700, Quota: 500000, PayeeAccount: "payee@example.com", PayeeName: "Payee"})
			if err == nil {
				successful.Add(1)
			}
		}(id)
	}
	group.Add(1)
	go func() {
		defer group.Done()
		<-start
		copy := user
		if copy.TransferAffQuotaToQuota(500000) == nil {
			successful.Add(1)
		}
	}()
	close(start)
	group.Wait()
	assert.EqualValues(t, 1, successful.Load())
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Zero(t, user.AffQuota)
	var frozen int64
	require.NoError(t, model.DB.Model(&model.AffiliateWithdrawal{}).Select("COALESCE(SUM(quota),0)").Scan(&frozen).Error)
	assert.EqualValues(t, 500000, int64(user.Quota)+frozen)
	_ = config
}

func TestAffiliateWithdrawalRefundNotificationAndRotation(t *testing.T) {
	user, config := setupWithdrawalTest(t)
	w := createWithdrawalTestOrder(t, user, config, "wd_notify", 140)
	require.NoError(t, model.ReviewAffiliateWithdrawal(w.ID, 9999, true))
	config.APIKey = strings.Repeat("b", 64)
	require.NoError(t, model.SaveWithdrawalConfig(config))
	config.PID = "different"
	require.Error(t, model.SaveWithdrawalConfig(config))
	config.PID = "1001"
	result := unipay.Result{PID: "1001", OutBizNo: w.ID, AmountCents: 140, PayoutNo: "PNOTIFY", Status: model.WithdrawalFailed}
	body, err := common.Marshal(result)
	require.NoError(t, err)
	send := func(raw []byte, signature string, stamp string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/user/withdrawals/notify", strings.NewReader(string(raw)))
		c.Request.Header.Set("X-Unipay-Pid", "1001")
		c.Request.Header.Set("X-Unipay-Timestamp", stamp)
		c.Request.Header.Set("X-Unipay-Nonce", "0123456789abcdef")
		c.Request.Header.Set("X-Unipay-Key-Id", unipay.KeyID(withdrawalTestKey))
		c.Request.Header.Set("X-Unipay-Signature", signature)
		WithdrawalNotify(c)
		c.Writer.WriteHeaderNow()
		return r
	}
	stamp := fmt.Sprint(time.Now().Unix())
	sig := unipay.Signature(withdrawalTestKey, "unipay-payout-notify-v1", "1001", stamp, "0123456789abcdef", "", body)
	assert.Equal(t, 401, send(body, "bad", stamp).Code)
	assert.Equal(t, 401, send(body, sig, "0").Code)
	tampered := strings.Replace(string(body), `"amount_cents":140`, `"amount_cents":141`, 1)
	assert.Equal(t, 401, send([]byte(tampered), sig, stamp).Code)
	signedMismatch := unipay.Signature(withdrawalTestKey, "unipay-payout-notify-v1", "1001", stamp, "0123456789abcdef", "", []byte(tampered))
	assert.Equal(t, 409, send([]byte(tampered), signedMismatch, stamp).Code)
	wrongPID := strings.Replace(string(body), `"pid":"1001"`, `"pid":"1002"`, 1)
	signedPID := unipay.Signature(withdrawalTestKey, "unipay-payout-notify-v1", "1001", stamp, "0123456789abcdef", "", []byte(wrongPID))
	assert.Equal(t, 400, send([]byte(wrongPID), signedPID, stamp).Code)

	for range 2 {
		response := send(body, sig, stamp)
		assert.Equal(t, 200, response.Code)
		assert.Equal(t, "success", response.Body.String())
	}
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 500000, user.AffQuota)
	var stored model.Option
	require.NoError(t, model.DB.Where(clause.Eq{Column: "key", Value: model.WithdrawalConfigKey}).First(&stored).Error)
	assert.NotContains(t, stored.Value, withdrawalTestKey)
	require.Error(t, model.UpdateOption(model.WithdrawalConfigKey, "{}"))
}

func TestAffiliateWithdrawalGatewayUnknownAndStableRetries(t *testing.T) {
	user, config := setupWithdrawalTest(t)
	var requests [][]byte
	var nonces []string
	var mode atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		expected := unipay.Signature(withdrawalTestKey, "unipay-payout-v1", r.Header.Get("X-Unipay-Pid"), r.Header.Get("X-Unipay-Timestamp"), r.Header.Get("X-Unipay-Nonce"), r.URL.Path, body)
		assert.Equal(t, expected, r.Header.Get("X-Unipay-Signature"))
		if r.URL.Path == unipay.QueryPath {
			if mode.Load() < 2 {
				w.WriteHeader(404)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"payout_no":"PGATEWAY","out_biz_no":"wd_gateway","amount_cents":140,"status":"succeeded"}`)
			return
		}
		requests = append(requests, append([]byte(nil), body...))
		nonces = append(nonces, r.Header.Get("X-Unipay-Nonce"))
		if mode.Load() == 0 {
			w.WriteHeader(502)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		fmt.Fprint(w, `{"payout_no":"PGATEWAY","out_biz_no":"wd_gateway","amount_cents":140,"status":"queued"}`)
	}))
	defer server.Close()
	config.Gateway = server.URL
	require.NoError(t, model.SaveWithdrawalConfig(config))
	order := createWithdrawalTestOrder(t, user, config, "wd_gateway", 140)
	require.NoError(t, model.ReviewAffiliateWithdrawal(order.ID, 9999, true))
	client := unipay.Client{BaseURL: server.URL, PID: config.PID, Key: withdrawalTestKey, HTTP: server.Client()}
	require.Error(t, service.ReconcileAffiliateWithdrawal(context.Background(), *order, client, true))
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 400000, user.AffQuota)
	// Disabling blocks an unaccepted create, but still permits authoritative queries.
	require.Error(t, service.ReconcileAffiliateWithdrawal(context.Background(), *order, client, false))
	assert.Len(t, requests, 1)
	config.Enabled = false
	require.NoError(t, model.SaveWithdrawalConfig(config))
	require.ErrorIs(t, service.ReconcileAffiliateWithdrawal(context.Background(), *order, client, true), model.ErrWithdrawalUnavailable)
	assert.Len(t, requests, 1, "a running batch honors disabling payouts")
	config.Enabled = true
	require.NoError(t, model.SaveWithdrawalConfig(config))
	mode.Store(1)
	require.NoError(t, service.ReconcileAffiliateWithdrawal(context.Background(), *order, client, true))
	require.Len(t, requests, 2)
	assert.Equal(t, requests[0], requests[1])
	assert.NotEqual(t, nonces[0], nonces[1])
	require.NoError(t, model.DB.Where("id = ?", order.ID).First(order).Error)
	mode.Store(0)
	require.Error(t, service.ReconcileAffiliateWithdrawal(context.Background(), *order, client, true))
	assert.Len(t, requests, 2, "accepted payouts cannot be sent again on 404")
	mode.Store(2)
	require.NoError(t, service.ReconcileAffiliateWithdrawal(context.Background(), *order, client, false))
	require.NoError(t, model.DB.Where("id = ?", order.ID).First(order).Error)
	assert.Equal(t, model.WithdrawalSucceeded, order.Status)
}

func TestAffiliateWithdrawalSecurityProofIsRequiredAndBound(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	t.Setenv("CRYPTO_SECRET", "synthetic-withdrawal-encryption-key-for-tests")
	operation := service.VerificationOperation{Scope: service.VerificationScopeWithdrawalCreate, Context: []byte(`{"id":"wd_proof","amount_cents":140,"quota":100000,"payee_account":"recipient@example.com","payee_name":"Recipient"}`)}
	requirements, err := service.GetVerificationRequirements(identity, operation.Scope)
	require.NoError(t, err)
	assert.Empty(t, requirements.Methods, "withdrawals do not fall back to a password when no strong factor is enrolled")
	require.NoError(t, model.DB.Create(&model.PasskeyCredential{UserID: user.Id, CredentialID: "withdrawal-passkey"}).Error)
	proof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPasskey)
	response := securityEnrollmentRequest("POST", "/api/user/withdrawals", string(operation.Context), "", identity, CreateWithdrawal)
	assert.Equal(t, 403, response.Code)
	tampered := operation
	tampered.Context = []byte(strings.Replace(string(operation.Context), "140", "141", 1))
	_, err = service.ConsumeOperationProof(proof, identity, tampered)
	require.ErrorIs(t, err, service.ErrProofContext)
	_, err = service.ConsumeOperationProof(proof, identity, operation)
	require.NoError(t, err)
	_, err = service.ConsumeOperationProof(proof, identity, operation)
	require.ErrorIs(t, err, service.ErrProofConsumed)
	_, err = service.GetVerificationRequirements(identity, service.VerificationScopeWithdrawalReview)
	require.ErrorIs(t, err, service.ErrVerificationForbidden)
	// The production root guard rejects ordinary users before any configuration can be disclosed.
	engine := gin.New()
	engine.GET("/config", middleware.RootAuth(), GetWithdrawalConfig)
	request := httptest.NewRequest("GET", "/config", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, 401, recorder.Code)
}

func TestAffiliateWithdrawalSignatureAndConfigDigest(t *testing.T) {
	// Fixed vectors independently calculated with Python hashlib/hmac.
	body := []byte(`{"out_biz_no":"wd_test"}`)
	assert.Equal(t, "175ccb70082655f0e341186af316a3d2ca9beadbd6b45a736f392771408d769f", unipay.Signature(withdrawalTestKey, "unipay-payout-v1", "1001", "1788998400", "0123456789abcdef", unipay.QueryPath, body))
	config := model.WithdrawalConfig{Gateway: "https://pay.yunnet.top", PID: "1001", CNYPerUnit: "7", MinCents: 100, MaxCents: 10000, Scene: "测试 & < >\u2028\u2029", SceneInfos: []unipay.SceneInfo{}}
	digest, err := service.WithdrawalConfigDigest(config)
	require.NoError(t, err)
	assert.Equal(t, "ef7dbb1924fb8e17aa81938c214ad69e054d36efae9afd2c1de3d808188fc60e", digest)
}

func TestAffiliateWithdrawalAuthenticatedRequestsAndIsolation(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	t.Setenv("CRYPTO_SECRET", "synthetic-withdrawal-encryption-key-for-tests")
	old := *operation_setting.GetPaymentSetting()
	t.Cleanup(func() { *operation_setting.GetPaymentSetting() = old })
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}, &model.AffiliateWithdrawal{}))
	require.NoError(t, model.DB.Create(&model.PasskeyCredential{UserID: user.Id, CredentialID: "withdrawal-http-passkey"}).Error)
	require.NoError(t, model.DB.Model(user).Updates(map[string]any{"aff_quota": 500000, "quota": 2000000}).Error)
	config := model.DefaultWithdrawalConfig()
	config.Enabled, config.PID, config.APIKey = true, "1001", withdrawalTestKey
	config.NotifyURL, config.Scene = "https://business.example.com/api/user/withdrawals/notify", "Test scene"
	require.NoError(t, model.SaveWithdrawalConfig(config))
	quota, err := model.QuoteWithdrawal(config, 140)
	require.NoError(t, err)
	input := model.WithdrawalRequest{ID: "wd_http", AmountCents: 140, Quota: quota, PayeeAccount: "recipient@example.com", PayeeName: "Recipient"}
	body, err := common.Marshal(input)
	require.NoError(t, err)
	operation := service.VerificationOperation{Scope: service.VerificationScopeWithdrawalCreate, Context: body}
	proof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPasskey)
	response := securityEnrollmentRequest("POST", "/api/user/withdrawals", string(body), proof, identity, CreateWithdrawal)
	assert.Equal(t, 200, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "request_ciphertext")
	response = securityEnrollmentRequest("POST", "/api/user/withdrawals", string(body), proof, identity, CreateWithdrawal)
	assert.Equal(t, 403, response.Code)
	// A fresh proof may retry the same request without another debit.
	proof = issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPasskey)
	response = securityEnrollmentRequest("POST", "/api/user/withdrawals", string(body), proof, identity, CreateWithdrawal)
	assert.Equal(t, 200, response.Code, response.Body.String())
	require.NoError(t, model.DB.First(user, user.Id).Error)
	assert.Equal(t, 500000-quota, user.AffQuota)
	assert.Equal(t, 2000000, user.Quota)
	input.PayeeAccount = "other@example.com"
	_, err = model.CreateAffiliateWithdrawal(user.Id, input)
	require.ErrorIs(t, err, model.ErrWithdrawalMismatch)
	input.PayeeAccount = "recipient@example.com"
	_, err = model.CreateAffiliateWithdrawal(user.Id+1, input)
	require.ErrorIs(t, err, model.ErrWithdrawalMismatch)
	rows, _, err := model.ListAffiliateWithdrawals(user.Id+1, 1)
	require.NoError(t, err)
	assert.Empty(t, rows)
	rows, _, err = model.ListAffiliateWithdrawals(user.Id, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "recipient@example.com", rows[0].PayeeAccount)
	// Main wallet funds cannot fund an additional referral withdrawal.
	require.NoError(t, model.DB.Model(user).Update("aff_quota", 0).Error)
	input.ID = "wd_no_rewards"
	_, err = model.CreateAffiliateWithdrawal(user.Id, input)
	require.ErrorIs(t, err, model.ErrWithdrawalFunds)
	// Full body parsing rejects trailing objects and oversized valid prefixes.
	for _, bad := range []string{string(body) + "{}", string(body) + strings.Repeat(" ", 4096)} {
		response = securityEnrollmentRequest("POST", "/api/user/withdrawals", bad, "", identity, CreateWithdrawal)
		assert.Equal(t, 400, response.Code)
	}
	configResponse := securityEnrollmentRequest("GET", "/config", "", "", identity, GetWithdrawalConfig)
	assert.NotContains(t, configResponse.Body.String(), withdrawalTestKey)
	assert.Contains(t, configResponse.Body.String(), `"key_configured":true`)
	// Encrypted recipient details cannot be moved to another order.
	var original model.AffiliateWithdrawal
	require.NoError(t, model.DB.Where("id = ?", "wd_http").First(&original).Error)
	original.ID = "different-order"
	_, err = original.Snapshot()
	require.Error(t, err)

	// Expired DB proof and a revoked session both fail even with a valid signature.
	proof = issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPasskey)
	require.NoError(t, model.DB.Model(&model.AuthFlow{}).Where("user_id = ?", user.Id).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	_, err = service.ConsumeOperationProof(proof, identity, operation)
	require.ErrorIs(t, err, service.ErrAuthTokenExpired)
	proof = issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPasskey)
	require.NoError(t, model.DB.Model(user).Update("auth_version", user.AuthVersion+1).Error)
	_, err = service.ConsumeOperationProof(proof, identity, operation)
	require.Error(t, err)
}

func TestAffiliateWithdrawalPaidRechargeRewardOnce(t *testing.T) {
	inviter, _ := setupWithdrawalTest(t)
	previous := common.InviteRewardRatio
	t.Cleanup(func() { common.InviteRewardRatio = previous })
	common.InviteRewardRatio = 0.1
	invitee := model.User{Username: "paying-invitee", Status: common.UserStatusEnabled, InviterId: inviter.Id, AffCode: "paid-invitee"}
	require.NoError(t, model.DB.Create(&invitee).Error)
	topup := model.TopUp{UserId: invitee.Id, TradeNo: "paid-referral", Amount: 2, Money: 14, Status: common.TopUpStatusPending, PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay"}
	require.NoError(t, model.DB.Create(&topup).Error)
	done, err := model.RechargeEpay(topup.TradeNo, "alipay", "")
	require.NoError(t, err)
	assert.False(t, done)
	done, err = model.RechargeEpay(topup.TradeNo, "alipay", "")
	require.NoError(t, err)
	assert.True(t, done)
	require.NoError(t, model.DB.First(&inviter, inviter.Id).Error)
	assert.Equal(t, 600000, inviter.AffQuota)
	assert.Equal(t, 600000, inviter.AffHistoryQuota)
	require.NoError(t, model.DB.First(&invitee, invitee.Id).Error)
	assert.Equal(t, 1000000, invitee.Quota)
}
