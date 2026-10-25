package sql

import (
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mrasu/spec-guardian/guardian/gcmp"
	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestProvider_ObserveTablePreservesRowsAndDetectsStateDifferences(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://postgres@localhost:15432/postgres?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(t.Context(), `CREATE TEMP TABLE "sg table" (record_key TEXT, value TEXT, unobserved TEXT)`)
	require.NoError(t, err)
	config := map[string]any{"tables": []any{map[string]any{"name": "sg table", "columns": []any{
		map[string]any{"name": "record_key", "type": "string"}, map[string]any{"name": "value", "type": "string"},
	}}}}
	request := provider.FieldRequest{
		Field:  "DB.records",
		Config: map[string]any{"table": "sg table", "columns": []any{map[string]any{"field": "key", "column": "record_key"}}},
		Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindList, Values: &provider.Type{Kind: provider.KindMessage, Fields: map[string]provider.Type{
			"key": {Kind: provider.KindString}, "value": {Kind: provider.KindString},
		}}}},
	}
	instance, err := New(Config{Database: db, Driver: "pgx"}, config, []provider.FieldRequest{request})
	require.NoError(t, err)
	type record struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	type state struct {
		Records *gcmp.ComparisonList[*gcmp.ComparisonField[record]]
	}
	allowed := &state{Records: gcmp.ListUnordered([]*gcmp.ComparisonField[record]{
		gcmp.Equal(record{Key: "a", Value: "one"}), gcmp.Equal(record{Key: "b", Value: "two"}), gcmp.Equal(record{Key: "a", Value: "one"}),
	})}
	tests := []struct {
		name    string
		rows    []record
		matches bool
	}{
		{"different order with duplicates", []record{{"b", "two"}, {"a", "one"}, {"a", "one"}}, true},
		{"extra row", []record{{"a", "one"}, {"b", "two"}, {"a", "one"}, {"c", "three"}}, false},
		{"missing duplicate", []record{{"a", "one"}, {"b", "two"}}, false},
		{"different duplicate count", []record{{"a", "one"}, {"b", "two"}, {"b", "two"}}, false},
		{"changed value", []record{{"a", "changed"}, {"b", "two"}, {"a", "one"}}, false},
		{"empty table", []record{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.ExecContext(t.Context(), `DELETE FROM "sg table"`)
			require.NoError(t, err)
			for _, row := range tt.rows {
				_, err := db.ExecContext(t.Context(), `INSERT INTO "sg table" VALUES ($1, $2, 'ignored')`, row.Key, row.Value)
				require.NoError(t, err)
			}
			raw, err := instance.Observe(t.Context(), "DB.records")
			require.NoError(t, err)
			rows, err := provider.Convert[[]record](raw)
			require.NoError(t, err)
			require.NotNil(t, rows)
			require.ElementsMatch(t, tt.rows, rows)
			observedValues := make([]*gcmp.ObservedField[record], len(rows))
			for i, row := range rows {
				observedValues[i] = gcmp.Observed(row)
			}
			observed := &struct {
				Records *gcmp.ObservedList[*gcmp.ObservedField[record]]
			}{
				Records: gcmp.ListObserved(observedValues),
			}
			comparison := gcmp.NewObservedStateComparator[*state](observed)
			require.NoError(t, comparison.Validate())
			require.Equal(t, tt.matches, len(comparison.Match(allowed)) == 0)
		})
	}
}

func TestProvider_ObserveTableScalarTypesAndNulls(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://postgres@localhost:15432/postgres?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(t.Context(), `CREATE TEMP TABLE sg_types (id BIGINT, enabled BOOLEAN, score DOUBLE PRECISION, payload BYTEA, note TEXT)`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO sg_types VALUES (9007199254740993, true, 1.5, '\x0102', NULL)`)
	require.NoError(t, err)
	columns := []any{
		map[string]any{"name": "id", "type": "int64"}, map[string]any{"name": "enabled", "type": "bool"},
		map[string]any{"name": "score", "type": "double"}, map[string]any{"name": "payload", "type": "bytes"}, map[string]any{"name": "note", "type": "string"},
	}
	fields := map[string]provider.Type{"id": {Kind: provider.KindInt64}, "enabled": {Kind: provider.KindBool}, "score": {Kind: provider.KindDouble}, "payload": {Kind: provider.KindBytes}, "note": {Kind: provider.KindString, Nullable: true}}
	config := map[string]any{"tables": []any{map[string]any{"name": "sg_types", "columns": columns}}}
	request := provider.FieldRequest{Field: "DB.types", Config: map[string]any{"table": "sg_types"}, Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindList, Values: &provider.Type{Kind: provider.KindMessage, Fields: fields}}}}
	instance, err := New(Config{Database: db, Driver: "pgx"}, config, []provider.FieldRequest{request})
	require.NoError(t, err)
	raw, err := instance.Observe(t.Context(), "DB.types")
	require.NoError(t, err)
	require.Equal(t, []map[string]any{{"id": int64(9007199254740993), "enabled": true, "score": 1.5, "payload": []byte{1, 2}, "note": nil}}, raw)
	fields["note"] = provider.Type{Kind: provider.KindString}
	strict, err := New(Config{Database: db, Driver: "pgx"}, config, []provider.FieldRequest{request})
	require.NoError(t, err)
	// The original provider keeps its validated nullable declaration.
	_, err = instance.Observe(t.Context(), "DB.types")
	require.NoError(t, err)
	raw, err = strict.Observe(t.Context(), "DB.types")
	require.ErrorContains(t, err, "unexpected SQL null")
	require.Nil(t, raw)
}

func TestProvider_ObserveQueryRemainsAvailable(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://postgres@localhost:15432/postgres?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	request := provider.FieldRequest{Field: "DB.query", Config: map[string]any{
		"query": "SELECT 'a' AS key, 'one' AS value", "result": map[string]any{"type": "kv", "key_column": "key", "value_column": "value"},
	}, Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindMap, Values: &provider.Type{Kind: provider.KindString}}}}
	instance, err := New(Config{Database: db, Driver: "pgx"}, nil, []provider.FieldRequest{request})
	require.NoError(t, err)
	value, err := instance.Observe(t.Context(), request.Field)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"a": "one"}, value)
}
