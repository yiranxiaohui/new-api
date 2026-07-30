package middleware

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
)

// UserConcurrencyLimit caps a user's in-flight relay requests. The effective
// limit is the per-user override when set (users.max_concurrency: -1 means
// unlimited, >0 overrides), otherwise the global setting.UserMaxConcurrency
// (0 disables). The slot is held until the handler returns, so streaming
// responses count for their full duration.
func UserConcurrencyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := setting.UserMaxConcurrency
		if override, ok := common.GetContextKeyType[int](c, constant.ContextKeyUserMaxConcurrency); ok && override != 0 {
			limit = override
		}
		if limit <= 0 {
			c.Next()
			return
		}

		userId := c.GetInt("id")
		if userId <= 0 {
			c.Next()
			return
		}

		if !service.TryAcquireUserConcurrency(userId, limit) {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests,
				fmt.Sprintf("您已达到并发请求上限：最多 %d 个进行中的请求，请稍后重试", limit))
			return
		}
		defer service.ReleaseUserConcurrency(userId)
		c.Next()
	}
}
