#Requires -Version 5.1
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$Repo     = "m00nk0d3/grove"
$Binary   = "grove"
$InstallDir = if ($env:GROVE_INSTALL_DIR) { $env:GROVE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "grove" }
$DataDir = if ($env:GROVE_DATA_DIR) { $env:GROVE_DATA_DIR } else { $InstallDir }
$NodeDir = Join-Path $DataDir "deps\node"
$HerdrDir = Join-Path $DataDir "deps\herdr"
$RuntimePrefix = Join-Path $DataDir "sandcastle"

function Copy-Download([string]$Source, [string]$Destination) {
    if (Test-Path $Source) {
        Copy-Item -Path $Source -Destination $Destination -Force
    } else {
        Invoke-WebRequest -Uri $Source -OutFile $Destination -UseBasicParsing
    }
}

if ($env:GROVE_VERSION) {
    $version = $env:GROVE_VERSION -replace '^v', ''
    $tag = "v$version"
} else {
    Write-Host "Fetching latest Grove release..."
    $apiUrl  = "https://api.github.com/repos/$Repo/releases/latest"
    $release = Invoke-RestMethod -Uri $apiUrl -UseBasicParsing
    $tag     = $release.tag_name
    $version = $tag -replace '^v', ''
}
if ($version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$') {
    throw "Invalid Grove version: $version"
}

# Only amd64 Windows builds are produced by GoReleaser
$archive = "${Binary}_${version}_windows_amd64.zip"
$url = if ($env:GROVE_ARCHIVE_URL) { $env:GROVE_ARCHIVE_URL } else { "https://github.com/$Repo/releases/download/$tag/$archive" }
$checksumsUrl = if ($env:GROVE_CHECKSUMS_URL) { $env:GROVE_CHECKSUMS_URL } else { "https://github.com/$Repo/releases/download/$tag/checksums.txt" }

Write-Host "Installing grove v$version for windows/amd64..."

$tmp     = Join-Path $env:TEMP "grove-install-$([System.IO.Path]::GetRandomFileName())"
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    $zipPath = Join-Path $tmp $archive
    Copy-Download $url $zipPath
    $checksumsPath = Join-Path $tmp "checksums.txt"
    Copy-Download $checksumsUrl $checksumsPath
    $checksums = Get-Content -Raw -Path $checksumsPath
    $checksumMatch = [regex]::Match($checksums, "(?m)^([a-fA-F0-9]{64})  $([regex]::Escape($archive))\r?$")
    if (-not $checksumMatch.Success) {
        throw "Release checksum is missing for $archive."
    }
    $expectedChecksum = $checksumMatch.Groups[1].Value.ToLowerInvariant()
    $actualChecksum = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualChecksum -ne $expectedChecksum) {
        throw "Grove release checksum verification failed."
    }

    Expand-Archive -Path $zipPath -DestinationPath $tmp -Force

    $stateDir = if ($env:GROVE_STATE_DIR) { $env:GROVE_STATE_DIR } else { Join-Path $HOME ".grove" }
    function Get-StateManifest([string]$Path) {
        if (-not (Test-Path $Path)) {
            return ""
        }
        return ((Get-ChildItem -Path $Path -File -Recurse | Sort-Object FullName | ForEach-Object {
            $relative = $_.FullName.Substring($Path.Length)
            "$relative`t$((Get-FileHash -Path $_.FullName -Algorithm SHA256).Hash)"
        }) -join "`n")
    }
    $stateBefore = Get-StateManifest $stateDir
    $configPath = Join-Path $stateDir "config.toml"
    if (Test-Path $configPath) {
        & (Join-Path $tmp "$Binary.exe") config validate $configPath
        if ($LASTEXITCODE -ne 0) {
            throw "The installed release is incompatible with the existing Grove configuration."
        }
    }

    # Create install directory
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }

    if (-not (Test-Path (Join-Path $NodeDir "node.exe"))) {
        Write-Host "Installing private Node.js 22 runtime..."
        $sumsUrl = "https://nodejs.org/dist/latest-v22.x/SHASUMS256.txt"
        $sums = (Invoke-WebRequest -Uri $sumsUrl -UseBasicParsing).Content
        $match = [regex]::Match($sums, "(?m)^([a-f0-9]{64})  (node-v[^\s]+-win-x64\.zip)$")
        if (-not $match.Success) {
            throw "Unable to resolve the private Node.js runtime."
        }
        $nodeChecksum = $match.Groups[1].Value
        $nodeArchive = $match.Groups[2].Value
        $nodeZip = Join-Path $tmp $nodeArchive
        Invoke-WebRequest -Uri "https://nodejs.org/dist/latest-v22.x/$nodeArchive" -OutFile $nodeZip -UseBasicParsing
        $actualChecksum = (Get-FileHash -Path $nodeZip -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actualChecksum -ne $nodeChecksum) {
            throw "Node.js checksum verification failed."
        }
        $nodeExtract = Join-Path $tmp "node"
        Expand-Archive -Path $nodeZip -DestinationPath $nodeExtract -Force
        $nodeSource = Get-ChildItem -Path $nodeExtract -Directory | Select-Object -First 1
        if (-not $nodeSource) {
            throw "Node.js archive did not contain the expected directory."
        }
        if (Test-Path $NodeDir) {
            Remove-Item -Recurse -Force $NodeDir
        }
        New-Item -ItemType Directory -Path (Split-Path $NodeDir) -Force | Out-Null
        Move-Item -Path $nodeSource.FullName -Destination $NodeDir
    } else {
        Write-Host "Using Grove private Node.js: $NodeDir\node.exe"
    }

    $runtimePath = Join-Path $tmp "runtime\sandcastle"
    if (-not (Test-Path $runtimePath)) {
        throw "Release archive is missing the Grove Sandcastle runtime."
    }
    Write-Host "Installing private Grove Sandcastle runtime..."
    $runtimeNew = "$RuntimePrefix.new"
    if (Test-Path $runtimeNew) {
        Remove-Item -Recurse -Force $runtimeNew
    }
    & (Join-Path $NodeDir "npm.cmd") install --prefix $runtimeNew --omit=dev --install-links --no-audit --no-fund $runtimePath
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to install Grove Sandcastle runtime."
    }
    $newPackageRoot = Join-Path $runtimeNew "node_modules\@grove\sandcastle-runtime\dist"
    if (-not (Test-Path (Join-Path $newPackageRoot "sandcastle.js"))) {
        throw "Sandcastle runtime installation is incomplete."
    }
    $runtimeOld = "$RuntimePrefix.old"
    if (Test-Path $runtimeOld) {
        Remove-Item -Recurse -Force $runtimeOld
    }
    if (Test-Path $RuntimePrefix) {
        Move-Item -Path $RuntimePrefix -Destination $runtimeOld
    }
    try {
        Move-Item -Path $runtimeNew -Destination $RuntimePrefix
    }
    catch {
        if (Test-Path $runtimeOld) {
            Move-Item -Path $runtimeOld -Destination $RuntimePrefix
        }
        throw
    }
    if (Test-Path $runtimeOld) {
        Remove-Item -Recurse -Force $runtimeOld
    }
    $packageRoot = Join-Path $RuntimePrefix "node_modules\@grove\sandcastle-runtime\dist"

    if (-not (Get-Command herdr -ErrorAction SilentlyContinue)) {
        Write-Host "Installing private Herdr runtime..."
        $herdrZip = Join-Path $tmp "herdr-windows-x86_64.zip"
        Invoke-WebRequest -Uri "https://github.com/herdrdev/herdr/releases/latest/download/herdr-windows-x86_64.zip" -OutFile $herdrZip -UseBasicParsing
        $herdrExtract = Join-Path $tmp "herdr"
        Expand-Archive -Path $herdrZip -DestinationPath $herdrExtract -Force
        $herdrExe = Get-ChildItem -Path $herdrExtract -Filter "herdr.exe" -Recurse | Select-Object -First 1
        if (-not $herdrExe) {
            throw "Herdr release did not contain herdr.exe."
        }
        New-Item -ItemType Directory -Path $HerdrDir -Force | Out-Null
        Copy-Item -Path $herdrExe.FullName -Destination (Join-Path $HerdrDir "herdr.exe") -Force
        & (Join-Path $HerdrDir "herdr.exe") --version | Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw "Private Herdr installation failed validation."
        }
        Set-Content -Path (Join-Path $InstallDir "herdr.cmd") -Encoding ASCII -Value "@`"$HerdrDir\herdr.exe`" %*"
    } else {
        Write-Host "Using existing Herdr: $((Get-Command herdr).Source)"
    }

    $binaryPath = Join-Path $InstallDir "$Binary.exe"
    $binaryNew = "$binaryPath.new"
    Copy-Item -Path (Join-Path $tmp "$Binary.exe") -Destination $binaryNew -Force
    Move-Item -Path $binaryNew -Destination $binaryPath -Force

    $commands = @{
        "grove-sandcastle" = "sandcastle.js"
        "grove-lab" = "lab-session.js"
        "imp" = "orchestrator.js"
        "agent-flow" = "orchestrator.js"
        "review" = "pr-review.js"
        "resolve" = "conflict-resolver.js"
        "ci" = "ci-fix.js"
        "clean" = "cleanup.js"
        "address" = "address-review.js"
    }
    foreach ($command in $commands.GetEnumerator()) {
        $wrapper = "@`"$NodeDir\node.exe`" `"$packageRoot\$($command.Value)`" %*"
        Set-Content -Path (Join-Path $InstallDir "$($command.Key).cmd") -Encoding ASCII -Value $wrapper
    }

    $versionOutput = & $binaryPath --version
    if ($LASTEXITCODE -ne 0 -or $versionOutput -notmatch [regex]::Escape("grove version $version")) {
        throw "Installed Grove binary failed version validation."
    }
    foreach ($command in $commands.Keys) {
        if (-not (Test-Path (Join-Path $InstallDir "$command.cmd"))) {
            throw "Installed command is missing: $command"
        }
    }
    $stateAfter = Get-StateManifest $stateDir
    if ($stateBefore -ne $stateAfter) {
        throw "Grove user state changed during installation; refusing to report success."
    }

    # Add to user PATH if not already present
    if ($env:GROVE_SKIP_PATH_UPDATE -ne "1") {
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        if ($userPath -notlike "*$InstallDir*") {
            [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
            Write-Host "Added $InstallDir to user PATH."
            Write-Host "Restart your terminal for PATH changes to take effect."
        }
    }

    Write-Host ""
    Write-Host "[OK] grove v$version installed to $InstallDir\$Binary.exe"
    Write-Host "[OK] Sandcastle runtime installed to $RuntimePrefix"
    Write-Host "[OK] Herdr is available"
    Write-Host ""
    Write-Host "Run: grove"
    Write-Host "Docs: https://github.com/$Repo"
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
