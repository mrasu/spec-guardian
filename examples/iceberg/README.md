# Iceberg Example

This example tests `Append`, `Delete`, and `Overwrite` through the public `Table` API of [Apache Iceberg Go](https://github.com/apache/iceberg-go) against a FizzBee spec. The conformance tests compare the visible data files, catalog metadata change, and operation result with states allowed by the spec when an S3 API call fails.

## Run the tests

You need Go and Docker Compose. The included Compose file starts Apache Polaris and RustFS and sets up an Iceberg warehouse. FizzBee exploration results and generated tests are included. From this directory:

```sh
docker compose up -d --wait
go test -tags=specguardian ./conformance/specguardian/...
docker compose down
```

A passing `go test` command means the observed states matched the spec's allowed states for the included cases.

## FizzBee spec and license

The FizzBee spec is adapted from Jack Vanlightly's [Apache Iceberg FizzBee specification](https://github.com/Vanlightly/table-formats-tlaplus/blob/main/iceberg/iceberg.fizz). The upstream copyright and MIT license notice are preserved in [LICENSE.iceberg-fizzbee](./LICENSE.iceberg-fizzbee).
