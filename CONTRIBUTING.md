# Contributing to kaxa

Thank you for contributing to **kaxa**. We appreciate your time and help.
Here are some guidelines to help you get started.

## Code of Conduct

Be kind and respectful to members of the community. Take time to educate
others who are seeking help. Harassment of any kind will not be tolerated.

## Questions

If you have questions about kaxa, open a [GitHub Discussion](https://github.com/perylemke/kaxa/discussions) or a [GitHub Issue](https://github.com/perylemke/kaxa/issues) with a clear title and as much context as you need.

## Filing a bug or feature

1. Before filing an issue, check [existing issues](https://github.com/perylemke/kaxa/issues) for duplicates. If one exists, comment there instead of opening a new issue.
1. For bugs, include detailed steps to reproduce, the output of `kaxa version`, your OS, and anything else useful (environment variables **names** only—never paste Sprites or Modal tokens). Describe current behavior vs. expected behavior.
1. For features or enhancements, use a clear title and description of what you want and why it would help the project and its users.

## Submitting changes

1. **Tests:** Include tests for new behavior. Run `make test` at minimum; before opening a PR, run `make ci` or the checks listed in [.github/pull_request_template.md](.github/pull_request_template.md).
1. **Format and lint:** Run `make fmt` and `make lint` so Go code stays consistent.
1. **Changelog:** When a change is visible to users (new command or flag, behavior change, fix, security fix), add a line under `## [Unreleased]` in [CHANGELOG.md](CHANGELOG.md), in the matching `Added`, `Changed`, `Fixed`, `Removed`, or `Security` subsection.
1. **Commits:** Use [Conventional Commits](https://www.conventionalcommits.org/)—one line, English, no trailing period (e.g. `feat: add sprites list command`, `fix: validate provider name`).

### Quick steps to contribute

1. Fork the project.
1. Clone your fork (`git clone https://github.com/your_username/kaxa.git && cd kaxa`)
1. Create a feature branch (`git checkout -b my-new-feature`)
1. Make changes and run tests (`make test`)
1. Stage changes (`git add .`)
1. Commit (`git commit -m 'feat: add some feature'`)
1. Push (`git push origin my-new-feature`)
1. Open a pull request against `main`

Use the PR template and confirm you did not include real API tokens in the description or in committed files.

## Development prerequisites

- Go 1.27+ (see [go.mod](go.mod))
- `golangci-lint`, `govulncheck`, and `gosec` v2.30.0 for local CI parity (`make lint`, `make vuln`, `make gosec`)
- [semtag](https://github.com/nico2sh/semtag) 0.2.0 (`brew install semtag`) and push access to `origin`, only for maintainers who cut releases

Run `make help` for all Makefile targets.

## Releases

Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and are tagged with semtag through `make`. Pushing a tag starts [.github/workflows/release.yml](.github/workflows/release.yml), which publishes the GitHub release with GoReleaser.

| Tag | Command | GitHub release | Homebrew cask |
| --- | --- | --- | --- |
| `vX.Y.Z` (final) | `make release SCOPE=minor` | Release, notes from `CHANGELOG.md` | Updated |
| `vX.Y.Z-rc.N`, `-beta.N`, `-alpha.N` (test) | `make prerelease SCOPE=minor TYPE=rc` | Pre-release, notes from commits | Not updated |

`SCOPE` is `major`, `minor`, or `patch`, and is always required. `TYPE` is `alpha`, `beta`, or `rc` (default `rc`). `make next-version SCOPE=minor` prints the next final version without tagging anything.

### Final version

1. `main` is protected, so prepare the changelog in a pull request. From an up-to-date branch, run `make changelog-release SCOPE=minor`. It moves the `[Unreleased]` entries into `## [X.Y.Z] - date` and updates the links at the end of the file. Review it, commit (`docs: update changelog for vX.Y.Z`), and merge the PR.
1. Update `main` locally (`git switch main && git pull`) and run `make release SCOPE=minor` with the same `SCOPE`. The command stops unless you are on `main`, the tree is clean, `main` equals `origin/main`, and `CHANGELOG.md` already has the section for the next version.
1. The workflow checks the tag is on `main`, uses that changelog section as the release notes, and updates the cask in `perylemke/homebrew-tap`.

### Test version

1. Push the commit you want to try (any branch), then run `make prerelease SCOPE=minor TYPE=rc` from it. The next `-rc.N` is counted from the existing tags, and the command stops when the commit is not on `origin`.
1. The workflow publishes a GitHub pre-release and skips Homebrew. Users install it with the script from that release; see [Test versions](docs/install.md#test-versions).

semtag orders pre-release names as text and requires them to increase, so `alpha` comes before `beta`, `beta` before `rc`, and a version allows up to 9 pre-releases per type. After that, cut a new version.

Creating a `v*` tag is restricted by the GitHub ruleset "Release tags" (Settings → Rules) to repository admins, so only maintainers can start a release. Deleting that ruleset removes the restriction.

A wrong tag can be undone with `git push origin :refs/tags/vX.Y.Z-rc.N`, `git tag -d vX.Y.Z-rc.N`, and deleting the GitHub release. Never reuse a final version.

## Install scripts

`install.sh` (POSIX `sh`) and `install.ps1` are attached to every release. When you change one, update the other and `docs/install.md`, and test it against a real release with a temporary `HOME`. Both must verify the archive hash and fail closed.

## AI agent rules

Rules for AI agents live in `.cursor/rules/*.mdc` (Cursor) and `.claude/rules/*.md` plus `CLAUDE.md` (Claude Code). When you change one, update the other so they stay in sync.
