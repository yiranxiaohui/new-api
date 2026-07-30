package service

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/go-redis/redis/v8"
)

//go:embed channel_concurrency_acquire.lua
var channelConcurrencyAcquireScript string

//go:embed channel_concurrency_release.lua
var channelConcurrencyReleaseScript string

// redis.Script.Run retries with EVAL on NOSCRIPT, so a Redis restart or
// failover that flushes the script cache self-heals instead of permanently
// failing every acquire/release open.
var (
	concurrencyAcquireScript = redis.NewScript(channelConcurrencyAcquireScript)
	concurrencyReleaseScript = redis.NewScript(channelConcurrencyReleaseScript)
)

// concurrencySlotTTL mirrors the EXPIRE hardcoded in the acquire Lua script.
// Held slots refresh it periodically so long-lived streams do not outlive the
// key (which would reset the counter and void the limit).
const (
	concurrencySlotTTL            = 300 * time.Second
	concurrencyTTLRefreshInterval = 100 * time.Second
)

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

// tryAcquireConcurrency reports whether the request may proceed (allowed) and
// whether a slot was actually taken (acquired). The two differ on the
// unlimited and Redis-fail-open paths, where the request proceeds without
// holding a slot; releasing in those cases would decrement a slot owned by
// another in-flight request.
func tryAcquireConcurrency(key string, maxConcurrency int) (allowed bool, acquired bool) {
	if maxConcurrency <= 0 {
		return true, false
	}

	if common.RedisEnabled {
		ctx := context.Background()
		result, err := concurrencyAcquireScript.Run(ctx, common.RDB, []string{key}, maxConcurrency).Int()
		if err != nil {
			common.SysLog(fmt.Sprintf("concurrency acquire redis error (key=%s): %v, falling back to allow", key, err))
			return true, false // fail-open without holding a slot
		}
		return result == 1, result == 1
	}

	counter := getMemoryCounter(key)
	for {
		current := counter.Load()
		if current >= int64(maxConcurrency) {
			return false, false
		}
		if counter.CompareAndSwap(current, current+1) {
			return true, true
		}
	}
}

// releaseConcurrency releases one in-flight slot under key, never dropping
// the counter below zero.
func releaseConcurrency(key string) {
	if common.RedisEnabled {
		ctx := context.Background()
		if err := concurrencyReleaseScript.Run(ctx, common.RDB, []string{key}).Err(); err != nil {
			common.SysLog(fmt.Sprintf("concurrency release redis error (key=%s): %v", key, err))
		}
		return
	}

	val, ok := memoryConcurrency.Load(key)
	if !ok {
		return
	}
	counter := val.(*atomic.Int64)
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

func refreshConcurrencyTTL(key string) {
	if !common.RedisEnabled {
		return
	}
	if err := common.RDB.Expire(context.Background(), key, concurrencySlotTTL).Err(); err != nil {
		common.SysLog(fmt.Sprintf("concurrency ttl refresh redis error (key=%s): %v", key, err))
	}
}

// acquireConcurrencySlot returns whether the request may proceed and, when a
// slot was actually taken, an idempotent release handle. While the slot is
// held, the Redis key TTL is refreshed periodically so requests longer than
// the safety-net TTL (long streams, realtime WebSocket sessions) keep their
// slot accounted. release is nil when no slot was taken (unlimited limit,
// Redis fail-open, or rejection).
func acquireConcurrencySlot(key string, maxConcurrency int) (bool, func()) {
	allowed, acquired := tryAcquireConcurrency(key, maxConcurrency)
	if !allowed {
		return false, nil
	}
	if !acquired {
		return true, nil
	}

	stop := make(chan struct{})
	if common.RedisEnabled {
		go func() {
			ticker := time.NewTicker(concurrencyTTLRefreshInterval)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					refreshConcurrencyTTL(key)
				}
			}
		}()
	}

	var once sync.Once
	return true, func() {
		once.Do(func() {
			close(stop)
			releaseConcurrency(key)
		})
	}
}

// TryAcquireChannelConcurrency tries to acquire a concurrency slot for the
// channel. It returns whether the request may proceed and a release handle
// (nil when no slot was taken).
func TryAcquireChannelConcurrency(channelId int, maxConcurrency int) (bool, func()) {
	return acquireConcurrencySlot(channelConcurrencyKey(channelId), maxConcurrency)
}

// TryAcquireUserConcurrency tries to acquire an in-flight request slot for
// the user. It returns whether the request may proceed and a release handle
// (nil when no slot was taken).
func TryAcquireUserConcurrency(userId int, maxConcurrency int) (bool, func()) {
	return acquireConcurrencySlot(userConcurrencyKey(userId), maxConcurrency)
}

// IsChannelConcurrencyAvailable checks if a channel has available concurrency slots
// without acquiring one. Used during channel selection to filter out full channels.
func IsChannelConcurrencyAvailable(channelId int, maxConcurrency int) bool {
	if maxConcurrency <= 0 {
		return true
	}

	if common.RedisEnabled {
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
