# Changelog

All notable changes to kaxa are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Add an entry under `[Unreleased]` in every pull request that changes what users see. `make changelog-release` moves those entries into the new version when a final version is cut. Pre-releases (`-alpha.N`, `-beta.N`, `-rc.N`) have no section here.

## [Unreleased]

### Added

- Sandbox providers: Fly.io Sprites and Modal, behind one CLI.
- `kaxa mcp`, which serves sandboxes to AI agents over the Model Context Protocol.
- Toolchain installation in Sprites sandboxes.
- Sandbox snapshots: create, list, restore, and delete.
- Install scripts for Linux, macOS (`install.sh`), and Windows (`install.ps1`), attached to every release and checked against `checksums.txt`.
- Homebrew cask in `perylemke/tap`, published for final versions only.
- Semantic versioning with semtag: `make release` for final versions and `make prerelease` for test versions.

### Changed

- Configuration is read from environment variables only; Viper was removed.
- The project was renamed from `sbox` to `kaxa`.

### Security

- gosec SARIF scan in CI.

[Unreleased]: https://github.com/perylemke/kaxa/commits/main
