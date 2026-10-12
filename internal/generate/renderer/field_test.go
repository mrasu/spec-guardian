package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewField_ValidPath(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantRole  string
		wantField string
	}{
		{name: "database field", value: "DB.records", wantRole: "DB", wantField: "records"},
		{name: "underscored field", value: "Cache.cached_records", wantRole: "Cache", wantField: "cached_records"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, err := NewField(tt.value)
			require.NoError(t, err)
			require.Equal(t, tt.wantRole, field.RoleName())
			require.Equal(t, tt.wantField, field.FieldName())
			require.Equal(t, tt.value, field.String())
		})
	}
}

func TestNewField_InvalidPath(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "missing separator", value: "DB"},
		{name: "missing role", value: ".records"},
		{name: "missing field", value: "DB."},
		{name: "nested field", value: "DB.records.extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, err := NewField(tt.value)
			require.ErrorContains(t, err, "invalid field path")
			require.Nil(t, field)
		})
	}
}
