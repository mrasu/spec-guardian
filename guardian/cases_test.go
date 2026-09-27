package guardian

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCases(t *testing.T) {
	type testCase struct {
		Name   string         `json:"name"`
		Value  int            `json:"value"`
		Nested map[string]any `json:"nested"`
	}

	path := filepath.Join(t.TempDir(), "cases.json")
	require.NoError(t, os.WriteFile(path, []byte(`[{"name":"first","value":1,"nested":{"integer":2,"float":2.5,"items":[3,3.5]}}]`), 0o600))

	cases := LoadCases[testCase](t, path)
	assert.Equal(t, []testCase{{
		Name:  "first",
		Value: 1,
		Nested: map[string]any{
			"integer": float64(2),
			"float":   2.5,
			"items":   []any{float64(3), 3.5},
		},
	}}, cases)
}
