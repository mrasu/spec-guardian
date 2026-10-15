# Idempotent Request Example

This example checks an order and delivery workflow against a FizzBee model, including failures, retries, and conflicting requests.

## Run the tests

You need Go and Docker Compose. The included Compose file starts MySQL and a local DynamoDB compatible service. FizzBee exploration results and generated tests are included. From this directory:

```sh
docker compose up -d
go test -tags=specguardian ./conformance/...
docker compose down
```

A passing `go test` command means the observed states matched the spec's allowed states for the included cases.
