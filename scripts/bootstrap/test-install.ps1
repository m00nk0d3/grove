#Requires -Version 5.1
$ErrorActionPreference = "Stop"

$root = Join-Path $env:TEMP "grove-installer-test-$([System.IO.Path]::GetRandomFileName())"
$sourceRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$fixture = Join-Path $root "fixture"
$package = Join-Path $root "package"
$installDir = Join-Path $root "install"
$dataDir = Join-Path $root "data"
$stateDir = Join-Path $root "state"
$fakeBin = Join-Path $root "fake-bin"
$archive = "grove_1.2.3_windows_amd64.zip"
$archivePath = Join-Path $fixture $archive
$checksumsPath = Join-Path $fixture "checksums.txt"
$runtimeSource = Join-Path $root "sandcastle.js"

function New-Fixture([string]$Marker) {
    if (Test-Path $package) { Remove-Item -Recurse -Force $package }
    New-Item -ItemType Directory -Path (Join-Path $package "runtime\sandcastle\dist") -Force | Out-Null
    go build -ldflags "-X github.com/m00nk0d3/grove/internal/version.Version=1.2.3" -o (Join-Path $package "grove.exe") ./cmd/grove
    Set-Content -Encoding ASCII -Path (Join-Path $package "runtime\sandcastle\dist\sandcastle.js") -Value $Marker
    Set-Content -Encoding ASCII -Path (Join-Path $package "runtime\sandcastle\package.json") -Value '{"name":"@grove/sandcastle-runtime","version":"1.2.3"}'
    if (Test-Path $archivePath) { Remove-Item -Force $archivePath }
    Compress-Archive -Path (Join-Path $package "*") -DestinationPath $archivePath
    $checksum = (Get-FileHash -Algorithm SHA256 -Path $archivePath).Hash.ToLowerInvariant()
    Set-Content -Encoding ASCII -Path $checksumsPath -Value "$checksum  $archive"
    Copy-Item (Join-Path $package "runtime\sandcastle\dist\sandcastle.js") $runtimeSource -Force
}

New-Item -ItemType Directory -Path $fixture, $installDir, (Join-Path $dataDir "deps\node"), $stateDir, $fakeBin -Force | Out-Null
Set-Content -Encoding ASCII -Path (Join-Path $stateDir "config.toml") -Value 'theme = "midnight"'
Set-Content -Encoding ASCII -Path (Join-Path $stateDir "grove.db") -Value "sqlite-state"
Set-Content -Encoding ASCII -Path (Join-Path $stateDir "dismissed.json") -Value '["workflow-1"]'
$expectedState = @{}
foreach ($name in "config.toml", "grove.db", "dismissed.json") {
    $expectedState[$name] = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $stateDir $name)).Hash
}

Copy-Item (Get-Command cmd.exe).Source (Join-Path $dataDir "deps\node\node.exe")
@'
@echo off
set PREFIX=
:loop
if "%~1"=="" goto install
if "%~1"=="--prefix" (
  set PREFIX=%~2
  shift
)
shift
goto loop
:install
mkdir "%PREFIX%\node_modules\@grove\sandcastle-runtime\dist" 2>nul
copy /y "%GROVE_TEST_RUNTIME_SOURCE%" "%PREFIX%\node_modules\@grove\sandcastle-runtime\dist\sandcastle.js" >nul
'@ | Set-Content -Encoding ASCII -Path (Join-Path $dataDir "deps\node\npm.cmd")
'@echo off' | Set-Content -Encoding ASCII -Path (Join-Path $fakeBin "herdr.cmd")

$oldLocation = Get-Location
$oldPath = $env:Path
try {
    Set-Location $sourceRoot
    $env:Path = "$fakeBin;$oldPath"
    $env:GROVE_VERSION = "v1.2.3"
    $env:GROVE_INSTALL_DIR = $installDir
    $env:GROVE_DATA_DIR = $dataDir
    $env:GROVE_STATE_DIR = $stateDir
    $env:GROVE_ARCHIVE_URL = $archivePath
    $env:GROVE_CHECKSUMS_URL = $checksumsPath
    $env:GROVE_SKIP_PATH_UPDATE = "1"
    $env:GROVE_TEST_RUNTIME_SOURCE = $runtimeSource

    New-Fixture "runtime-v1"
    & (Join-Path $PSScriptRoot "install.ps1")
    if ($LASTEXITCODE -ne 0) { throw "Fresh install failed." }

    New-Fixture "runtime-v2"
    & (Join-Path $PSScriptRoot "install.ps1")
    if ($LASTEXITCODE -ne 0) { throw "Reinstall failed." }

    $installedRuntime = Join-Path $dataDir "sandcastle\node_modules\@grove\sandcastle-runtime\dist\sandcastle.js"
    if ((Get-Content -Raw $installedRuntime).Trim() -ne "runtime-v2") {
        throw "Reinstall did not replace the Sandcastle runtime."
    }
    foreach ($name in $expectedState.Keys) {
        $actual = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $stateDir $name)).Hash
        if ($actual -ne $expectedState[$name]) { throw "Installer changed $name." }
    }
    foreach ($command in "grove-sandcastle", "grove-lab", "imp", "agent-flow", "review", "resolve", "ci", "clean", "address") {
        if (-not (Test-Path (Join-Path $installDir "$command.cmd"))) { throw "Missing command: $command" }
    }

    $binaryHash = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $installDir "grove.exe")).Hash
    Set-Content -Encoding ASCII -Path (Join-Path $stateDir "config.toml") -Value "[broken"
    $rejected = $false
    try {
        & (Join-Path $PSScriptRoot "install.ps1")
    }
    catch {
        $rejected = $true
    }
    if (-not $rejected) { throw "Installer accepted an incompatible configuration." }
    $binaryHashAfter = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $installDir "grove.exe")).Hash
    if ($binaryHashAfter -ne $binaryHash) { throw "Rejected install changed grove.exe." }

    Write-Host "Windows installer behavior verified"
}
finally {
    Set-Location $oldLocation
    $env:Path = $oldPath
    Remove-Item -Recurse -Force $root -ErrorAction SilentlyContinue
}
