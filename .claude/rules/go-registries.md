---
paths:
  - "**/*.go"
---

# Registries

- New sandbox backends register via `registry.Register` in `main`, not scattered `switch provider` on string names.
- Provider `Name()` returns a stable lowercase identifier (`sprites`, `modal`, `e2b`).
- Lookup uses `registry.Get(name)`; unknown names return a clear error at config validation time when possible.
- Do not hardcode provider lists in help text; derive from `registry.Names()` when listing supported providers.
