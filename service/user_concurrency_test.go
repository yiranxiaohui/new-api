package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useMemoryConcurrency(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		resetConcurrencyCountersForTest()
	})
}

func TestUserConcurrencyMemoryLimit(t *testing.T) {
	useMemoryConcurrency(t)
	const userId = 42

	allowed1, release1 := TryAcquireUserConcurrency(userId, 2)
	require.True(t, allowed1)
	require.NotNil(t, release1)
	allowed2, _ := TryAcquireUserConcurrency(userId, 2)
	require.True(t, allowed2)

	allowed3, release3 := TryAcquireUserConcurrency(userId, 2)
	assert.False(t, allowed3, "third acquire must be rejected at max 2")
	assert.Nil(t, release3)

	release1()
	allowed4, _ := TryAcquireUserConcurrency(userId, 2)
	assert.True(t, allowed4, "slot must be reusable after release")

	// Other users are unaffected.
	otherAllowed, _ := TryAcquireUserConcurrency(userId+1, 1)
	assert.True(t, otherAllowed)
}

func TestUserConcurrencyReleaseIsIdempotent(t *testing.T) {
	useMemoryConcurrency(t)
	const userId = 43

	allowed, release := TryAcquireUserConcurrency(userId, 1)
	require.True(t, allowed)
	require.NotNil(t, release)

	// Double release must free exactly one slot, not drive the counter
	// negative and unlock extra capacity.
	release()
	release()
	againAllowed, _ := TryAcquireUserConcurrency(userId, 1)
	require.True(t, againAllowed)
	overAllowed, _ := TryAcquireUserConcurrency(userId, 1)
	assert.False(t, overAllowed)
}

func TestConcurrencyReleaseFloor(t *testing.T) {
	useMemoryConcurrency(t)
	const key = "user_concurrency:test-floor"

	// Releases without a matching acquire must not go negative.
	releaseConcurrency(key)
	releaseConcurrency(key)
	allowed, acquired := tryAcquireConcurrency(key, 1)
	require.True(t, allowed)
	require.True(t, acquired)
	overAllowed, _ := tryAcquireConcurrency(key, 1)
	assert.False(t, overAllowed)
}

func TestUserConcurrencyUnlimited(t *testing.T) {
	useMemoryConcurrency(t)

	for range 5 {
		allowed, release := TryAcquireUserConcurrency(44, 0)
		assert.True(t, allowed, "0 means unlimited")
		assert.Nil(t, release, "unlimited acquire must not hold a slot")
		negAllowed, _ := TryAcquireUserConcurrency(44, -1)
		assert.True(t, negAllowed, "negative means unlimited")
	}
}

func TestUserConcurrencyRedisLimit(t *testing.T) {
	server := miniredis.RunT(t)
	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
	})

	const userId = 45
	allowed1, release1 := TryAcquireUserConcurrency(userId, 2)
	require.True(t, allowed1)
	require.NotNil(t, release1)
	allowed2, _ := TryAcquireUserConcurrency(userId, 2)
	require.True(t, allowed2)
	allowed3, _ := TryAcquireUserConcurrency(userId, 2)
	assert.False(t, allowed3)

	release1()
	allowed4, _ := TryAcquireUserConcurrency(userId, 2)
	assert.True(t, allowed4)

	// Double release of the same handle must not unlock extra slots.
	release1()
	release1()
	allowed5, _ := TryAcquireUserConcurrency(userId, 2)
	assert.False(t, allowed5, "counter already at max 2; idempotent release must not free more")
}
