package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
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
		userGroupRatios  map[string]float64
		wantGroupRatio   float64
		wantSpecial      bool
		wantSpecialRatio float64
	}{
		{"user ratio 1 keeps group ratio", "default", "vip", 1.0, nil, 1.5, false, -1},
		{"user ratio multiplies group ratio", "default", "vip", 0.8, nil, 1.2, false, -1},
		{"zero-value user ratio treated as 1", "default", "vip", 0, nil, 1.5, false, -1},
		{"user ratio multiplies group-group special ratio", "svip", "vip", 0.8, nil, 0.4, true, 0.4},
		{"user group ratio is final and ignores user ratio", "default", "vip", 0.8, map[string]float64{"vip": 0.7}, 0.7, true, 0.7},
		{"user group ratio overrides group-group special ratio", "svip", "vip", 1, map[string]float64{"vip": 2}, 2, true, 2},
		{"zero user group ratio makes the group free", "default", "vip", 1, map[string]float64{"vip": 0}, 0, true, 0},
		{"user group ratio of another group does not apply", "default", "vip", 0.8, map[string]float64{"default": 0.1}, 1.2, false, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				UserGroup:       tt.userGroup,
				UsingGroup:      tt.usingGroup,
				UserRatio:       tt.userRatio,
				UserGroupRatios: tt.userGroupRatios,
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

func TestHandleGroupRatioAppliesUserGroupRatioToAutoGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.5}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set("auto_group", "vip")
	info := &relaycommon.RelayInfo{
		UserGroup:       "default",
		UsingGroup:      "auto",
		UserRatio:       1,
		UserGroupRatios: map[string]float64{"vip": 0.6},
	}

	got := HandleGroupRatio(c, info)

	assert.Equal(t, "vip", info.UsingGroup)
	assert.InDelta(t, 0.6, got.GroupRatio, 1e-9)
}

// The authenticated user's cached group ratios must reach the relay billing
// path: user cache -> request context -> RelayInfo -> HandleGroupRatio.
func TestHandleGroupRatioUsesGroupRatiosFromUserContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.5}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	userRatio := 0.8
	(&model.UserBase{Id: 1, Group: "default", Ratio: &userRatio, GroupRatios: `{"vip":0.6}`}).WriteContext(c)
	c.Set(string(constant.ContextKeyUsingGroup), "vip")

	info := relaycommon.GenRelayInfoOpenAI(c, &dto.GeneralOpenAIRequest{Model: "gpt-test"})
	got := HandleGroupRatio(c, info)

	assert.InDelta(t, 0.6, got.GroupRatio, 1e-9)
	assert.True(t, got.HasSpecialRatio)

	info.UsingGroup = "default"
	assert.InDelta(t, 0.8, HandleGroupRatio(c, info).GroupRatio, 1e-9, "other groups keep group ratio x user ratio")
}

// TestDisplayedGroupRatioMatchesBilledRatio pins the key-management/pricing
// display path (service.GetUserEffectiveGroupRatio) to the billing path, so an
// administrator changing a user ratio or a per-user group ratio sees the ratio
// the user is actually charged.
func TestDisplayedGroupRatioMatchesBilledRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.5}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"svip":{"vip":0.5}}`))

	tests := []struct {
		name            string
		userGroup       string
		group           string
		userRatio       float64
		userGroupRatios map[string]float64
		want            float64
	}{
		{"neutral user ratio", "default", "vip", 1, nil, 1.5},
		{"discounted user ratio", "default", "vip", 0.8, nil, 1.2},
		{"unset user ratio", "default", "vip", 0, nil, 1.5},
		{"group-group special ratio", "svip", "vip", 0.8, nil, 0.4},
		{"user group ratio", "svip", "vip", 0.8, map[string]float64{"vip": 0.75}, 0.75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			displayed := service.GetUserEffectiveGroupRatio(tt.userGroup, tt.group, tt.userRatio, tt.userGroupRatios)
			assert.InDelta(t, tt.want, displayed, 1e-9)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			billed := HandleGroupRatio(c, &relaycommon.RelayInfo{
				UserGroup:       tt.userGroup,
				UsingGroup:      tt.group,
				UserRatio:       tt.userRatio,
				UserGroupRatios: tt.userGroupRatios,
			})
			assert.InDelta(t, billed.GroupRatio, displayed, 1e-9)
		})
	}
}
