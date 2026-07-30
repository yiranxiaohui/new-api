package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The per-user concurrency override must survive the Redis user cache round
// trip, including the explicit -1 (unlimited) sentinel. A dropped field would
// make the limit flap between the override and the global default depending
// on cache hit/miss (same failure mode as the per-user ratio regression).
func TestUserCacheRoundTripsMaxConcurrency(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	tests := []struct {
		name  string
		value *int
	}{
		{name: "override", value: intPtr(5)},
		{name: "unlimited", value: intPtr(-1)},
		{name: "follow-global", value: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := User{
				Username:       "conc-cache-" + tt.name,
				Password:       "password",
				Role:           common.RoleCommonUser,
				Status:         common.UserStatusEnabled,
				Group:          "default",
				AffCode:        "conc-" + tt.name,
				MaxConcurrency: tt.value,
				AuthVersion:    1,
			}
			require.NoError(t, DB.Create(&user).Error)
			require.NoError(t, populateUserCache(user))

			cached, err := cacheGetUserBase(user.Id)
			require.NoError(t, err)
			if tt.value == nil {
				assert.Nil(t, cached.MaxConcurrency)
			} else {
				require.NotNil(t, cached.MaxConcurrency)
				assert.Equal(t, *tt.value, *cached.MaxConcurrency)
			}
		})
	}
}

func intPtr(v int) *int {
	return &v
}
