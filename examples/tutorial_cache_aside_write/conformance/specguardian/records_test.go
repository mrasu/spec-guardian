//go:build specguardian

package specguardian_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"
)

func setupRecords(t *testing.T, db *sql.DB, cache valkey.Client, dbRecords, cacheRecords map[string]any) {
	t.Helper()
	clearRecords(t, db, cache)

	for key, value := range dbRecords {
		_, err := db.ExecContext(t.Context(), "INSERT INTO records(KEY, value) VALUES ($1, $2)", key, value)
		require.NoError(t, err)
	}
	for key, value := range cacheRecords {
		err := cache.Do(t.Context(), cache.B().Set().Key(key).Value(value.(string)).Build()).Error()
		require.NoError(t, err)
	}
}

func clearRecords(t *testing.T, db *sql.DB, cache valkey.Client) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, "TRUNCATE TABLE records")
	require.NoError(t, err)
	require.NoError(t, cache.Do(ctx, cache.B().Flushdb().Build()).Error())
}

func readDBRecords(t *testing.T, db *sql.DB) map[string]any {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT key, value FROM records")
	require.NoError(t, err)
	defer rows.Close()

	records := make(map[string]any)
	for rows.Next() {
		var key, value string
		require.NoError(t, rows.Scan(&key, &value))
		records[key] = value
	}
	require.NoError(t, rows.Err())
	return records
}

func readCacheRecords(t *testing.T, cache valkey.Client) map[string]any {
	t.Helper()
	keys, err := cache.Do(t.Context(), cache.B().Keys().Pattern("*").Build()).AsStrSlice()
	require.NoError(t, err)

	records := make(map[string]any, len(keys))
	for _, key := range keys {
		value, err := cache.Do(t.Context(), cache.B().Get().Key(key).Build()).ToString()
		require.NoError(t, err)
		records[key] = value
	}
	return records
}
