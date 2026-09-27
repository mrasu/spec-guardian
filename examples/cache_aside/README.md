# Cache-Aside Example

This example tests `Read` and `Write` against a FizzBee cache-aside spec. `Write` stores a value in PostgreSQL and invalidates its Valkey cache entry; `Read` uses the cache or loads the value from PostgreSQL. The conformance tests compare the resulting database, cache, and request state with states allowed by the FizzBee spec, including injected I/O failures.

## Run the tests

You need Go and Docker Compose. The FizzBee exploration results and generated tests are included, so FizzBee is not needed to run them. From this directory:

```sh
docker compose up -d
go test -tags=specguardian ./conformance/...
docker compose down
```

A passing `go test` command means the observed states matched the FizzBee spec's allowed states for the included cases.
