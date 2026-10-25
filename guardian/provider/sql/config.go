package sql

import sqlfield "github.com/mrasu/spec-guardian/guardian/provider/sql/internal/field"

type providerConfig struct {
	Driver           string             `json:"driver"`
	ConnectionString string             `json:"connection_string"`
	Tables           []sqlfield.Table   `json:"tables"`
	Fields           []sqlfield.Request `json:"-"`
}
