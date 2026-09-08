package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionPlanUsageWindow(t *testing.T) {
	tests := []struct {
		name string
		plan SubscriptionPlan
		at   time.Time
		want bool
	}{
		{
			name: "empty window is available all day",
			plan: SubscriptionPlan{UsageWindowTimezone: "UTC"},
			at:   time.Date(2026, time.September, 8, 18, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "daytime window includes start",
			plan: SubscriptionPlan{UsageWindowStart: "09:00", UsageWindowEnd: "17:00", UsageWindowTimezone: "Asia/Shanghai"},
			at:   time.Date(2026, time.September, 8, 1, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "daytime window excludes end",
			plan: SubscriptionPlan{UsageWindowStart: "09:00", UsageWindowEnd: "17:00", UsageWindowTimezone: "Asia/Shanghai"},
			at:   time.Date(2026, time.September, 8, 9, 0, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "overnight window includes late night",
			plan: SubscriptionPlan{UsageWindowStart: "22:00", UsageWindowEnd: "06:00", UsageWindowTimezone: "UTC"},
			at:   time.Date(2026, time.September, 8, 23, 30, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "overnight window includes early morning",
			plan: SubscriptionPlan{UsageWindowStart: "22:00", UsageWindowEnd: "06:00", UsageWindowTimezone: "UTC"},
			at:   time.Date(2026, time.September, 8, 5, 59, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "overnight window excludes daytime",
			plan: SubscriptionPlan{UsageWindowStart: "22:00", UsageWindowEnd: "06:00", UsageWindowTimezone: "UTC"},
			at:   time.Date(2026, time.September, 8, 12, 0, 0, 0, time.UTC),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, test.plan.ValidateUsageWindow())
			assert.Equal(t, test.want, test.plan.IsWithinUsageWindow(test.at))
		})
	}
}

func TestSubscriptionPlanUsageWindowValidation(t *testing.T) {
	tests := []SubscriptionPlan{
		{UsageWindowStart: "09:00", UsageWindowTimezone: "UTC"},
		{UsageWindowStart: "09:00", UsageWindowEnd: "09:00", UsageWindowTimezone: "UTC"},
		{UsageWindowStart: "9:00", UsageWindowEnd: "17:00", UsageWindowTimezone: "UTC"},
		{UsageWindowStart: "09:00", UsageWindowEnd: "17:00", UsageWindowTimezone: "Mars/Olympus"},
	}

	for _, plan := range tests {
		assert.Error(t, plan.ValidateUsageWindow())
		assert.False(t, plan.IsWithinUsageWindow(time.Now()))
	}
}

func TestActiveSubscriptionUsageWindowEligibilityAndWalletFallback(t *testing.T) {
	truncateTables(t)
	at := time.Date(2026, time.September, 8, 10, 0, 0, 0, time.UTC)
	closedPlan := &SubscriptionPlan{
		Id:                  9701,
		Title:               "Night plan",
		DurationUnit:        SubscriptionDurationMonth,
		DurationValue:       1,
		UsageWindowStart:    "22:00",
		UsageWindowEnd:      "06:00",
		UsageWindowTimezone: "UTC",
	}
	require.NoError(t, DB.Create(closedPlan).Error)
	InvalidateSubscriptionPlanCache(closedPlan.Id)
	require.NoError(t, DB.Create(&UserSubscription{
		Id: 9702, UserId: 9701, PlanId: closedPlan.Id, Status: "active",
		StartTime: at.Add(-time.Hour).Unix(), EndTime: at.Add(time.Hour).Unix(),
		AllowWalletOverflow: false,
	}).Error)

	hasEligible, err := HasActiveUserSubscriptionInUsageWindow(9701, at)
	require.NoError(t, err)
	assert.False(t, hasEligible)
	allowWallet, err := UserActiveSubscriptionsAllowWalletOverflowInUsageWindow(9701, at)
	require.NoError(t, err)
	assert.True(t, allowWallet)

	openPlan := &SubscriptionPlan{
		Id:                  9703,
		Title:               "Day plan",
		DurationUnit:        SubscriptionDurationMonth,
		DurationValue:       1,
		UsageWindowStart:    "09:00",
		UsageWindowEnd:      "17:00",
		UsageWindowTimezone: "UTC",
	}
	require.NoError(t, DB.Create(openPlan).Error)
	InvalidateSubscriptionPlanCache(openPlan.Id)
	require.NoError(t, DB.Create(&UserSubscription{
		Id: 9704, UserId: 9701, PlanId: openPlan.Id, Status: "active",
		StartTime: at.Add(-time.Hour).Unix(), EndTime: at.Add(time.Hour).Unix(),
		AllowWalletOverflow: false,
	}).Error)

	hasEligible, err = HasActiveUserSubscriptionInUsageWindow(9701, at)
	require.NoError(t, err)
	assert.True(t, hasEligible)
	allowWallet, err = UserActiveSubscriptionsAllowWalletOverflowInUsageWindow(9701, at)
	require.NoError(t, err)
	assert.False(t, allowWallet)
}

func TestPreConsumeUserSubscriptionSkipsPlanOutsideUsageWindow(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM subscription_pre_consume_records") })

	now := time.Unix(GetDBTimestamp(), 0).UTC()
	startMinute := (now.Hour()*60 + now.Minute() + 2) % (24 * 60)
	endMinute := (startMinute + 1) % (24 * 60)
	closedPlan := &SubscriptionPlan{
		Id:                  9711,
		Title:               "Closed window",
		DurationUnit:        SubscriptionDurationMonth,
		DurationValue:       1,
		UsageWindowStart:    fmt.Sprintf("%02d:%02d", startMinute/60, startMinute%60),
		UsageWindowEnd:      fmt.Sprintf("%02d:%02d", endMinute/60, endMinute%60),
		UsageWindowTimezone: "UTC",
	}
	openPlan := &SubscriptionPlan{
		Id:            9712,
		Title:         "All day",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
	}
	require.NoError(t, DB.Create(closedPlan).Error)
	require.NoError(t, DB.Create(openPlan).Error)
	InvalidateSubscriptionPlanCache(closedPlan.Id)
	InvalidateSubscriptionPlanCache(openPlan.Id)
	require.NoError(t, DB.Create(&UserSubscription{
		Id: 9713, UserId: 9711, PlanId: closedPlan.Id, Status: "active",
		StartTime: now.Add(-time.Hour).Unix(), EndTime: now.Add(time.Hour).Unix(),
		AmountTotal: 100, AllowWalletOverflow: true,
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id: 9714, UserId: 9711, PlanId: openPlan.Id, Status: "active",
		StartTime: now.Add(-time.Hour).Unix(), EndTime: now.Add(2 * time.Hour).Unix(),
		AmountTotal: 100, AllowWalletOverflow: true,
	}).Error)

	result, err := PreConsumeUserSubscription("usage-window-request", 9711, "test-model", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 9714, result.UserSubscriptionId)
	assert.Zero(t, getSubscriptionResetSub(t, 9713).AmountUsed)
	assert.EqualValues(t, 10, getSubscriptionResetSub(t, 9714).AmountUsed)
}
