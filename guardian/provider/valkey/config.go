package valkey

import "github.com/mrasu/spec-guardian/guardian/provider"

type providerConfig struct {
	Address   string         `json:"address"`
	Keyspaces []keyspace     `json:"keyspaces"`
	Fields    []fieldRequest `json:"-"`
}

type fieldRequest struct {
	Field  string
	Config fieldConfig
	Output provider.OutputSpec
}

type fieldConfig struct {
	Keyspace string `json:"keyspace"`
	Keys     string `json:"keys"`
}
