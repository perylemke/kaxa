---
paths:
  - "**/*.go"
---

# Security

- Never log, print, or include tokens in error messages (`sprites.token`, `modal.token_id`, `modal.token_secret`, `e2b.api_key`). E2B daemon access tokens count as tokens too; the e2b package redacts them, and sandbox URLs are private unless `--api` opens the API port.
- Do not execute user input via shell (`sh -c`); when exec is implemented, pass argv slices only.
- Store secrets in environment variables only; avoid long-lived secrets in command-line flags.
- Validate provider names before use.
