# Tests

## Location and scope

```text
.
├── cmd/                      CLI tests
├── internal/                 Tests for code called by the CLI
├── guardian/                 Guardian library tests
└── examples/                 Demos only; no regular tests
    └── <example>/
        └── conformance/      Generated test demos
```

## Test design

- Name the behavior. Favor DAMP over Composed Method: keep the operation, decisive inputs, configuration, and assertions in `Test`; extract only incidental setup and cleanup.
- Name tests after the target and behavior, such as `TestNewField_ValidPath` or `TestField_String_ValidPath`. Use separate functions for distinct behaviors.
- For multiple cases, use a table-driven test: define `tests` before the loop, then iterate with `tt` and `t.Run`.
- Assert that behavior, not implementation steps. Compare full results for completeness claims, and introduce one cause at a time for expected mismatches.
- Test current contracts; do not add or retain tests solely to verify rejection of retired formats or APIs.
- For expected `(T, error)` failures, check the cause and a zero result unless the API promises a partial result.

## Running tests

- Run tests for the changed package, then affected modules and boundaries.
- Start required services before dependent tests; distinguish connection failures from behavior failures.
- After changing generated output, test it and confirm regeneration produces no further changes.
