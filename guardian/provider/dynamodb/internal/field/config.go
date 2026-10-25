package field

import "github.com/mrasu/spec-guardian/guardian/provider"

// Request carries a field's schema-validated settings and model type.
type Request struct {
	Field  string
	Config Config
	Output provider.OutputSpec
}

// Config selects the table and item mapping.
type Config struct {
	Table  string       `json:"table"`
	Result ResultConfig `json:"result"`
}

// ResultConfig selects item_map or item_list.
type ResultConfig struct {
	Type ResultType `json:"type"`
}
