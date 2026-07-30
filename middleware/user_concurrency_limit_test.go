package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func runUserConcurrencyRequest(t *testing.T, userId int, override int) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", userId)
	common.SetContextKey(c, constant.ContextKeyUserMaxConcurrency, override)

	UserConcurrencyLimit()(c)
	if !c.IsAborted() {
		c.Status(http.StatusOK)
	}
	return recorder.Code
}

func withUserConcurrencyFixture(t *testing.T, globalLimit int) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	oldGlobal := setting.UserMaxConcurrency
	common.RedisEnabled = false
	setting.UserMaxConcurrency = globalLimit
	// Tests use distinct user IDs, so leaked in-memory counters cannot
	// interfere across tests.
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		setting.UserMaxConcurrency = oldGlobal
	})
}

func TestUserConcurrencyLimitDisabledByDefault(t *testing.T) {
	withUserConcurrencyFixture(t, 0)
	for range 3 {
		assert.Equal(t, http.StatusOK, runUserConcurrencyRequest(t, 1, 0))
	}
}

func TestUserConcurrencyLimitRejectsAtGlobalCap(t *testing.T) {
	withUserConcurrencyFixture(t, 2)
	const userId = 7
	// Two requests already in flight.
	assert.True(t, service.TryAcquireUserConcurrency(userId, 2))
	assert.True(t, service.TryAcquireUserConcurrency(userId, 2))

	assert.Equal(t, http.StatusTooManyRequests, runUserConcurrencyRequest(t, userId, 0))

	// Another user is unaffected.
	assert.Equal(t, http.StatusOK, runUserConcurrencyRequest(t, userId+1, 0))
}

func TestUserConcurrencyLimitPerUserOverride(t *testing.T) {
	withUserConcurrencyFixture(t, 5)
	const userId = 8
	// Override tightens the limit to 1: one in-flight request blocks the next.
	assert.True(t, service.TryAcquireUserConcurrency(userId, 1))
	assert.Equal(t, http.StatusTooManyRequests, runUserConcurrencyRequest(t, userId, 1))
}

func TestUserConcurrencyLimitUnlimitedOverrideBypassesGlobal(t *testing.T) {
	withUserConcurrencyFixture(t, 1)
	const userId = 9
	assert.True(t, service.TryAcquireUserConcurrency(userId, 1))
	// -1 override: user ignores the global cap entirely.
	assert.Equal(t, http.StatusOK, runUserConcurrencyRequest(t, userId, -1))
}

func TestUserConcurrencyLimitReleasesSlotAfterRequest(t *testing.T) {
	withUserConcurrencyFixture(t, 1)
	const userId = 10
	assert.Equal(t, http.StatusOK, runUserConcurrencyRequest(t, userId, 0))
	// The slot must be free again after the request completed.
	assert.Equal(t, http.StatusOK, runUserConcurrencyRequest(t, userId, 0))
}
