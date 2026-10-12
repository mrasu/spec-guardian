# Go API design

1. Follow the Google Go Style Guide, including its Style Decisions and Best Practices.
2. When returning `(T, error)`, return a nil or zero result with a non-nil error unless a meaningful partial result is explicitly part of the API contract.
3. Except for data carriers (structs whose sole purpose is to transport fields across an API boundary) and function-local temporary structs, every named struct type must have a `NewXxx` smart constructor that establishes and validates its invariants. Put the type, constructor, methods, and type-specific helpers in one file. `NewXxx` must return `*Xxx`, and all methods must use pointer receivers.
4. Prefer Composed Method: write each non-trivial function as well-named operations at a single level of abstraction, extracting by responsibility rather than length.
5. Apply semantic vertical formatting: keep related declarations and statements together, separate distinct concepts, and order code from higher-level behavior to details.
6. Favor high cohesion: keep related types and helpers alongside the code they support unless they form a distinct responsibility.
7. Name functions that only convert a value `toXxx`, using the destination type for `Xxx`.
8. Wrap errors from outside the current Go module with `github.com/cockroachdb/errors`; return errors from within the module unchanged.
9. Use line breaks to make nested calls easier to read.
