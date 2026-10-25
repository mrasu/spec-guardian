# Spec Guardian

<img src="docs/img/specguardian-logo.png" alt="SpecGuardian" width="640">

Let formal specs test AI implementations.

A [FizzBee](https://fizzbee.io/) spec describes the state transitions your system allows. However, the spec alone does not test whether your implementation follows those transitions. SpecGuardian turns FizzBee's results into tests that check whether your implementation reaches an allowed state.

SpecGuardian lets you verify that an AI-generated implementation actually conforms to your specification.

![FizzBee explores allowed states; SpecGuardian generates tests that run normally and with injected I/O failures, then check the resulting states against the specification.](docs/img/specguardian-workflow.png)

## From Spec to Test: Cache-Aside

In this cache-aside example, a `WriteAndInvalidate` action stores a new value in the database, then invalidates the corresponding cache entry. The [tutorial spec](examples/tutorial_cache_aside_write/docs/spec/CacheAside.fizz) models each operation as atomic and allows a crash before or between them:

```python
@conformance
role CacheAside:
  fair action WriteAndInvalidate:
    # Yield to allow a crash before the first I/O.
    atomic:
      pass

    # Write to the database first, then invalidate the cache.
    db.write(self.key, self.write_value)
    cache.delete_cache(self.key)
```

SpecGuardian [generates Go tests](examples/tutorial_cache_aside_write/conformance/specguardian/cacheaside_conformance_generated_test.go) from FizzBee's results. They compare the observed state after the action with states allowed by the spec, including states reached when an I/O operation fails.

SQL and Valkey providers seed each attempt from the model's input state, observe the result, and clean up afterward. The test compares that result with the spec's allowed states, injecting faults only during action execution.

For example, the test also checks what happens when an error is injected before cache invalidation. This case can pass because the spec allows a state where the database holds the new value and the cache retains the old one.

### Catching a Spec Mismatch

Consider an ordering bug in `WriteAndInvalidate`: cache invalidation runs before the database write.

```diff
func (ca *CacheAside) WriteAndInvalidate(ctx context.Context, input *Input) (*Output, error) {
+  if err := ca.cache.Delete(ctx, input.Key); err != nil {
+    return nil, err
+  }
+
   if err := ca.db.Write(ctx, input.Key, input.Value); err != nil {
     return nil, err
   }

-  if err := ca.cache.Delete(ctx, input.Key); err != nil {
-    return nil, err
-  }
-
   return &Output{Key: input.Key, Value: input.Value}, nil
}
```

Both orders produce the same final state on success. But when the database write fails after cache invalidation, the database still holds `old` and the cache entry is gone. The spec writes to the database first, so stopping at any point cannot produce this combination.

The test fails with the following output (excerpt):

```text
actual state does not match any allowed state:
    I/O events:
      1. valkey.Do
      2. sql.BeginTx [fault injected; call not executed]
```

The mismatch excerpts show the unchanged database value and missing cache entry:

```text
Fields.DB.Records[0].Value (-want +got):
  string(
-  "new",
+  "old",
  )
```

```text
Fields.Cache.CachedRecords (-want +got):
  map[string]string{
-  "cached": "old",
  }
```

## Getting Started

To try this example locally, start with the included cache-aside write tutorial. You'll need Go and Docker Compose; the FizzBee exploration results and generated tests are included.

Clone the repository and run the example:

```sh
git clone https://github.com/mrasu/spec-guardian.git
cd spec-guardian/examples/tutorial_cache_aside_write
docker compose up -d
go test -count=1 -tags=specguardian ./conformance/...
```

The test passes, checking normal completion and injected I/O failures. Apply the diff above to `cache_aside.go` and rerun the `go test` command to reproduce the failure. Restore the original order to make the test pass again.

## How It Works

Here's how SpecGuardian works:

1. FizzBee explores the possible states described by the spec.
2. SpecGuardian generates Go tests from the exploration results.
3. Either AI or you write the setup and Action execution code, configure providers in `specguardian.yml`, and write any remaining state observation code.
4. The tests run a function normally and with an error injected immediately before a supported I/O call, then compare the observed state with states allowed by the spec.

`specguardian.yml` configures state observation and comparison. See the [configuration schema](docs/specguardian.schema.yaml) for available settings and the [example configuration](examples/tutorial_cache_aside_write/specguardian.yml) for an example.

## When to Use

SpecGuardian checks whether an Action ends in a state allowed by the FizzBee spec, including after an injected I/O failure. It complements other tests:

- **Different from unit tests:** It checks the resulting application state against the spec, beyond assertions about individual functions.
- **Different from integration tests:** The expected states come from a FizzBee spec defined independently of the implementation.

## Generate Tests for Your Project

To generate tests for your project, install [FizzBee](https://fizzbee.io/) and run these commands from your Go module directory:

```sh
fizz --output-dir path/to/output_dir path/to/YourSpec.fizz
go get -tool github.com/mrasu/spec-guardian
go tool spec-guardian init --fizzbee-output-dir path/to/output_dir

# Configure observation and comparison.
$EDITOR specguardian.yml

go generate ./conformance

# Replace <role> with the generated role name and connect the tests to your application.
$EDITOR "conformance/specguardian/<role>_conformance_impl_test.go"

go mod tidy
go test -tags=specguardian ./conformance/...
```

See the [user guide](docs/user-guide.md) for the complete setup.

## Examples

- [Cache-aside write tutorial](examples/tutorial_cache_aside_write) is the completed project from the [user guide](docs/user-guide.md).
- [Cache-aside](examples/cache_aside) demonstrates the cache-aside pattern.
- [Idempotent request](examples/idempotent_request) demonstrates idempotent requests.
- [Iceberg](examples/iceberg) is a real-world example that tests [apache/iceberg-go](https://github.com/apache/iceberg-go).

## Current Scope

SpecGuardian currently generates Go tests for one FizzBee Action invocation at a time. See the [guardian directory](guardian/) for the available I/O hooks.

SpecGuardian is under development. Its public APIs or generated Go code may change.
