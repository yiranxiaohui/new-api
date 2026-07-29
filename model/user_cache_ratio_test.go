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
