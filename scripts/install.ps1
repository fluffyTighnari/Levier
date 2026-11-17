# Levier — install a release binary on Windows.
#
# Usage:
#   .\scripts\install.ps1
#   .\scripts\install.ps1 -Version v0.1.0 -Prefix "$env:USERPROFILE\.local\bin"
#   .\scripts\install.ps1 -Version v0.1.0 -BaseUrl http://127.0.0.1:8000
[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$Prefix = "",
    [string]$Repo = "fluffyTighnari/levier",
    [string]$BaseUrl = "",
    [switch]$DryRun,
    [switch]$Help
)

$ErrorActionPreference = "Stop"

function Show-Usage {
    @"
usage: install.ps1 [options]

options:
  -Version <tag>      install a specific release, for example v0.1.0
  -Prefix <dir>       install directory; defaults to %USERPROFILE%\.local\bin
  -Repo <owner/repo>  GitHub repository; default fluffyTighnari/levier
  -BaseUrl <url>      mirror/release directory URL; requires -Version
  -DryRun             print the plan without downloading or writing
  -Help               show this help

The installer always downloads checksums.txt and verifies the archive before
installing. It does not edit the user PATH.
"@
}

function Fail([string]$Message) {
    [Console]::Error.WriteLine("install: $Message")
    exit 2
}

function FailRuntime([string]$Message) {
    [Console]::Error.WriteLine("install: $Message")
    exit 1
}

if ($Help) {
    Show-Usage
    exit 0
}

if ($Version -eq "latest") {
    $Version = ""
}
if ($Version -and -not $Version.StartsWith("v")) {
    $Version = "v$Version"
}

if (-not $Repo.Contains("/") -or ($Repo -split "/").Count -ne 2) {
    Fail "repository must look like owner/name"
}
if ($BaseUrl -and -not $Version) {
    Fail "-BaseUrl requires -Version"
}

$arch = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) {
    $arch = $env:PROCESSOR_ARCHITEW6432
}
if (-not $arch) {
    Fail "could not detect processor architecture"
}
switch ($arch.ToUpperInvariant()) {
    "AMD64" { $arch = "amd64" }
    "ARM64" { Fail "windows/arm64 release is not published yet" }
    default { Fail "unsupported architecture: $arch" }
}

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
} catch {
    # PowerShell 7+ uses the platform TLS stack.
}

if (-not $Version) {
    $apiUrl = "https://api.github.com/repos/$Repo/releases/latest"
    $release = Invoke-RestMethod -Uri $apiUrl -Headers @{ "User-Agent" = "levier-install" }
    $Version = $release.tag_name
    if (-not $Version) {
        FailRuntime "could not resolve latest release; pass -Version"
    }
}

if ($BaseUrl) {
    $releaseBase = $BaseUrl.TrimEnd("/")
} else {
    $releaseBase = "https://github.com/$Repo/releases/download/$Version"
}

$archive = "levier_${Version}_windows_${arch}.zip"

if (-not $Prefix) {
    $Prefix = Join-Path $env:USERPROFILE ".local\bin"
} elseif ($Prefix -eq "~") {
    $Prefix = $env:USERPROFILE
} elseif ($Prefix.StartsWith("~\")) {
    $Prefix = Join-Path $env:USERPROFILE $Prefix.Substring(2)
}
if (-not [IO.Path]::IsPathRooted($Prefix)) {
    $Prefix = Join-Path (Get-Location) $Prefix
}
$Prefix = [IO.Path]::GetFullPath($Prefix)

Write-Host "repository: $Repo"
Write-Host "version:    $Version"
Write-Host "platform:   windows/$arch"
Write-Host "archive:    $archive"
Write-Host "url:        $releaseBase/$archive"
Write-Host "prefix:     $Prefix"

if ($DryRun) {
    Write-Host "dry-run:    no download or write"
    exit 0
}

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("levier-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $archivePath = Join-Path $tmp $archive
    $checksumsPath = Join-Path $tmp "checksums.txt"

    Invoke-WebRequest -Uri "$releaseBase/$archive" -OutFile $archivePath -UseBasicParsing
    Invoke-WebRequest -Uri "$releaseBase/checksums.txt" -OutFile $checksumsPath -UseBasicParsing

    $expected = $null
    foreach ($line in Get-Content $checksumsPath) {
        $parts = $line -split "\s+", 2
        if ($parts.Count -eq 2 -and $parts[1] -eq $archive) {
            $expected = $parts[0]
            break
        }
    }
    if (-not $expected) {
        FailRuntime "checksums.txt does not contain $archive"
    }

    $actual = (Get-FileHash -Algorithm SHA256 -Path $archivePath).Hash.ToLowerInvariant()
    if ($actual -ne $expected.ToLowerInvariant()) {
        FailRuntime "checksum mismatch for ${archive}: expected $expected, got $actual"
    }

    $extract = Join-Path $tmp "extract"
    Expand-Archive -Path $archivePath -DestinationPath $extract -Force
    $sourceBin = Join-Path $extract "levier.exe"
    if (-not (Test-Path $sourceBin -PathType Leaf)) {
        FailRuntime "archive does not contain levier.exe"
    }

    New-Item -ItemType Directory -Force -Path $Prefix | Out-Null
    $target = Join-Path $Prefix "levier.exe"
    $staged = Join-Path $Prefix (".levier.tmp." + [Guid]::NewGuid().ToString("N"))
    Copy-Item -Path $sourceBin -Destination $staged -Force
    Move-Item -Path $staged -Destination $target -Force

    Write-Host "installed:  $target"
    $pathParts = $env:Path -split ";"
    if (-not ($pathParts | Where-Object { $_.TrimEnd("\") -eq $Prefix.TrimEnd("\") })) {
        Write-Host ""
        Write-Host "PATH hint:"
        Write-Host "  Add this directory to your user PATH:"
        Write-Host "  $Prefix"
    }
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
