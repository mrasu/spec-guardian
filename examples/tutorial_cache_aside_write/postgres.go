package cacheaside

import (
	"context"
	"database/sql"
)

type postgresDB struct {
	database *sql.DB
}

func NewPostgresDB(database *sql.DB) *postgresDB {
	if database == nil {
		panic("cacheaside: nil database")
	}
	return &postgresDB{database: database}
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
