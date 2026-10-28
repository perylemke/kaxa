---
paths:
  - "**/*.go"
---

# Style

- Keep functions short; extract when cyclomatic complexity grows.
- Wrap errors with `%w` for context; use sentinel errors only when callers need `errors.Is`.
- Use stdlib `testing` unless a dependency already exists in the module.
- Avoid premature abstractions; wait for a second real use case before generic helpers.
- Exported symbols need godoc ending with a period.
- Stdout for command output; stderr for errors and diagnostics.
