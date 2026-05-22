#Requires -Version 5.1
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$Repo     = "m00nk0d3/nexus"
$Binary   = "nexus"
$InstallDir = Join-Path $env:LOCALAPPDATA "nexus"

Write-Host "Fetching latest Nexus release..."

# Fetch latest release version
$apiUrl  = "https://api.github.com/repos/$Repo/releases/latest"
$release = Invoke-RestMethod -Uri $apiUrl -UseBasicParsing
$tag     = $release.tag_name          # e.g. "v0.5.0"
$version = $tag.TrimStart("v")

# Only amd64 Windows builds are produced by GoReleaser
$archive = "${Binary}_${version}_windows_amd64.zip"
$url     = "https://github.com/$Repo/releases/download/$tag/$archive"

Write-Host "Installing nexus v$version for windows/amd64..."

$tmp     = Join-Path $env:TEMP "nexus-install-$([System.IO.Path]::GetRandomFileName())"
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    $zipPath = Join-Path $tmp $archive
    Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing

    Expand-Archive -Path $zipPath -DestinationPath $tmp -Force

    # Create install directory
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }

    Copy-Item -Path (Join-Path $tmp "$Binary.exe") -Destination (Join-Path $InstallDir "$Binary.exe") -Force

    # Add to user PATH if not already present
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        Write-Host "Added $InstallDir to user PATH."
        Write-Host "Restart your terminal for PATH changes to take effect."
    }

    Write-Host ""
    Write-Host "✓ nexus v$version installed to $InstallDir\$Binary.exe"
    Write-Host ""
    Write-Host "Run: nexus"
    Write-Host "Docs: https://github.com/$Repo"
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
