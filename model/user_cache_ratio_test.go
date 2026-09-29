package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The per-user billing ratio must survive the Redis user cache round trip.
// A cache hit that silently drops Ratio makes billing flap between
// groupRatio and groupRatio*userRatio depending on cache hit/miss.
func TestUserCacheRoundTripsPerUserRatio(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	ratio := 0.7
	user := User{
		Username:    "ratio-cache-roundtrip",
		Password:    "password",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		Ratio:       &ratio,
		AuthVersion: 1,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, populateUserCache(user))

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	require.NotNil(t, cached.Ratio, "Ratio must be stored in the Redis user hash")
	assert.InDelta(t, 0.7, *cached.Ratio, 1e-9)
	assert.InDelta(t, 0.7, cached.GetRatio(), 1e-9)
}

// A user without a per-user ratio must stay neutral (1.0) after a cache hit.
func TestUserCacheRoundTripsNilRatioAsNeutral(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	user := User{
		Username:    "ratio-cache-nil",
		Password:    "password",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, populateUserCache(user))

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Nil(t, cached.Ratio)
	assert.InDelta(t, 1.0, cached.GetRatio(), 1e-9)
}

// Per-user group ratios must survive the Redis user cache round trip and be
// refreshed by an administrator edit, otherwise billing would keep charging
// the regular group ratio until the cached hash expires.
func TestUserCacheRoundTripsGroupRatiosAcrossEdits(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	initial := `{"vip":0.5}`
	user := User{
		Username:    "group-ratio-cache",
		Password:    "password",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		GroupRatios: &initial,
		AuthVersion: 1,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, populateUserCache(user))

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"vip": 0.5}, cached.GetGroupRatios())

	// Omitting group_ratios in an edit keeps the stored overrides.
	edit := User{Id: user.Id, Username: user.Username, Group: "default"}
	require.NoError(t, edit.Edit(false))
	cached, err = cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"vip": 0.5}, cached.GetGroupRatios())

	updated := `{"default":0,"vip":0.8}`
	edit = User{Id: user.Id, Username: user.Username, Group: "default", GroupRatios: &updated}
	require.NoError(t, edit.Edit(false))
	cached, err = cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"default": 0, "vip": 0.8}, cached.GetGroupRatios())

	cleared := ""
	edit = User{Id: user.Id, Username: user.Username, Group: "default", GroupRatios: &cleared}
	require.NoError(t, edit.Edit(false))
	cached, err = cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Empty(t, cached.GetGroupRatios())

	stored, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "", stored.GetGroupRatiosJSON())
}

// A stored value that no longer parses must not break authentication or
// billing; it is ignored so the regular group ratios apply.
func TestUserGroupRatiosIgnoreInvalidStoredValue(t *testing.T) {
	for _, raw := range []string{`not-json`, `{"vip":-1}`, `{"vip":null}`, `{"vip":101}`} {
		assert.Nil(t, (&UserBase{GroupRatios: raw}).GetGroupRatios(), raw)
	}
}
