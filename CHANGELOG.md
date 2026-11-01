# Changelog

All notable changes to kaxa are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Add an entry under `[Unreleased]` in every pull request that changes what users see. `make changelog-release` moves those entries into the new version when a final version is cut. Pre-releases (`-alpha.N`, `-beta.N`, `-rc.N`) have no section here.

## [Unreleased]

### Added

- Sandbox providers: Fly.io Sprites, Modal, and E2B, behind one CLI. E2B supports create (with `--sync` and `--api`), list, exec, console, restart, sync, and destroy, and reads `KAXA_E2B_API_KEY`.
  - E2B sandboxes can start from your own template with `create --template`, `KAXA_E2B_TEMPLATE`, or `template` in the MCP `create_sandbox`.
- Docker Sandboxes in Docker's cloud as the `docker` provider, through the experimental Docker Sandboxes API and a Docker ID with a personal access token (`KAXA_DOCKER_ID`, `KAXA_DOCKER_PAT`). It supports create (with `--sync`, `--template`, and `--api`), list, exec, console, restart, sync, snapshot, and destroy, starts from `KAXA_DOCKER_TEMPLATE` or `--template`, and needs a Docker Agentic Platform subscription. `create --api` starts an HTTP service and publishes its port on a Docker URL. `snapshot` saves the disk of a sandbox and restores it into a new sandbox with `--to`; Docker keeps the snapshots until `snapshot delete`, and kaxa lists only its own. A restored sandbox is recorded in `docker-restored.json` in the kaxa config directory, because Docker gives it no labels, so it is visible only on the machine that restored it.
- `kaxa mcp`, which serves sandboxes to AI agents over the Model Context Protocol.
- Toolchain installation in Sprites sandboxes.
- Sandbox snapshots: create, list, restore, and delete.
- Install scripts for Linux, macOS (`install.sh`), and Windows (`install.ps1`), attached to every release and checked against `checksums.txt`.
- Homebrew cask in `perylemke/tap`, published for final versions only.
- Semantic versioning with semtag: `make release` for final versions and `make prerelease` for test versions.

- `KAXA_MCP_API_ENV` lists the host variables an agent may copy into a service with a bare `KEY` in `api_env`.
- `timeout_seconds` on the MCP `exec` tool, 30 minutes by default and 2 hours at most.
- Cosign keyless signature of `checksums.txt`, SPDX SBOMs, and build provenance attestations on every release. `install.sh` and `install.ps1` check the signature when `cosign` is installed, and `REQUIRE_SIGNATURE=1` makes it mandatory.
- Dependabot for GitHub Actions and Go modules.

### Changed

- Configuration is read from environment variables only; Viper was removed.
- `kaxa --help` lists every command in one `Commands:` list, whichever providers implement it. Command help splits `Flags:` from `Provider flags:`, and `--provider` names the registered providers.
- The project was renamed from `sbox` to `kaxa`.
- `kaxa create --api` names who can open the URL by provider: Sprites asks for a Sprites account, E2B is public, and Docker says nothing, instead of always saying "requires a Sprites account".
- `--api-env` and `api_env` reject names starting with `MODAL_TOKEN_`, `FLY_`, or `SPRITES_` as well as `KAXA_`.
- GitHub Actions are pinned to commit SHAs, the release job runs in the `release` environment with a read-only default token, and the Homebrew token reaches only the step that needs it.
- The Modal base image is pinned by digest.
- Dependency installs after `--sync` and `sync` are tried up to three times when a network error such as an HTTP/2 `GOAWAY` or a connection reset interrupts them.
- Updated `golang.org/x/net` to v0.62.0.

### Security

- gosec SARIF scan in CI.
- `kaxa url <name> public` and `create --running --public` are refused for a sandbox whose URL serves the unauthenticated tmux terminal.
- The MCP server no longer copies arbitrary host environment variables into a service, and it refuses a `sync` whose scope is a home directory or the filesystem root.
- `create --sync` removes credentials from `.git/config`, refuses untracked secret files, and no longer follows symlinks out of the repository.
- Remote command output, provider error bodies, and project files are read with size limits, provider HTTP calls have timeouts, and MCP `exec` has a deadline.
- Names and URLs from the provider are stripped of control characters before they are printed, and only `http(s)` URLs are opened in the browser.
- An API command that starts with an option instead of a binary is rejected.

[Unreleased]: https://github.com/perylemke/kaxa/commits/main
