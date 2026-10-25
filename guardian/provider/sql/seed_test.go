package sql

import (
	"context"
	"database/sql"
	"testing"

	"github.com/mrasu/spec-guardian/guardian/provider"
	"github.com/stretchr/testify/require"
)

func TestProvider_SeedTableIsolatesAttemptsAndPassesQueryValuesThrough(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://postgres@localhost:15432/postgres?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(t.Context(), `CREATE TEMP TABLE "seed table" (record_key TEXT CHECK (record_key <> 'fail'), value TEXT)`)
	require.NoError(t, err)
	config := map[string]any{"tables": []any{map[string]any{"name": "seed table", "columns": []any{
		map[string]any{"name": "record_key", "type": "string"}, map[string]any{"name": "value", "type": "string"},
	}}}}
	fields := []provider.FieldRequest{
		{Field: "DB.rows", Config: map[string]any{"table": "seed table", "columns": []any{map[string]any{"field": "key", "column": "record_key"}}}, Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindList, Values: &provider.Type{Kind: provider.KindMessage, Fields: map[string]provider.Type{"key": {Kind: provider.KindString}, "value": {Kind: provider.KindString}}}}}},
		{Field: "DB.query", Config: map[string]any{"query": `SELECT record_key AS key, value FROM "seed table"`, "result": map[string]any{"type": "kv", "key_column": "key", "value_column": "value"}}, Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindMap, Values: &provider.Type{Kind: provider.KindString}}}},
	}
	instance, err := New(Config{Database: db, Driver: "pgx"}, config, fields)
	require.NoError(t, err)
	type row struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	tests := []struct {
		name  string
		rows  []row
		fails bool
	}{
		{"first attempt", []row{{"a", "one"}, {"b", "two"}, {"a", "one"}}, false},
		{"next attempt", []row{{"c", "three"}}, false},
		{"empty input", []row{}, false},
		{"partial insert failure", []row{{"a", "one"}, {"fail", "two"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, instance.PrepareAttempt(t.Context()))
			request := provider.SeedRequest{Values: []provider.SeedValue{
				{Field: "DB.rows", Value: tt.rows},
				{Field: "DB.query", Value: map[string]string{"query": "passes through without writing"}},
			}}
			result, err := instance.Seed(t.Context(), request)
			if tt.fails {
				require.ErrorContains(t, err, "SQL seed DB.rows")
				require.Equal(t, provider.SeedResult{}, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, provider.SeedResult{Values: request.Values}, result)
				raw, err := instance.Observe(t.Context(), "DB.rows")
				require.NoError(t, err)
				actual, err := provider.Convert[[]row](raw)
				require.NoError(t, err)
				require.ElementsMatch(t, tt.rows, actual)
			}
			// Cleanup remains usable after a partially successful seed.
			require.NoError(t, instance.CleanupAttempt(context.Background()))
			var count int
			require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM "seed table"`).Scan(&count))
			require.Zero(t, count)
		})
	}
	request := provider.SeedRequest{Values: []provider.SeedValue{{Field: "DB.query", Value: map[string]string{}}, {Field: "DB.rows", Value: []row{}}}}
	result, err := instance.Seed(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, provider.SeedResult{Values: request.Values}, result)
	request.Values[1].Field = "unknown"
	result, err = instance.Seed(t.Context(), request)
	require.ErrorContains(t, err, "unknown or duplicate SQL seed field")
	require.Equal(t, provider.SeedResult{}, result)
	result, err = instance.Seed(t.Context(), provider.SeedRequest{})
	require.ErrorContains(t, err, "requires all 2 fields")
	require.Equal(t, provider.SeedResult{}, result)
}

func TestProvider_SeedTablePreservesScalarTypesAndNulls(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://postgres@localhost:15432/postgres?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(t.Context(), `CREATE TEMP TABLE seed_types (id BIGINT, enabled BOOLEAN, score DOUBLE PRECISION, payload BYTEA, note TEXT)`)
	require.NoError(t, err)
	columns := []any{
		map[string]any{"name": "id", "type": "int64"}, map[string]any{"name": "enabled", "type": "bool"},
		map[string]any{"name": "score", "type": "double"}, map[string]any{"name": "payload", "type": "bytes"}, map[string]any{"name": "note", "type": "string"},
	}
	fields := map[string]provider.Type{"id": {Kind: provider.KindInt64}, "enabled": {Kind: provider.KindBool}, "score": {Kind: provider.KindDouble}, "payload": {Kind: provider.KindBytes}, "note": {Kind: provider.KindString, Nullable: true}}
	config := map[string]any{"tables": []any{map[string]any{"name": "seed_types", "columns": columns}}}
	field := provider.FieldRequest{Field: "DB.types", Config: map[string]any{"table": "seed_types"}, Output: provider.OutputSpec{Type: provider.Type{Kind: provider.KindList, Values: &provider.Type{Kind: provider.KindMessage, Fields: fields}}}}
	instance, err := New(Config{Database: db, Driver: "pgx"}, config, []provider.FieldRequest{field})
	require.NoError(t, err)
	type row struct {
		ID      int64   `json:"id"`
		Enabled bool    `json:"enabled"`
		Score   float64 `json:"score"`
		Payload []byte  `json:"payload"`
		Note    *string `json:"note"`
	}
	note := "text"
	rows := []row{
		{ID: 9007199254740993, Enabled: true, Score: 1.5, Payload: []byte{1, 2}},
		{ID: 2, Payload: []byte{}, Note: &note},
	}
	require.NoError(t, instance.PrepareAttempt(t.Context()))
	t.Cleanup(func() { require.NoError(t, instance.CleanupAttempt(context.Background())) })
	result, err := instance.Seed(t.Context(), provider.SeedRequest{Values: []provider.SeedValue{{Field: "DB.types", Value: rows}}})
	require.NoError(t, err)
	require.NoError(t, provider.ValidateSeedValue("DB.types", rows, result.Values[0].Value))
	raw, err := instance.Observe(t.Context(), "DB.types")
	require.NoError(t, err)
	actual, err := provider.Convert[[]row](raw)
	require.NoError(t, err)
	require.ElementsMatch(t, rows, actual)
}

func TestProvider_SeedAndClearRespectForeignKeyDependencies(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://postgres@localhost:15432/postgres?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(t.Context(), `
 CREATE TEMP TABLE seed_parent (id BIGINT PRIMARY KEY);
 CREATE TEMP TABLE seed_child (id BIGINT REFERENCES seed_parent(id));
 INSERT INTO seed_parent VALUES (99);
 INSERT INTO seed_child VALUES (99)`)
	require.NoError(t, err)
	config := map[string]any{"tables": []any{
		map[string]any{"name": "seed_parent", "columns": []any{map[string]any{"name": "id", "type": "int64"}}},
		map[string]any{"name": "seed_child", "columns": []any{map[string]any{"name": "id", "type": "int64"}}},
	}}
	spec := provider.Type{Kind: provider.KindList, Values: &provider.Type{Kind: provider.KindMessage, Fields: map[string]provider.Type{"id": {Kind: provider.KindInt64}}}}
	instance, err := New(Config{Database: db, Driver: "pgx"}, config, []provider.FieldRequest{
		{Field: "DB.parent", Config: map[string]any{"table": "seed_parent"}, Output: provider.OutputSpec{Type: spec}},
		{Field: "DB.child", Config: map[string]any{"table": "seed_child"}, Output: provider.OutputSpec{Type: spec}},
	})
	require.NoError(t, err)
	// Preparation preserves existing rows; cleanup deletes children before parents.
	require.NoError(t, instance.PrepareAttempt(t.Context()))
	for _, field := range []string{"DB.parent", "DB.child"} {
		actual, err := instance.Observe(t.Context(), field)
		require.NoError(t, err)
		require.Equal(t, []map[string]any{{"id": int64(99)}}, actual)
	}
	require.NoError(t, instance.CleanupAttempt(t.Context()))
	for _, field := range []string{"DB.parent", "DB.child"} {
		actual, err := instance.Observe(t.Context(), field)
		require.NoError(t, err)
		require.Equal(t, []map[string]any{}, actual)
	}
	// Request order differs from registration order and must survive in the result.
	request := provider.SeedRequest{Values: []provider.SeedValue{
		{Field: "DB.child", Value: []map[string]any{{"id": int64(1)}}},
		{Field: "DB.parent", Value: []map[string]any{{"id": int64(1)}, {"id": int64(2)}}},
	}}
	result, err := instance.Seed(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, provider.SeedResult{Values: request.Values}, result)
	for _, value := range request.Values {
		actual, err := instance.Observe(t.Context(), value.Field)
		require.NoError(t, err)
		require.ElementsMatch(t, value.Value, actual)
	}
	require.NoError(t, instance.CleanupAttempt(context.Background()))
	for _, field := range []string{"DB.parent", "DB.child"} {
		actual, err := instance.Observe(t.Context(), field)
		require.NoError(t, err)
		require.Equal(t, []map[string]any{}, actual)
	}
}
