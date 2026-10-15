# Architecture

This document records the boundaries and relationships that guide conformance testing.

## Repository structure

```text
.
├── cmd/                    SpecGuardian CLI
├── internal/
├── guardian/               Public conformance runtime
└── examples/               Runnable demonstrations
    └── <example>/
        └── conformance/    Generated conformance tests
```

## Conformance scope

- The model defines allowed resulting states, not the implementation's internal steps.
- Conformance covers an isolated Action with a single fault before supported I/O or no injection; it does not establish concurrent execution correctness.
- Coverage is bounded by model exploration and the test's observation and injection scope; generation does not prove exploration completeness.

## Responsibility boundaries

- [SpecGuardian](spec-guardian.md) resolves the conformance contract during generation; [Guardian](guardian.md) enforces it at runtime.
- Plugins own data-source-specific observation; user implementations bridge the model and application. Generation must not absorb those responsibilities.
- Preparing the execution environment is separate from generating tests.

## Observation and comparison

- SpecGuardian metadata follows the same fetch, observation, and comparison stages as application fields.
- Inject faults only during Action execution so test preparation and observation cannot become failure sources under test.
- User setup establishes the context for plugin preparation; plugin cleanup precedes user cleanup to preserve that context.
- Plugins isolate state between attempts.
- Model inputs retain preparation information independently of comparison scope.
- Observation must be independent of expected values so unexpected application state remains detectable.
- Model-to-application mapping requires human review; passing tests cannot establish that setup and observation faithfully represent the model.
