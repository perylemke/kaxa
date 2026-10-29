## Summary

<!-- What changed and why (1–3 bullets). -->

-

## Test plan

- [ ] Unit: `go test -race -shuffle=on ./...`
- [ ] Integration: `go test -race -count=1 -tags=integration ./internal/config/... ./internal/provider/...`
- [ ] E2E: `go test -race -count=1 -tags=e2e ./test/e2e/...`
- [ ] Acceptance: `go test -race -count=1 -tags=acceptance ./test/acceptance/...`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Vulnerabilities: `govulncheck ./...`
- [ ] SAST: `make gosec`
- [ ] CHANGELOG.md updated under `[Unreleased]` (skip only for changes users never see)

## Security

Do not paste Sprites or Modal tokens in this PR.
