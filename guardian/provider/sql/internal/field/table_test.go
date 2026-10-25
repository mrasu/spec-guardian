package field

import (
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestNewTableField_ValidatesShapeAndMapping(t *testing.T) {
	table := Table{Name: "records", Columns: []Column{{Name: "key", Kind: provider.KindString}, {Name: "value", Kind: provider.KindString}}}
	spec := provider.Type{Kind: provider.KindList, Values: &provider.Type{Kind: provider.KindMessage, Fields: map[string]provider.Type{"key": {Kind: provider.KindString}, "value": {Kind: provider.KindString}}}}
	tests := []struct {
		name   string
		config Config
		spec   provider.Type
		want   string
	}{
		{"unknown table", Config{Table: "missing"}, spec, "unknown SQL table"},
		{"wrong container", Config{Table: "records"}, provider.Type{Kind: provider.KindMap}, "list"},
		{"wrong column type", Config{Table: "records"}, provider.Type{Kind: provider.KindList, Values: &provider.Type{Kind: provider.KindMessage, Fields: map[string]provider.Type{"key": {Kind: provider.KindInt64}, "value": {Kind: provider.KindString}}}}, "matching message field"},
		{"missing column field", Config{Table: "records"}, provider.Type{Kind: provider.KindList, Values: &provider.Type{Kind: provider.KindMessage, Fields: map[string]provider.Type{"key": {Kind: provider.KindString}}}}, "matching message field"},
		{"extra message field", Config{Table: "records"}, provider.Type{Kind: provider.KindList, Values: &provider.Type{Kind: provider.KindMessage, Fields: map[string]provider.Type{"key": {Kind: provider.KindString}, "value": {Kind: provider.KindString}, "extra": {Kind: provider.KindString}}}}, "must match"},
		{"unknown mapping field", Config{Table: "records", Columns: []ColumnMapping{{Field: "missing", Column: "key"}}}, spec, "invalid SQL column mapping"},
		{"unknown mapping column", Config{Table: "records", Columns: []ColumnMapping{{Field: "key", Column: "missing"}}}, spec, "matching message field"},
		{"duplicate mapping", Config{Table: "records", Columns: []ColumnMapping{{Field: "key", Column: "key"}, {Field: "key", Column: "value"}}}, spec, "invalid SQL column mapping"},
		{"colliding columns", Config{Table: "records", Columns: []ColumnMapping{{Field: "value", Column: "key"}}}, spec, "multiple fields"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := NewTableField(Request{Field: "DB.records", Config: tt.config, Output: provider.OutputSpec{Type: tt.spec}}, map[string]Table{"records": table}, "pgx")
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, query)
		})
	}
}

func TestResolveTables_RejectsDuplicateNames(t *testing.T) {
	tests := []struct {
		name   string
		tables []Table
		want   string
	}{
		{"duplicate table", []Table{{Name: "records", Columns: []Column{{Name: "key", Kind: provider.KindString}}}, {Name: "records", Columns: []Column{{Name: "key", Kind: provider.KindString}}}}, "duplicate SQL table"},
		{"duplicate column", []Table{{Name: "records", Columns: []Column{{Name: "key", Kind: provider.KindString}, {Name: "key", Kind: provider.KindInt64}}}}, "duplicate column"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables, err := ResolveTables(tt.tables)
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, tables)
		})
	}
}

func TestQuoteIdentifier_EscapesDriverQuotes(t *testing.T) {
	tests := []struct{ driver, name, want string }{
		{"pgx", `a"b`, `"a""b"`},
		{"mysql", "a`b", "`a``b`"},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			require.Equal(t, tt.want, quoteIdentifier(tt.name, tt.driver))
		})
	}
}
