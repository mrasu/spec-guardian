# Guardian

This document records design constraints for the conformance test runtime.

## Public boundary

- Use [Architecture](architecture.md) for responsibility boundaries and lifecycle contracts.
- Keep public APIs limited to caller needs and dependencies minimal; implementation details belong under `internal`.
- Use only the standard library for error handling.

## Provider configuration

- Each provider must define shared and field configs in a YAML schema file.
- Validate configs at construction and decode them into internal structs.
- Schemas check structure; Go checks references, uniqueness, model types, and runtime settings.
- Share schema machinery internally; each provider owns its schema, config types, and cache.

## Fault injection and hooks

- Hooks must preserve wrapped-library behavior except for deliberate injection; they must not change retry policies to make tests pass.
- Define injection units at the wrapped API boundary, not by incidental lower-level work. Retries can mask injected failures and must be controlled by the caller.
- Recovery after injection must remain observable without injecting a second fault in the same attempt.
- Report incomplete fault coverage rather than treating exhausted limits or absent hooked I/O as full coverage.
- Operation labels are diagnostics, not stable identifiers; omit application payloads.

## Seeding

- Pass a deep copy to Seed and validate its result against the unchanged model input.
- Seed results must preserve input values, structure, and field and list order.
- Call CleanupAttempt even after preparation or seeding fails.

## Observation

- Keep observation stages consistent for application fields and SpecGuardian metadata.
  - **Fetched values:** Hold raw values returned by providers, without comparison wrappers.
  - **Manual observation:** Supply comparison-ready values through the existing manual observation builder.
  - **Observed state:** Wrap fetched values for comparison and combine them with manually observed values in the same field structure.
- Define shared observation field names and output contracts in the provider package, independently of concrete provider implementations.
- Keep expected JSON metadata and fetched metadata in separate types, with names matching their processing stages.
- Use data-source terminology for provider configuration rather than forcing source-specific concepts into a universal vocabulary.
- Fetch failures are test failures, not empty state or unobservability.
- Field order must not imply a consistent snapshot across sources.
- Conversion must preserve model distinctions; reject implicit coercion and ambiguous null representations.
- Delete attempt data only in CleanupAttempt; PrepareAttempt must preserve existing data.

## Comparison

- Judge conformance by resulting state and configured execution metadata, not I/O traces or the identity of the injected error. When Action error comparison is enabled, compare error presence together with the same allowed state; error wrapping must not change the verdict.
- Preserve container shape and multiplicity. Ignoring order must not turn a collection into a set.
- Unobservability explicitly limits verification; it is distinct from failure to fetch a value.
- Mismatch diagnostics must identify the full field path and expose enough state to explain structural differences.
