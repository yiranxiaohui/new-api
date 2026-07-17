package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fillTopUpUsernames is one of our local-only patches on top of upstream.
// This test pins its contract so future upstream syncs that touch TopUp
// won't silently break the admin-side topup listing.

func TestFillTopUpUsernames_EmptyInput(t *testing.T) {
	// Should not panic and should be a no-op.
	fillTopUpUsernames(nil)
	fillTopUpUsernames([]*TopUp{})
}

func TestFillTopUpUsernames_PopulatesUsernameByUserId(t *testing.T) {
	truncateTables(t)

	// Seed two users. aff_code has a UNIQUE index, so give each user a distinct value.
	alice := &User{Username: "alice", Password: "x", Role: 1, Status: 1, AffCode: "aff-alice"}
	bob := &User{Username: "bob", Password: "x", Role: 1, Status: 1, AffCode: "aff-bob"}
	require.NoError(t, DB.Create(alice).Error)
	require.NoError(t, DB.Create(bob).Error)

	// Build TopUps referencing both users (and an unknown user id).
	topups := []*TopUp{
		{UserId: alice.Id, Amount: 100, Money: 1.0, TradeNo: "t-a-1", Status: "success"},
		{UserId: bob.Id, Amount: 200, Money: 2.0, TradeNo: "t-b-1", Status: "success"},
		{UserId: alice.Id, Amount: 300, Money: 3.0, TradeNo: "t-a-2", Status: "pending"},
		{UserId: 99999, Amount: 400, Money: 4.0, TradeNo: "t-x-1", Status: "pending"}, // unknown user
	}

	fillTopUpUsernames(topups)

	assert.Equal(t, "alice", topups[0].Username, "topup[0] should be alice")
	assert.Equal(t, "bob", topups[1].Username, "topup[1] should be bob")
	assert.Equal(t, "alice", topups[2].Username, "topup[2] should also be alice (duplicate user id)")
	assert.Equal(t, "", topups[3].Username, "topup[3] references unknown user => empty username")
}

func TestFillTopUpUsernames_UsernameIsNotPersisted(t *testing.T) {
	// Username has `gorm:"-"`, so it must never be written to the DB column.
	truncateTables(t)

	carol := &User{Username: "carol", Password: "x", Role: 1, Status: 1, AffCode: "aff-carol"}
	require.NoError(t, DB.Create(carol).Error)

	topup := &TopUp{
		UserId:   carol.Id,
		Amount:   500,
		Money:    5.0,
		TradeNo:  "t-carol-1",
		Status:   "success",
		Username: "should-be-ignored-on-write",
	}
	require.NoError(t, DB.Create(topup).Error)

	// Reload from DB — Username column should NOT exist / be empty.
	var reloaded TopUp
	require.NoError(t, DB.First(&reloaded, topup.Id).Error)
	assert.Equal(t, "", reloaded.Username, "Username must be transient (gorm:\"-\"), never persisted")

	// And fillTopUpUsernames repopulates it from the users table.
	fillTopUpUsernames([]*TopUp{&reloaded})
	assert.Equal(t, "carol", reloaded.Username)
}

func TestFillTopUpUsernames_PaymentProviderFieldCoexists(t *testing.T) {
	// Sanity: our local Username field must coexist with upstream's PaymentProvider field.
	// If a future upstream sync drops Username or PaymentProvider, this test must keep failing
	// until the merge re-applies both.
	truncateTables(t)

	dave := &User{Username: "dave", Password: "x", Role: 1, Status: 1, AffCode: "aff-dave"}
	require.NoError(t, DB.Create(dave).Error)

	topup := &TopUp{
		UserId:          dave.Id,
		Amount:          600,
		Money:           6.0,
		TradeNo:         "t-dave-1",
		Status:          "pending",
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
	}
	require.NoError(t, DB.Create(topup).Error)

	var reloaded TopUp
	require.NoError(t, DB.First(&reloaded, topup.Id).Error)
	assert.Equal(t, PaymentProviderStripe, reloaded.PaymentProvider, "PaymentProvider must persist (upstream security fix)")

	fillTopUpUsernames([]*TopUp{&reloaded})
	assert.Equal(t, "dave", reloaded.Username, "Username must be filled (local patch)")
}

func TestSearchAllTopUps_ByUsername(t *testing.T) {
	truncateTables(t)

	// Seed two users with distinct AffCode (AffCode has a UNIQUE index).
	alice := &User{Username: "alice_pay", Password: "x", Role: 1, Status: 1, AffCode: "aff-alice-pay"}
	bob := &User{Username: "bob_pay", Password: "x", Role: 1, Status: 1, AffCode: "aff-bob-pay"}
	require.NoError(t, DB.Create(alice).Error)
	require.NoError(t, DB.Create(bob).Error)

	now := time.Now().Unix()
	// alice has two orders, bob has one. TradeNo is globally unique.
	topups := []*TopUp{
		{UserId: alice.Id, Amount: 100, Money: 1.0, TradeNo: "ORDER-ALICE-1", Status: "success", CreateTime: now},
		{UserId: alice.Id, Amount: 200, Money: 2.0, TradeNo: "ORDER-ALICE-2", Status: "success", CreateTime: now},
		{UserId: bob.Id, Amount: 300, Money: 3.0, TradeNo: "ORDER-BOB-1", Status: "success", CreateTime: now},
	}
	for _, tu := range topups {
		require.NoError(t, DB.Create(tu).Error)
	}

	page := &common.PageInfo{Page: 1, PageSize: 100}

	t.Run("keyword matches a username returns that user's orders only", func(t *testing.T) {
		results, total, err := SearchAllTopUps("alice", page)
		require.NoError(t, err)
		assert.Equal(t, int64(2), total, "alice has two orders")
		require.Len(t, results, 2)
		for _, r := range results {
			assert.Equal(t, alice.Id, r.UserId)
			assert.Equal(t, "alice_pay", r.Username, "username should be back-filled")
		}
	})

	t.Run("keyword matches an order number still works", func(t *testing.T) {
		results, total, err := SearchAllTopUps("ORDER-BOB-1", page)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, results, 1)
		assert.Equal(t, "ORDER-BOB-1", results[0].TradeNo)
		assert.Equal(t, bob.Id, results[0].UserId)
	})

	t.Run("partial order-number fragment matches (fuzzy, not exact)", func(t *testing.T) {
		// "ORDER-ALICE" is a prefix, not a full trade_no; fuzzy %...% must still hit both.
		results, total, err := SearchAllTopUps("ORDER-ALICE", page)
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		require.Len(t, results, 2)
		for _, r := range results {
			assert.Equal(t, alice.Id, r.UserId)
		}
	})

	t.Run("keyword matches both a username and another order's trade_no", func(t *testing.T) {
		// "pay" is a substring of every username (alice_pay/bob_pay) but no trade_no,
		// so it must return all three orders with no duplication.
		results, total, err := SearchAllTopUps("pay", page)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		require.Len(t, results, 3)
		ids := map[int]bool{}
		for _, r := range results {
			ids[r.Id] = true
		}
		assert.Len(t, ids, 3, "no duplicate rows")
	})

	t.Run("underscore in keyword is escaped, not treated as wildcard", func(t *testing.T) {
		// "b_b" must NOT match "bob_pay" via _ acting as a single-char wildcard.
		// It is escaped to a literal underscore, so it matches nothing here.
		results, total, err := SearchAllTopUps("b_b", page)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Empty(t, results)
	})

	t.Run("keyword matching neither returns empty", func(t *testing.T) {
		results, total, err := SearchAllTopUps("no_such_thing", page)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Empty(t, results)
	})

	t.Run("single-char keyword is rejected (guards against over-broad scan)", func(t *testing.T) {
		results, total, err := SearchAllTopUps("a", page)
		require.Error(t, err, "a 1-char fuzzy keyword must be rejected")
		assert.Zero(t, total)
		assert.Empty(t, results)
	})
}
