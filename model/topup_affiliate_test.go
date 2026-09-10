package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreditTopUpQuotaCreditsConfiguredInviterRatio(t *testing.T) {
	truncateTables(t)
	oldRatio := common.InviteRewardRatio
	oldCompliance := *operation_setting.GetPaymentSetting()
	t.Cleanup(func() {
		common.InviteRewardRatio = oldRatio
		*operation_setting.GetPaymentSetting() = oldCompliance
	})
	common.InviteRewardRatio = 0.1
	settings := operation_setting.GetPaymentSetting()
	settings.ComplianceConfirmed = true
	settings.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	inviter := &User{Username: "inviter", Password: "x", Status: common.UserStatusEnabled, AffCode: "inviter-aff"}
	invitee := &User{Username: "invitee", Password: "x", Status: common.UserStatusEnabled, AffCode: "invitee-aff", InviterId: 0}
	require.NoError(t, DB.Create(inviter).Error)
	invitee.InviterId = inviter.Id
	require.NoError(t, DB.Create(invitee).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return creditPaidTopUpQuota(tx, invitee.Id, 1000, nil)
	}))

	var got User
	require.NoError(t, DB.First(&got, inviter.Id).Error)
	assert.Equal(t, 100, got.AffQuota)
	assert.Equal(t, 100, got.AffHistoryQuota)
	var gotInvitee User
	require.NoError(t, DB.First(&gotInvitee, invitee.Id).Error)
	assert.Equal(t, 1000, gotInvitee.Quota)
	// Redemption uses the wallet-only path and must never generate withdrawable rewards.
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return creditTopUpQuota(tx, invitee.Id, 1000, nil) }))
	require.NoError(t, DB.First(&got, inviter.Id).Error)
	assert.Equal(t, 100, got.AffQuota)
	require.NoError(t, DB.Model(inviter).Updates(map[string]any{"aff_quota": common.MaxWalletQuota, "aff_history": common.MaxWalletQuota}).Error)
	err := DB.Transaction(func(tx *gorm.DB) error { return creditPaidTopUpQuota(tx, invitee.Id, 1000, nil) })
	require.ErrorIs(t, err, ErrWalletQuotaLimitExceeded)
	require.NoError(t, DB.First(&gotInvitee, invitee.Id).Error)
	assert.Equal(t, 2000, gotInvitee.Quota, "reward overflow rolls back the recharge")
}

func TestCreditTopUpQuotaDoesNotRewardWithoutInviterOrRatio(t *testing.T) {
	truncateTables(t)
	oldRatio := common.InviteRewardRatio
	t.Cleanup(func() { common.InviteRewardRatio = oldRatio })
	common.InviteRewardRatio = 0
	invitee := &User{Username: "invitee-no-reward", Password: "x", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(invitee).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return creditPaidTopUpQuota(tx, invitee.Id, 1000, nil)
	}))
	var got User
	require.NoError(t, DB.First(&got, invitee.Id).Error)
	assert.Equal(t, 1000, got.Quota)
	assert.Zero(t, got.AffQuota)
}
