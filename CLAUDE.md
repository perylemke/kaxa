# CLAUDE.md

Guidance for AI agents working in `kaxa` (Go CLI that unifies sandbox providers: Fly.io Sprites, Modal, and E2B).
Cursor users: the same rules live in `.cursor/rules/*.mdc`; Claude Code reads `.claude/rules/*.md`. Keep both in sync.

## Stack

- Go 1.27+ (see `go.mod`), Cobra CLI, `CGO_ENABLED=0` builds.
- Providers: Sprites, Modal, and E2B (stdlib REST and Connect-JSON client, no SDK), registered in `internal/registry`.
- `kaxa mcp` serves sandboxes to AI agents over the Model Context Protocol.
- Releases: semtag tags (`make release`, `make prerelease`), GoReleaser (`.goreleaser.yml`), and the Homebrew cask in `perylemke/homebrew-tap`. Only final `vX.Y.Z` tags reach Homebrew; `-alpha/-beta/-rc` tags are GitHub pre-releases.

## Commands

```bash
make build # build ./bin/kaxa
make test # unit tests (-race -shuffle=on)
make test-integration # integration tests with fakes (-tags=integration), free
make coverage # coverage of unit and integration tests (coverage.out); coverage-html adds coverage.html
make test-live # real provider APIs (-tags=live); needs credentials and bills compute, never part of make ci
make test-e2e # end-to-end tests (-tags=e2e)
make test-acceptance # acceptance tests (-tags=acceptance)
make fmt # golangci-lint fmt (gofmt + gofumpt)
make lint # golangci-lint
make vuln # govulncheck
make gosec # gosec SAST
make ci # everything CI runs
make next-version SCOPE=minor # print the next final version, tags nothing
make changelog-release SCOPE=minor # move [Unreleased] into the next version of CHANGELOG.md
make release SCOPE=minor # tag and push a final version from main (needs semtag)
make prerelease SCOPE=minor TYPE=rc # tag and push a test version (alpha|beta|rc), not on Homebrew
```

Run `make test` at minimum, and `make lint` after Go changes, before saying work is done. Report the real result.

## Layout

- `cmd/kaxa/main.go` — builds the registry and calls `cli.Execute`; no business logic.
- `internal/cli` — Cobra commands.
- `internal/config` — environment and flag configuration.
- `internal/sandbox` — provider contracts; `internal/registry` — provider registration.
- `internal/provider/{sprites,modal,e2b}` — one package per provider.
- `internal/{mcp,lifecycle,reposync,project,toolchain,usererr,version}` — MCP server, sandbox lifecycle, repo sync, project detection, toolchain install, user errors, version info.
- `install.sh`, `install.ps1` — one-line installers, attached to every release by GoReleaser (`release.extra_files`); keep in sync with `docs/install.md`.
- `CHANGELOG.md` — Keep a Changelog; `internal/changelog` and `tools/changelog` edit it at release time; `scripts/release.sh` wraps semtag.
- `docs/` — one manual per command; keep in sync with `README.md`.
- `test/e2e`, `test/acceptance` — build-tagged tests.

## Rules

Detailed rules are in `.claude/rules/` (mirrors of `.cursor/rules/`):

- `go-style.md`, `go-tdd.md`, `go-srp.md`, `go-registries.md`, `go-security.md` — load for `**/*.go`.
- `commits.md`, `owasp.md`, `releases.md` — always apply.

Highlights:

- Test first for new behavior; table-driven tests with `t.Run`; `t.Setenv`/`t.TempDir` instead of global env or fixed paths.
- Wrap errors with `%w`; stdout for command output, stderr for errors.
- Never log or print tokens; secrets come from `KAXA_*` environment variables only.
- No `sh -c` with user input; use `exec.Command` with argv.
- Register providers via `registry.Register`; never hardcode provider lists.

## Working agreements

- Keep changes minimal and scoped; do not refactor unrelated code.
- Any command or flag change must update `README.md` and the matching `docs/*.md`.
- Avoid new dependencies; keep `go.mod` and `go.sum` consistent (`make tidy`).
- Any user-visible change adds a line under `## [Unreleased]` in `CHANGELOG.md`; never edit released sections by hand.
- Never tag or push a release unless asked; use `make release` or `make prerelease`, not `git tag`.
- Never commit `.env` or real tokens.
- Commits (only when asked): Conventional Commits, one line, English, no trailing period (`feat: add sprites provider stub`).
