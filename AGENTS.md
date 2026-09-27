# Go API design

1. Follow the Google Go Style Guide, including its Style Decisions and Best Practices.
2. When returning `(T, error)`, return a nil or zero result with a non-nil error unless a meaningful partial result is explicitly part of the API contract.
3. Except for data carriers (structs whose sole purpose is to transport fields across an API boundary) and function-local temporary structs, every named struct type must have a `NewXxx` smart constructor that establishes and validates its invariants. Put the type, constructor, methods, and type-specific helpers in one file. `NewXxx` must return `*Xxx`, and all methods must use pointer receivers.
4. Prefer Composed Method: write each non-trivial function as well-named operations at a single level of abstraction, extracting by responsibility rather than length.
5. Apply semantic vertical formatting: keep related declarations and statements together, separate distinct concepts, and order code from higher-level behavior to details.

# Markdown

1. Follow the Google Markdown Style Guide, except for its 80-character line limit for prose.
2. Do not hard-wrap prose. Keep each paragraph and list item on a single source line unless a line break is semantically required.

# Repository architecture

- `spec-guardian`
  - Owns the CLI and extractor. Keep commands in `cmd` and non-public code in `internal`.
  - Return internal errors unchanged; wrap external errors with context using `github.com/cockroachdb/errors`.
- `guardian`
  - Owns the public fault-injection runtime and official I/O hooks. Keep dependencies minimal.
  - Hooks return wrapped-library values and errors unchanged. Guardian-created errors are allowed.
- Use relative paths in local `replace` directives.

<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->
