package helper

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleGroupRatioAppliesUserRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.5}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"svip":{"vip":0.5}}`))

	tests := []struct {
		name             string
		userGroup        string
		usingGroup       string
		userRatio        float64
		wantGroupRatio   float64
		wantSpecial      bool
		wantSpecialRatio float64
	}{
		{"user ratio 1 keeps group ratio", "default", "vip", 1.0, 1.5, false, -1},
		{"user ratio multiplies group ratio", "default", "vip", 0.8, 1.2, false, -1},
		{"zero-value user ratio treated as 1", "default", "vip", 0, 1.5, false, -1},
		{"user ratio multiplies group-group special ratio", "svip", "vip", 0.8, 0.4, true, 0.4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				UserGroup:  tt.userGroup,
				UsingGroup: tt.usingGroup,
				UserRatio:  tt.userRatio,
			}
			got := HandleGroupRatio(c, info)
			assert.InDelta(t, tt.wantGroupRatio, got.GroupRatio, 1e-9)
			assert.Equal(t, tt.wantSpecial, got.HasSpecialRatio)
			if tt.wantSpecial {
				assert.InDelta(t, tt.wantSpecialRatio, got.GroupSpecialRatio, 1e-9)
			}
		})
	}
}

// TestDisplayedGroupRatioMatchesBilledRatio pins the key-management/pricing
// display path (service.ApplyUserRatio over service.GetUserGroupRatio) to the
// billing path, so an administrator changing a user ratio sees the ratio the
// user is actually charged.
func TestDisplayedGroupRatioMatchesBilledRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.5}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"svip":{"vip":0.5}}`))

	tests := []struct {
		name      string
		userGroup string
		group     string
		userRatio float64
		want      float64
	}{
		{"neutral user ratio", "default", "vip", 1, 1.5},
		{"discounted user ratio", "default", "vip", 0.8, 1.2},
		{"unset user ratio", "default", "vip", 0, 1.5},
		{"group-group special ratio", "svip", "vip", 0.8, 0.4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			displayed := service.ApplyUserRatio(
				service.GetUserGroupRatio(tt.userGroup, tt.group),
				tt.userRatio,
			)
			assert.InDelta(t, tt.want, displayed, 1e-9)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			billed := HandleGroupRatio(c, &relaycommon.RelayInfo{
				UserGroup:  tt.userGroup,
				UsingGroup: tt.group,
				UserRatio:  tt.userRatio,
			})
			assert.InDelta(t, billed.GroupRatio, displayed, 1e-9)
		})
	}
}
