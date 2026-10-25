package dynamodb

import dynamofield "github.com/mrasu/spec-guardian/guardian/provider/dynamodb/internal/field"

type providerConfig struct {
	Endpoint        string                `json:"endpoint"`
	Region          string                `json:"region"`
	AccessKeyID     string                `json:"access_key_id"`
	SecretAccessKey string                `json:"secret_access_key"`
	Tables          []dynamofield.Table   `json:"tables"`
	Fields          []dynamofield.Request `json:"-"`
}
