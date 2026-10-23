## Summary

<!-- What changed and why (1–3 bullets). -->

-

## Test plan

- [ ] Unit: `go test -race -shuffle=on ./...`
- [ ] Integration: `go test -race -count=1 -tags=integration ./internal/config/...`
- [ ] E2E: `go test -race -count=1 -tags=e2e ./test/e2e/...`
- [ ] Acceptance: `go test -race -count=1 -tags=acceptance ./test/acceptance/...`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Vulnerabilities: `govulncheck ./...`

## Security

Do not paste Sprites or Modal tokens in this PR. Use placeholders in config examples only.
