// Package common provides S3 table object access shared by Iceberg providers.
package common

import (
	"context"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cockroachdb/errors"
)

// TableObjects reads objects under one table prefix.
type TableObjects struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewTableObjects validates the S3 location used by a provider attempt.
func NewTableObjects(client *s3.Client, bucket, prefix string) (*TableObjects, error) {
	if client == nil || bucket == "" || prefix == "" {
		return nil, errors.New("an S3 client, bucket, and table prefix are required")
	}
	return &TableObjects{client: client, bucket: bucket, prefix: prefix}, nil
}

// ListKeys returns keys under the table prefix with the given suffix.
func (o *TableObjects) ListKeys(ctx context.Context, suffix string) ([]string, error) {
	paginator := s3.NewListObjectsV2Paginator(o.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(o.bucket), Prefix: aws.String(o.prefix),
	})
	var keys []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "list table objects under %s", o.prefix)
		}
		for _, object := range page.Contents {
			key := aws.ToString(object.Key)
			if strings.HasSuffix(key, suffix) {
				keys = append(keys, key)
			}
		}
	}
	return keys, nil
}

// Read returns the complete contents of a table object.
func (o *TableObjects) Read(ctx context.Context, key string) ([]byte, error) {
	response, err := o.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(o.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, errors.Wrapf(err, "get object %s", key)
	}
	contents, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil {
		return nil, errors.Wrapf(readErr, "read object %s", key)
	}
	if closeErr != nil {
		return nil, errors.Wrapf(closeErr, "close object %s", key)
	}
	return contents, nil
}
