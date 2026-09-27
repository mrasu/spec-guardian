# Idempotent Request Example

This example tests `OrderServer.CreateOrder` and `DeliveryServer.Deliver` against a FizzBee spec. The tests check how order and delivery records in MySQL and DynamoDB, along with request results, change when an I/O call fails. They also check repeated and conflicting requests against the states allowed by the spec.

## Run the tests

You need Go and Docker Compose. The included Compose file starts MySQL and a local DynamoDB compatible service. FizzBee exploration results and generated tests are included. From this directory:

```sh
docker compose up -d
go test -tags=specguardian ./conformance/...
docker compose down
```

A passing `go test` command means the observed states matched the spec's allowed states for the included cases.
