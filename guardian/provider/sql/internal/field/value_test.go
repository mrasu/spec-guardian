package field

import (
	"math"
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestToScalar_UnsignedSQLInteger(t *testing.T) {
	tests := []struct {
		name    string
		value   uint64
		want    int64
		wantErr bool
	}{
		{name: "zero", value: 0, want: 0},
		{name: "generated ID", value: 42, want: 42},
		{name: "largest int64", value: math.MaxInt64, want: math.MaxInt64},
		{name: "overflow", value: math.MaxInt64 + 1, wantErr: true},
		{name: "largest uint64", value: math.MaxUint64, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toScalar(tt.value, provider.Type{Kind: provider.KindInt64})
			if tt.wantErr {
				require.ErrorContains(t, err, "overflows int64")
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
