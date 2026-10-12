package field

import (
	"database/sql"
	"fmt"

	"github.com/mrasu/spec-guardian/guardian/observer"
)

// Query is a validated SQL query and row reader for one field.
type Query struct {
	query  string
	reader Reader
}

// Reader reads SQL rows into an observed field value.
type Reader interface {
	Read(*sql.Rows) (any, error)
}

type resultType string

const (
	resultTypeKV     resultType = "kv"
	resultTypeRowMap resultType = "row_map"
)

// NewQuery validates one field's query and result mapping.
func NewQuery(request observer.FieldRequest) (*Query, error) {
	query, ok := request.Config["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("SQL field %s requires query", request.Field)
	}
	resultConfig, ok := request.Config["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("SQL field %s requires result", request.Field)
	}
	reader, err := newReader(resultConfig, request.Output.Type)
	if err != nil {
		return nil, fmt.Errorf("SQL field %s: %w", request.Field, err)
	}
	return &Query{query: query, reader: reader}, nil
}

// SQL returns the query text.
func (q *Query) SQL() string {
	return q.query
}

// Read converts the query's rows into an observed field value.
func (q *Query) Read(rows *sql.Rows) (any, error) {
	return q.reader.Read(rows)
}

func newReader(config map[string]any, spec observer.Type) (Reader, error) {
	resultName, ok := config["type"].(string)
	if !ok {
		return nil, fmt.Errorf("result.type is required")
	}
	switch resultType(resultName) {
	case resultTypeKV:
		return NewKVReader(config, spec)
	case resultTypeRowMap:
		return NewRowMapReader(config, spec)
	default:
		return nil, fmt.Errorf("unsupported result.type %q", resultName)
	}
}
