package service

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

//go:embed channel_concurrency_acquire.lua
var channelConcurrencyAcquireScript string

//go:embed channel_concurrency_release.lua
var channelConcurrencyReleaseScript string

var (
	concurrencyAcquireSHA string
	concurrencyReleaseSHA string
	concurrencyScriptOnce sync.Once
)

func initConcurrencyScripts() {
	if !common.RedisEnabled {
		return
	}
	ctx := context.Background()
	var err error
	concurrencyAcquireSHA, err = common.RDB.ScriptLoad(ctx, channelConcurrencyAcquireScript).Result()
	if err != nil {
		common.SysLog(fmt.Sprintf("Failed to load concurrency acquire script: %v", err))
	}
	concurrencyReleaseSHA, err = common.RDB.ScriptLoad(ctx, channelConcurrencyReleaseScript).Result()
	if err != nil {
		common.SysLog(fmt.Sprintf("Failed to load concurrency release script: %v", err))
	}
}

func channelConcurrencyKey(channelId int) string {
	return fmt.Sprintf("channel_concurrency:%d", channelId)
}

func userConcurrencyKey(userId int) string {
	return fmt.Sprintf("user_concurrency:%d", userId)
}

// --- Memory implementation ---

var memoryConcurrency sync.Map // map[string]*atomic.Int64

func getMemoryCounter(key string) *atomic.Int64 {
	val, ok := memoryConcurrency.Load(key)
	if ok {
		return val.(*atomic.Int64)
	}
	counter := &atomic.Int64{}
	actual, _ := memoryConcurrency.LoadOrStore(key, counter)
	return actual.(*atomic.Int64)
}

// tryAcquireConcurrency acquires one in-flight slot under key, bounded by
// maxConcurrency. maxConcurrency <= 0 means unlimited. Redis errors fail open.
func tryAcquireConcurrency(key string, maxConcurrency int) bool {
	if maxConcurrency <= 0 {
		return true
	}

	if common.RedisEnabled {
		concurrencyScriptOnce.Do(initConcurrencyScripts)
		ctx := context.Background()
		result, err := common.RDB.EvalSha(ctx, concurrencyAcquireSHA, []string{key}, maxConcurrency).Int()
		if err != nil {
			common.SysLog(fmt.Sprintf("concurrency acquire redis error (key=%s): %v, falling back to allow", key, err))
			return true // fail-open
		}
		return result == 1
	}

	counter := getMemoryCounter(key)
	for {
		current := counter.Load()
		if current >= int64(maxConcurrency) {
			return false
		}
		if counter.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

// releaseConcurrency releases one in-flight slot under key, never dropping
// the counter below zero.
func releaseConcurrency(key string) {
	if common.RedisEnabled {
		concurrencyScriptOnce.Do(initConcurrencyScripts)
		ctx := context.Background()
		_, err := common.RDB.EvalSha(ctx, concurrencyReleaseSHA, []string{key}).Result()
		if err != nil {
			common.SysLog(fmt.Sprintf("concurrency release redis error (key=%s): %v", key, err))
		}
		return
	}

	counter := getMemoryCounter(key)
	for {
		current := counter.Load()
		if current <= 0 {
			return
		}
		if counter.CompareAndSwap(current, current-1) {
			return
		}
	}
}

// TryAcquireChannelConcurrency tries to acquire a concurrency slot for the channel.
// Returns true if acquired, false if the channel is at max concurrency.
func TryAcquireChannelConcurrency(channelId int, maxConcurrency int) bool {
	return tryAcquireConcurrency(channelConcurrencyKey(channelId), maxConcurrency)
}

// ReleaseChannelConcurrency releases a concurrency slot for the channel.
func ReleaseChannelConcurrency(channelId int) {
	releaseConcurrency(channelConcurrencyKey(channelId))
}

// TryAcquireUserConcurrency tries to acquire an in-flight request slot for the user.
func TryAcquireUserConcurrency(userId int, maxConcurrency int) bool {
	return tryAcquireConcurrency(userConcurrencyKey(userId), maxConcurrency)
}

// ReleaseUserConcurrency releases an in-flight request slot for the user.
func ReleaseUserConcurrency(userId int) {
	releaseConcurrency(userConcurrencyKey(userId))
}

// IsChannelConcurrencyAvailable checks if a channel has available concurrency slots
// without acquiring one. Used during channel selection to filter out full channels.
func IsChannelConcurrencyAvailable(channelId int, maxConcurrency int) bool {
	if maxConcurrency <= 0 {
		return true
	}

	if common.RedisEnabled {
		concurrencyScriptOnce.Do(initConcurrencyScripts)
		ctx := context.Background()
		result, err := common.RDB.Get(ctx, channelConcurrencyKey(channelId)).Int()
		if err != nil {
			// Key doesn't exist or error — treat as available
			return true
		}
		return result < maxConcurrency
	}

	counter := getMemoryCounter(channelConcurrencyKey(channelId))
	return counter.Load() < int64(maxConcurrency)
}

// resetConcurrencyCountersForTest clears in-memory counters between tests.
func resetConcurrencyCountersForTest() {
	memoryConcurrency.Range(func(key, _ any) bool {
		memoryConcurrency.Delete(key)
		return true
	})
}

// resetConcurrencyScriptsForTest forces script reload against the current
// Redis client (tests swap in a fresh miniredis per test).
func resetConcurrencyScriptsForTest() {
	concurrencyScriptOnce = sync.Once{}
}
