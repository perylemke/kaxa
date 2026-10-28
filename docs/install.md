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

## Download with curl

Download the release archive, check it against `checksums.txt`, and put the `kaxa` binary in a `bin` folder of your user account. No `sudo` is needed. Releases cover Linux, macOS, and Windows on `amd64` and `arm64`.

The commands install the latest release. Set `VERSION` (without the `v`, for example `0.0.0-rc.1`) to install another one.

### Linux and macOS

```bash
(
set -eu
VERSION="${VERSION:-}"
if [ -z "$VERSION" ]; then
  url=$(curl -fsSLI -o /dev/null -w "%{url_effective}" https://github.com/perylemke/kaxa/releases/latest)
  VERSION="${url##*/v}"
fi
OS=$(uname -s | tr "[:upper:]" "[:lower:]")
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "unsupported architecture" >&2; exit 1 ;;
esac
FILE="kaxa_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/perylemke/kaxa/releases/download/v${VERSION}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
curl -fsSL -o "$TMP/$FILE" "$BASE/$FILE"
curl -fsSL -o "$TMP/checksums.txt" "$BASE/checksums.txt"
cd "$TMP"
if command -v sha256sum >/dev/null 2>&1; then SUM="sha256sum -c -"; else SUM="shasum -a 256 -c -"; fi
grep "  ${FILE}\$" checksums.txt | $SUM
mkdir -p "$HOME/.local/bin"
tar -xzf "$FILE" -C "$HOME/.local/bin" kaxa
chmod +x "$HOME/.local/bin/kaxa"
)
```

The subshell keeps the variables and `set -eu` out of your terminal. The `grep` line prints `<archive>: OK`; the install stops if the hash does not match.

Make sure `~/.local/bin` is on your `PATH`. Add this line to `~/.zshrc` or `~/.bashrc` when it is not:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

On macOS, `curl` does not set the quarantine flag, so Gatekeeper does not block the binary. If you downloaded the archive with a browser instead, remove the flag as described in [Gatekeeper](#gatekeeper).

### Windows (PowerShell)

```powershell
$ErrorActionPreference = "Stop"
$version = if ($env:VERSION) { $env:VERSION } else { (Invoke-RestMethod https://api.github.com/repos/perylemke/kaxa/releases/latest).tag_name.TrimStart("v") }
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$file = "kaxa_${version}_windows_${arch}.zip"
$base = "https://github.com/perylemke/kaxa/releases/download/v$version"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $tmp | Out-Null
curl.exe -fsSL -o "$tmp\$file" "$base/$file"
curl.exe -fsSL -o "$tmp\checksums.txt" "$base/checksums.txt"
$expected = ((Select-String -Path "$tmp\checksums.txt" -Pattern ("  " + [regex]::Escape($file) + "$")).Line -split "\s+")[0]
$actual = (Get-FileHash "$tmp\$file" -Algorithm SHA256).Hash.ToLower()
if ($actual -ne $expected) { throw "checksum mismatch for $file" }
$bin = Join-Path $env:USERPROFILE "bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null
Expand-Archive -Path "$tmp\$file" -DestinationPath $tmp -Force
Copy-Item "$tmp\kaxa.exe" $bin -Force
Remove-Item $tmp -Recurse -Force
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $bin) {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$bin", "User")
}
```

The last step adds `%USERPROFILE%\bin` to your user `PATH`. Open a new terminal for it to take effect.

### Verify, update, and remove

```bash
kaxa version
```

To update, run the same commands again; they overwrite the binary. To remove kaxa:

```bash
rm ~/.local/bin/kaxa
```

```powershell
Remove-Item "$env:USERPROFILE\bin\kaxa.exe"
```

The checksum comes from the same release as the archive. It catches a corrupted or truncated download, but it does not protect you from a compromised release. kaxa binaries are not signed.

## From source

Requires Go 1.27 or newer.

```bash
go install ./cmd/kaxa
```

`go install` writes the binary to `$(go env GOPATH)/bin/kaxa`, or to `$GOBIN` when that variable is set. `make build` writes `./bin/kaxa`.
