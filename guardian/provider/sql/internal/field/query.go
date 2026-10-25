package field

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mrasu/spec-guardian/guardian/provider"
)

// QueryField observes a configured SQL query.
type QueryField struct {
	query  string
	reader Reader
}

// Reader reads SQL rows into an observed field value.
type Reader interface {
	Read(*sql.Rows) (any, error)
}

// NewQueryField resolves schema-validated query settings against the model type.
func NewQueryField(request Request) (*QueryField, error) {
	reader, err := newReader(request.Config.Result, request.Output.Type)
	if err != nil {
		return nil, fmt.Errorf("SQL field %s: %w", request.Field, err)
	}
	return &QueryField{query: request.Config.Query, reader: reader}, nil
}

// Observe executes the configured query and decodes its result.
func (q *QueryField) Observe(ctx context.Context, db *sql.DB) (any, error) {
	rows, err := db.QueryContext(ctx, q.query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return q.reader.Read(rows)
}

func newReader(config ResultConfig, spec provider.Type) (Reader, error) {
	if config.Type == ResultTypeKV {
		return NewKVReader(config, spec)
	}
	return NewRowMapReader(config, spec)
}
