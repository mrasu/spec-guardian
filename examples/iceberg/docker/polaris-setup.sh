#!/bin/sh
set -eu

response="$(curl --fail-with-body --silent --show-error -X POST http://polaris:8181/api/catalog/v1/oauth/tokens -H 'Content-Type: application/x-www-form-urlencoded' -d 'grant_type=client_credentials&client_id=test&client_secret=test&scope=PRINCIPAL_ROLE:ALL')"
token="$(printf '%s' "$response" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')"
test -n "$token"

curl --fail-with-body --silent --show-error -X POST http://polaris:8181/api/management/v1/catalogs \
  -H "Authorization: Bearer $token" \
  -H 'Content-Type: application/json' \
  -H 'Polaris-Realm: POLARIS' \
  -d '{"catalog":{"name":"iceberg","type":"INTERNAL","readOnly":false,"properties":{"default-base-location":"s3://iceberg-warehouse"},"storageConfigInfo":{"storageType":"S3","allowedLocations":["s3://iceberg-warehouse"],"endpoint":"http://localhost:9000","endpointInternal":"http://rustfs:9000","pathStyleAccess":true,"stsUnavailable":true,"region":"us-east-1"}}}'

curl --fail-with-body --silent --show-error -X POST http://polaris:8181/api/management/v1/principal-roles \
  -H "Authorization: Bearer $token" \
  -H 'Content-Type: application/json' \
  -H 'Polaris-Realm: POLARIS' \
  -d '{"principalRole":{"name":"iceberg_role","properties":{}}}'

curl --fail-with-body --silent --show-error -X POST http://polaris:8181/api/management/v1/catalogs/iceberg/catalog-roles \
  -H "Authorization: Bearer $token" \
  -H 'Content-Type: application/json' \
  -H 'Polaris-Realm: POLARIS' \
  -d '{"catalogRole":{"name":"iceberg_catalog_role","properties":{}}}'

curl --fail-with-body --silent --show-error -X PUT http://polaris:8181/api/management/v1/principals/root/principal-roles \
  -H "Authorization: Bearer $token" \
  -H 'Content-Type: application/json' \
  -H 'Polaris-Realm: POLARIS' \
  -d '{"principalRole":{"name":"iceberg_role"}}'

curl --fail-with-body --silent --show-error -X PUT http://polaris:8181/api/management/v1/principal-roles/iceberg_role/catalog-roles/iceberg \
  -H "Authorization: Bearer $token" \
  -H 'Content-Type: application/json' \
  -H 'Polaris-Realm: POLARIS' \
  -d '{"catalogRole":{"name":"iceberg_catalog_role"}}'

curl --fail-with-body --silent --show-error -X PUT http://polaris:8181/api/management/v1/catalogs/iceberg/catalog-roles/iceberg_catalog_role/grants \
  -H "Authorization: Bearer $token" \
  -H 'Content-Type: application/json' \
  -H 'Polaris-Realm: POLARIS' \
  -d '{"type":"catalog","privilege":"CATALOG_MANAGE_CONTENT"}'

touch /tmp/setup-complete
tail -f /dev/null
