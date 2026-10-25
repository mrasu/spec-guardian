# SpecGuardian

This document records design constraints for conformance test generation.

## Implementation

- Use [Architecture](architecture.md) for component boundaries and execution contracts.
- Keep structural configuration rules in [the schema](../../internal/configschema/specguardian.schema.yaml); Go owns semantic resolution and generation support checks.

## Case extraction

- Prior Actions may establish inputs; extraction must not be limited to initial-state executions.
- Action names do not identify executions. Preserve thread identity and exclude other Actions' intervention, even when state is unchanged.
- Canonicalization merges equivalent cases, not semantically distinct values.

## Observation and comparison

- Derive allowed states from the model; user-written expected-state mapping would weaken the model's authority.
- Keep model and observation types consistent without losing value distinctions; use [Guardian contracts](guardian.md) for runtime conversion and comparison.
- Keep the execution lifecycle visible in generated tests so fault scope and observation timing remain reviewable.
- Incomplete user implementations must fail, not silently reduce coverage.

## Regeneration

- Preserve user implementations across model changes, including removed Actions. Signature updates may require later user adaptation; compilation is not a condition for saving generated output.
- Generation processing failures must leave existing files unchanged; unchanged inputs must regenerate without differences.
- Reserve `_sg` for generator-owned internal identifiers to avoid collisions with user code.

### Generated files

Examples contain runnable demonstrations and their generated conformance tests.

Regenerate example conformance tests with `make generate-conformance-spec` when available.

- `conformance/specguardian.go`: regeneration entry point.
- `conformance/specguardian/<role>_conformance_cases_generated_test.go`: state types, provider wiring, case loading, and allowed-state comparisons.
- `conformance/specguardian/<role>_conformance_generated_test.go`: test execution and fault iteration.
- `conformance/specguardian/<role>_conformance_impl_test.go`: user-edited functions that map abstract specification states to concrete application states.
- `conformance/specguardian/conformance_common_generated_test.go`: shared message types and field-level comparison helpers.
- `conformance/specguardian/testdata/<role>_<action>_cases_generated.json`: test-ready data extracted from FizzBee results and loaded by the generated tests.
