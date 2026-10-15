SpecGuardian checks whether an application conforms to a FizzBee model by generating tests that execute application actions, inject I/O faults, and compare observed outcomes with those allowed by the model.

# Rules

## Agent behavior

- Act only on explicit user instructions.
- Before making changes, inspect the existing implementation and agree on the approach and concrete steps.
- If the proposed approach would require awkward workarounds or structural compromises, explain the concerns and discuss alternatives, even when the approach was requested by the user.
- After editing, check that the result serves its intended purpose and follows the applicable guidelines.

## Guidelines

- [Project Architecture](docs/agents/architecture.md)
- [Go API design](docs/agents/go.md)
- [Tests](docs/agents/test.md)
- [Markdown](docs/agents/markdown.md)
- [Documentation](docs/agents/document.md)

<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->
