# Cache-Aside Example

This example checks whether cache-aside reads and writes, including injected I/O failures, produce states and errors allowed by a FizzBee model.

## Run the tests

You need Go and Docker Compose. The FizzBee exploration results and generated tests are included, so FizzBee is not needed to run them. From this directory:

```sh
docker compose up -d
go test -tags=specguardian ./conformance/...
docker compose down
```

A passing `go test` command means the observed states matched the FizzBee spec's allowed states for the included cases.
