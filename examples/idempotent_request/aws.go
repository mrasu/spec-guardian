package idempotentrequest

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

const (
	dynamoEndpoint = "http://localhost:14566"
	dynamoRegion   = "us-east-1"
)

// NewDynamoDBClient returns a client configured for the example's Floci service.
func NewDynamoDBClient() *dynamodb.Client {
	return dynamodb.New(dynamodb.Options{
		BaseEndpoint: aws.String(dynamoEndpoint),
		Region:       dynamoRegion,
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
		Retryer:      aws.NopRetryer{},
	})
}

type dynamoDBAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}
