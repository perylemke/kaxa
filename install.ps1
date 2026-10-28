# Install kaxa from a GitHub release on Windows.
#
#   irm https://github.com/perylemke/kaxa/releases/latest/download/install.ps1 | iex
#
# Environment:
#   VERSION      Release to install, without the leading v (default: latest).
#   INSTALL_DIR  Where to put kaxa.exe (default: %USERPROFILE%\bin).
#
# The archive is checked against the release checksums.txt before anything is
# installed. Nothing here needs administrator rights. The work happens in a
# function that is called on the last line, so a cut download runs nothing.

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RepoUrl = "https://github.com/perylemke/kaxa"

function Fail([string]$Message) {
    throw "kaxa install: $Message"
}

function Get-KaxaArch {
    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($arch) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { Fail "unsupported architecture: $arch" }
    }
}

# The version ends up in a URL, so only characters that can appear in a
# release tag are accepted.
function Get-KaxaVersion {
    $version = $env:VERSION
    if (-not $version) {
        $resp = Invoke-WebRequest -Uri "$RepoUrl/releases/latest" -Method Head -UseBasicParsing
        $uri = if ($resp.BaseResponse.ResponseUri) { $resp.BaseResponse.ResponseUri } else { $resp.BaseResponse.RequestMessage.RequestUri }
        if (-not $uri -or $uri.AbsoluteUri -notmatch "/tag/v(.+)$") {
            Fail "could not determine the latest version"
        }
        $version = $Matches[1]
    }
    $version = $version.TrimStart("v")
    if ($version -notmatch "^[0-9A-Za-z.+-]+$") {
        Fail "invalid version: $version"
    }
    return $version
}

# Fails closed: a missing entry or a different hash stops the install.
function Test-Checksum([string]$Dir, [string]$File) {
    $line = Get-Content (Join-Path $Dir "checksums.txt") |
        Where-Object { ($_ -split "\s+")[1] -eq $File } |
        Select-Object -First 1
    if (-not $line) {
        Fail "no checksum for $File in checksums.txt"
    }
    $expected = ($line -split "\s+")[0]
    $actual = (Get-FileHash -Path (Join-Path $Dir $File) -Algorithm SHA256).Hash
    if ($actual -ne $expected) {
        Fail "checksum mismatch for $File"
    }
    Write-Host "${File}: OK"
}

function Add-ToUserPath([string]$Dir) {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $entries = @($userPath -split ";" | Where-Object { $_ } | ForEach-Object { $_.TrimEnd("\") })
    if ($entries -contains $Dir.TrimEnd("\")) {
        return
    }
    $newPath = if ($userPath) { "$userPath;$Dir" } else { $Dir }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    $env:Path = "$env:Path;$Dir"
    Write-Host "Added $Dir to your user PATH. Open a new terminal for other windows to see it."
}

function Install-Kaxa {
    # Windows PowerShell 5.1 does not enable TLS 1.2 by default.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $arch = Get-KaxaArch
    $version = Get-KaxaVersion
    $installDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:USERPROFILE "bin" }
    $file = "kaxa_${version}_windows_${arch}.zip"
    $base = "$RepoUrl/releases/download/v$version"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Installing kaxa $version (windows/$arch) to $installDir"
        Invoke-WebRequest -Uri "$base/$file" -OutFile (Join-Path $tmp $file) -UseBasicParsing
        Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt") -UseBasicParsing
        Test-Checksum $tmp $file

        $extract = Join-Path $tmp "extract"
        Expand-Archive -Path (Join-Path $tmp $file) -DestinationPath $extract -Force
        New-Item -ItemType Directory -Force -Path $installDir | Out-Null
        # Copy next to the target, then rename, so a half-copied kaxa.exe is never left behind.
        $staged = Join-Path $installDir ".kaxa.exe.new"
        Copy-Item (Join-Path $extract "kaxa.exe") $staged -Force
        Move-Item $staged (Join-Path $installDir "kaxa.exe") -Force
    }
    finally {
        Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    Write-Host "Installed $(Join-Path $installDir 'kaxa.exe')"
    Add-ToUserPath $installDir
}

Install-Kaxa
