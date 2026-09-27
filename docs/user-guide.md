# Use SpecGuardian with Cache-Aside

Use this guide to run the [cache-aside example](../examples/cache_aside/), then build a `Write` conformance test in a separate Go project. A conformance test runs an action in the application and checks its resulting state against the spec's allowed states.

## Getting Started

You need Go 1.27 or later and Docker Compose. From the repository root, start PostgreSQL and Valkey, then run the tests:

```sh
cd examples/cache_aside
docker compose up -d
go test -tags=specguardian ./conformance/...
docker compose down
```

The example already includes the FizzBee exploration results and generated tests. The `specguardian` build tag includes those tests and the application-owned setup.

## Build and Connect the Tests

You have run the finished example. Now build the cache-aside `Write` conformance test in a separate Go project. Start from the [completed tutorial](../examples/tutorial_cache_aside_write/), then explore the spec and generate the tests yourself.

### 1. Prepare an independent project

Install [FizzBee](https://fizzbee.io/) separately. Set `TUTORIAL_ROOT` to the completed tutorial in your checkout and `NEW_TUTORIAL_DIR` to a new directory of your choice. Copy the tutorial, then reset the copy so you can generate the results and tests yourself:

```bash
TUTORIAL_ROOT="/path/to/spec-guardian/examples/tutorial_cache_aside_write"
NEW_TUTORIAL_DIR="/path/to/your/tutorial_cache_aside_write"
cp -R "$TUTORIAL_ROOT" "$NEW_TUTORIAL_DIR"
rm -rf "$NEW_TUTORIAL_DIR/conformance" "$NEW_TUTORIAL_DIR/docs"
```

The application writes to PostgreSQL and invalidates the Valkey cache.

### 2. Prepare and explore the FizzBee spec

Next, add the FizzBee spec to your project and explore it. Copy [CacheAside.fizz](../examples/tutorial_cache_aside_write/docs/spec/CacheAside.fizz):

```bash
cd "$NEW_TUTORIAL_DIR"
mkdir -p docs/spec
cp "$TUTORIAL_ROOT/docs/spec/CacheAside.fizz" docs/spec/
```

The spec allows `Write` to succeed or fail depending on I/O. The `@conformance` marker identifies the role to check against the application. Its actions usually represent functions or endpoints; here, SpecGuardian tests `Write`.

```fizzbee
@conformance
role Application:
  action Init:
    self.write_status = RequestStatus.INIT

  fair action Write:
    require self.write_status == RequestStatus.INIT
    self.write_status = RequestStatus.PROCESSING

    db_succeeds = db.write(self.key, self.write_value)
    if not db_succeeds:
      self.write_status = RequestStatus.FAILED
      return

    delete_cache_succeeds = cache.delete_cache(self.key)
    if not delete_cache_succeeds:
      self.write_status = RequestStatus.FAILED
      return

    self.write_status = RequestStatus.DONE

action Init:
  db = DB()
  cache = Cache()
  key = oneof ["cached", "uncached"]
  application = Application(key=key, write_value="new")
```

Run FizzBee from your project directory:

```bash
cd "$NEW_TUTORIAL_DIR"
fizz --output-dir docs/spec/fizzbee_output docs/spec/CacheAside.fizz
```

When FizzBee succeeds, it writes exploration results to `docs/spec/fizzbee_output`. SpecGuardian reads those results; it does not run FizzBee. Run FizzBee again after changing the spec.

### 3. Generate the conformance tests

Generate conformance tests from the FizzBee exploration results. Set `SPEC_GUARDIAN_PATH` to the path of your SpecGuardian checkout, then update the copied `go.mod`:

```bash
cd "$NEW_TUTORIAL_DIR"
SPEC_GUARDIAN_PATH="/path/to/spec-guardian"
go mod edit -replace="github.com/mrasu/spec-guardian=$SPEC_GUARDIAN_PATH" -replace="github.com/mrasu/spec-guardian/guardian=$SPEC_GUARDIAN_PATH/guardian"
go run github.com/mrasu/spec-guardian generate --project-dir . --fizz-output-dir docs/spec/fizzbee_output
```

The command creates tagged tests and an application-specific scaffold under `conformance/`.

Run the generated test once before connecting the application:

```bash
cd "$NEW_TUTORIAL_DIR"
go test -tags=specguardian ./conformance/...
```

The test fails with `TODO: implement me` because the TODOs are not filled in yet. The tests cannot run the application or observe its state.

### 4. Connect the spec to the application

Connect the generated tests to the application. Copy the completed conformance code from the [tutorial](../examples/tutorial_cache_aside_write/conformance/specguardian/) into your project:

```bash
cd "$NEW_TUTORIAL_DIR"
cp "$TUTORIAL_ROOT"/conformance/specguardian/{application_conformance_impl_test.go,records_test.go} conformance/specguardian/
```

The copied code exercises the application under injected I/O failures and retrieves its resulting state. SpecGuardian hooks can return an injected error immediately before a supported I/O call executes. The generated test compares that state with the spec's allowed states. For your own application, change the code that runs the action and retrieves its state.

### 5. Run the connected test

Start this project's services and run the connected test:

```bash
cd "$NEW_TUTORIAL_DIR"
go mod tidy
docker compose up -d
go test -tags=specguardian ./conformance/...
```

The generated test exercises `Write` with injected I/O failures. It passes when every observed state is allowed by the spec.
If the observed state matches no state allowed by the spec, the test fails and reports the differences.

### 6. Complete the tutorial

You now have a working `Write` conformance test that checks the application's state against the FizzBee spec. To test `Read`, see the [full cache-aside example](../examples/cache_aside/).
