package cacheaside

import (
	"context"
	"database/sql"
	"errors"
)

type postgresDB struct {
	database *sql.DB
}

func (db *postgresDB) Read(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := db.database.QueryRowContext(ctx, `SELECT value FROM records WHERE KEY = $1`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (db *postgresDB) Write(ctx context.Context, key string, value string) error {
	tx, err := db.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO records (key, value) VALUES ($1, $2)
		ON CONFLICT (KEY) DO UPDATE SET VALUE = EXCLUDED.value
	`, key, value)
	if err != nil {
		return err
	}
	return tx.Commit()
}
