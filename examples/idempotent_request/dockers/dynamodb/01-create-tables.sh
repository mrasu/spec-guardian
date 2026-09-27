#!/bin/sh
set -eu

for table in order_idempotency_records delivery_records; do
  aws dynamodb create-table \
    --table-name "${table}" \
    --attribute-definitions AttributeName=id,AttributeType=S \
    --key-schema AttributeName=id,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST
done
