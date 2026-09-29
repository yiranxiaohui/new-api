package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUserGroupRatios(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    map[string]float64
		wantErr bool
	}{
		{name: "empty means no overrides", raw: "", want: nil},
		{name: "blank means no overrides", raw: "  ", want: nil},
		{name: "empty object", raw: `{}`, want: map[string]float64{}},
		{name: "valid ratios including free group", raw: `{"vip":0.8,"default":0}`, want: map[string]float64{"vip": 0.8, "default": 0}},
		{name: "upper bound accepted", raw: `{"vip":100}`, want: map[string]float64{"vip": 100}},
		{name: "negative ratio rejected", raw: `{"vip":-0.1}`, wantErr: true},
		{name: "ratio above bound rejected", raw: `{"vip":100.5}`, wantErr: true},
		{name: "null ratio rejected instead of becoming free", raw: `{"vip":null}`, wantErr: true},
		{name: "string ratio rejected", raw: `{"vip":"0.8"}`, wantErr: true},
		{name: "empty group name rejected", raw: `{"":1}`, wantErr: true},
		{name: "padded group name rejected", raw: `{" vip":1}`, wantErr: true},
		{name: "non-object rejected", raw: `[1]`, wantErr: true},
		{name: "malformed JSON rejected", raw: `{"vip":`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUserGroupRatios(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
