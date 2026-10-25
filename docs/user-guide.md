# Use SpecGuardian in Your Go Project

This guide explains how to add SpecGuardian to your Go project using the [cache-aside example](../examples/cache_aside/).

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

You have run the finished example. Now follow the steps for connecting SpecGuardian to a project, using the [tutorial application](../examples/tutorial_cache_aside_write/) to build a `WriteAndInvalidate` conformance test.

### 1. Prepare an independent project

Install [FizzBee](https://fizzbee.io/) separately. For this tutorial, copy the completed project and remove its configuration and generated tests so you can create them yourself. Set `TUTORIAL_ROOT` to the tutorial in your checkout and `NEW_TUTORIAL_DIR` to a new directory of your choice:

```bash
TUTORIAL_ROOT="/path/to/spec-guardian/examples/tutorial_cache_aside_write"
NEW_TUTORIAL_DIR="/path/to/your/tutorial_cache_aside_write"
cp -R "$TUTORIAL_ROOT" "$NEW_TUTORIAL_DIR"
rm -rf "$NEW_TUTORIAL_DIR/conformance" "$NEW_TUTORIAL_DIR/docs" "$NEW_TUTORIAL_DIR/specguardian.yml" "$NEW_TUTORIAL_DIR/go.mod" "$NEW_TUTORIAL_DIR/go.sum"
cd "$NEW_TUTORIAL_DIR"
go mod init spec-guardian/examples/tutorial-cache-aside-write
```

The application writes to PostgreSQL and invalidates the Valkey cache.

### 2. Prepare and explore the FizzBee spec

Next, add the FizzBee spec to your project and explore it. Copy [CacheAside.fizz](../examples/tutorial_cache_aside_write/docs/spec/CacheAside.fizz):

```bash
cd "$NEW_TUTORIAL_DIR"
mkdir -p docs/spec
cp "$TUTORIAL_ROOT/docs/spec/CacheAside.fizz" docs/spec/
```

The spec models each database and cache operation as atomic and allows a crash before or between them. The `@conformance` marker identifies the role to check against the application. Its actions usually represent functions or endpoints; here, SpecGuardian tests `WriteAndInvalidate`.

```fizzbee
@conformance
role CacheAside:
  fair action WriteAndInvalidate:
    # Yield to allow a crash before the first I/O.
    atomic:
      pass
    db.write(self.key, self.write_value)
    cache.delete_cache(self.key)

action Init:
  db = DB()
  cache = Cache()
  key = oneof ["cached", "uncached"]
  cache_aside = CacheAside(key=key, write_value="new")
```

Run FizzBee from your project directory:

```bash
cd "$NEW_TUTORIAL_DIR"
fizz --output-dir docs/spec/fizzbee_output docs/spec/CacheAside.fizz
```

FizzBee writes exploration results to `docs/spec/fizzbee_output`. SpecGuardian uses these results to generate the tests.

### 3. Initialize and generate the conformance tests

Install SpecGuardian and initialize the project:

```bash
cd "$NEW_TUTORIAL_DIR"
go get -tool github.com/mrasu/spec-guardian
go tool spec-guardian init --fizzbee-output-dir docs/spec/fizzbee_output
```

`init` creates the initial configuration and a `go generate` entry point.

For this tutorial, use the [completed configuration](../examples/tutorial_cache_aside_write/specguardian.yml), which declares SQL and Valkey providers. Omit its `dev` setting, which is used for developing SpecGuardian:

```bash
sed '/^dev:/d' "$TUTORIAL_ROOT/specguardian.yml" > specguardian.yml
```

The configuration maps model fields to stored data:

- `DB.records` maps to rows in the SQL `records` table, compared without regard to order.
- `Cache.cached_records` maps to Valkey string keys stored with the `records:` prefix.

These providers seed each attempt from the model's input state, observe the result, and clean up afterward.

Generate the tests:

```bash
go generate ./conformance
go mod tidy
```

`go generate ./conformance` generates tests and application connection code under `conformance/specguardian/` using `specguardian.yml`.

Run the generated test once before connecting the application:

```bash
cd "$NEW_TUTORIAL_DIR"
go test -tags=specguardian ./conformance/...
```

The test fails with `TODO: implement me` because the application connection code is not filled in yet.

### 4. Connect the spec to the application

Connect the generated tests to the application. Copy the completed conformance code from the [tutorial](../examples/tutorial_cache_aside_write/conformance/specguardian/) into your project:

```bash
cd "$NEW_TUTORIAL_DIR"
cp "$TUTORIAL_ROOT"/conformance/specguardian/cacheaside_conformance_impl_test.go conformance/specguardian/
```

The copied code opens database and cache connections, wires fault injection for `WriteAndInvalidate`, and maps the model's arguments to the application call.

The configuration's `hooks: [cleanup_case]` enables the hook that closes these connections after each case.

### 5. Run the connected test

Start this project's services and run the connected test:

```bash
cd "$NEW_TUTORIAL_DIR"
go mod tidy
docker compose up -d
go test -tags=specguardian ./conformance/...
```

The test checks whether the application's state and failure outcome are allowed by the model. If no allowed outcome matches, it reports the differences.

### 6. Complete the tutorial

You now have a working `WriteAndInvalidate` conformance test that checks the application's state against the FizzBee spec. To test `Read`, see the [full cache-aside example](../examples/cache_aside/).

Follow the same flow in your own project: explore your FizzBee spec, initialize SpecGuardian, configure data preparation and observation, connect the generated tests to your application, and run them. Use your project's spec, `specguardian.yml`, and application connection code in place of the tutorial files.

When you change the spec, rerun FizzBee and regenerate the tests:

```bash
fizz --output-dir docs/spec/fizzbee_output docs/spec/CacheAside.fizz
go generate ./conformance
```

If you only change `specguardian.yml`, run `go generate ./conformance`. Your application connection code is preserved when you regenerate.

See the [configuration schema](specguardian.schema.yaml) for available settings in `specguardian.yml`.
