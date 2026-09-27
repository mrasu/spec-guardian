package guardian

import (
	"encoding/json"
	"os"
	"testing"
)

// LoadCases reads a JSON array of test cases from path.
func LoadCases[Case any](t testing.TB, path string) []Case {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test cases %q: %v", path, err)
	}

	var cases []Case
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decode test cases %q: %v", path, err)
	}
	return cases
}
