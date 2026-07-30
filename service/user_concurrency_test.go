package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserConcurrencyMemoryLimit(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })

	const userId = 42
	t.Cleanup(func() { resetConcurrencyCountersForTest() })

	assert.True(t, TryAcquireUserConcurrency(userId, 2))
	assert.True(t, TryAcquireUserConcurrency(userId, 2))
	assert.False(t, TryAcquireUserConcurrency(userId, 2), "third acquire must be rejected at max 2")

	ReleaseUserConcurrency(userId)
	assert.True(t, TryAcquireUserConcurrency(userId, 2), "slot must be reusable after release")

	// Other users are unaffected.
	assert.True(t, TryAcquireUserConcurrency(userId+1, 1))
}

func TestUserConcurrencyMemoryReleaseFloor(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })

	const userId = 43
	t.Cleanup(func() { resetConcurrencyCountersForTest() })

	ReleaseUserConcurrency(userId)
	ReleaseUserConcurrency(userId)
	// Counter must not go negative: max 1 still admits exactly one request.
	assert.True(t, TryAcquireUserConcurrency(userId, 1))
	assert.False(t, TryAcquireUserConcurrency(userId, 1))
}

func TestUserConcurrencyUnlimited(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	t.Cleanup(func() { resetConcurrencyCountersForTest() })

	for range 5 {
		assert.True(t, TryAcquireUserConcurrency(44, 0), "0 means unlimited")
		assert.True(t, TryAcquireUserConcurrency(44, -1), "negative means unlimited")
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
		resetConcurrencyScriptsForTest()
	})
	resetConcurrencyScriptsForTest()

	const userId = 45
	require.True(t, TryAcquireUserConcurrency(userId, 2))
	require.True(t, TryAcquireUserConcurrency(userId, 2))
	assert.False(t, TryAcquireUserConcurrency(userId, 2))

	ReleaseUserConcurrency(userId)
	assert.True(t, TryAcquireUserConcurrency(userId, 2))

	// Release below zero must not unlock more than max slots.
	ReleaseUserConcurrency(userId)
	ReleaseUserConcurrency(userId)
	ReleaseUserConcurrency(userId)
	require.True(t, TryAcquireUserConcurrency(userId, 1))
	assert.False(t, TryAcquireUserConcurrency(userId, 1))
}
