# Cache-Aside Write Tutorial

This is the completed project built in the [SpecGuardian user guide](../../docs/user-guide.md). It models and tests `Application.Write` only. For `Read` and `Write` together, see the [full cache-aside example](../cache_aside/).

With Go 1.27, FizzBee, and Docker Compose installed, run these commands from this directory:

```sh
fizz --output-dir docs/spec/fizzbee_output docs/spec/CacheAside.fizz
go generate ./conformance/...
docker compose up -d
go test -tags=specguardian ./conformance/...
docker compose down
```
