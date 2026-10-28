# Install kaxa

## Homebrew (macOS)

kaxa is published as a cask in the `perylemke/tap` tap. Each tagged release updates the cask.

```bash
brew install --cask perylemke/tap/kaxa
kaxa version
```

Upgrade and uninstall:

```bash
brew upgrade --cask kaxa
brew uninstall --cask kaxa
```

The cask downloads the darwin archive of the release (arm64 or amd64) and checks its SHA-256 before it installs anything. The hash comes from the same GoReleaser run that publishes `checksums.txt`. Homebrew stops when the download does not match.

Homebrew links the binary at `$(brew --prefix)/bin/kaxa`, for example `/opt/homebrew/bin/kaxa` on Apple Silicon and `/usr/local/bin/kaxa` on Intel. Use that absolute path as `command` in a desktop MCP client; see [mcp](agent.md).

Pre-release tags (for example `v0.0.0-rc.1`) also update the cask, so `brew upgrade` can install a release candidate.

### Gatekeeper

The kaxa binary is not signed or notarized, so macOS Gatekeeper can block a binary downloaded from the internet. After install, the cask removes the `com.apple.quarantine` attribute from the binary with `xattr`. Nothing else is changed.

If you download an archive from the [releases page](https://github.com/perylemke/kaxa/releases) yourself, verify it against `checksums.txt`, then remove the flag by hand:

```bash
xattr -d com.apple.quarantine /path/to/kaxa
```

## From source

Requires Go 1.27 or newer.

```bash
go install ./cmd/kaxa
```

`go install` writes the binary to `$(go env GOPATH)/bin/kaxa`, or to `$GOBIN` when that variable is set. `make build` writes `./bin/kaxa`.
