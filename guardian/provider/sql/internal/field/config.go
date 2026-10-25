package field

import "github.com/mrasu/spec-guardian/guardian/provider"

// Request carries a field's schema-validated settings and model type.
type Request struct {
	Field  string
	Config Config
	Output provider.OutputSpec
}

// Config contains the selected table or query settings.
type Config struct {
	Table   string          `json:"table"`
	Columns []ColumnMapping `json:"columns"`
	Query   string          `json:"query"`
	Result  ResultConfig    `json:"result"`
}

// ResultConfig selects the query result mapping.
type ResultConfig struct {
	Type        ResultType `json:"type"`
	KeyColumn   string     `json:"key_column"`
	ValueColumn string     `json:"value_column"`
}
