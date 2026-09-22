#Requires -Version 5.1
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$Repo     = "m00nk0d3/grove"
$Binary   = "grove"
$InstallDir = Join-Path $env:LOCALAPPDATA "grove"
$NodeDir = Join-Path $InstallDir "deps\node"
$HerdrDir = Join-Path $InstallDir "deps\herdr"
$RuntimePrefix = Join-Path $InstallDir "sandcastle"

Write-Host "Fetching latest Grove release..."

# Fetch latest release version
$apiUrl  = "https://api.github.com/repos/$Repo/releases/latest"
$release = Invoke-RestMethod -Uri $apiUrl -UseBasicParsing
$tag     = $release.tag_name          # e.g. "v0.5.0"
$version = $tag.TrimStart("v")

# Only amd64 Windows builds are produced by GoReleaser
$archive = "${Binary}_${version}_windows_amd64.zip"
$url     = "https://github.com/$Repo/releases/download/$tag/$archive"

Write-Host "Installing grove v$version for windows/amd64..."

$tmp     = Join-Path $env:TEMP "grove-install-$([System.IO.Path]::GetRandomFileName())"
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    $zipPath = Join-Path $tmp $archive
    Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing

    Expand-Archive -Path $zipPath -DestinationPath $tmp -Force

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
    # npm treats a package whose version has not changed as already satisfied,
    # so an upgrade that keeps the runtime version leaves the previously
    # installed files in place. Remove the installed package first, so every run
    # delivers the runtime it is installing rather than reporting success over a
    # stale copy.
    $installedRuntime = Join-Path $RuntimePrefix "node_modules\@grove\sandcastle-runtime"
    if (Test-Path $installedRuntime) {
        Remove-Item -Recurse -Force $installedRuntime
    }
    & (Join-Path $NodeDir "npm.cmd") install --prefix $RuntimePrefix --omit=dev --install-links --no-audit --no-fund $runtimePath
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to install Grove Sandcastle runtime."
    }
    $packageRoot = Join-Path $RuntimePrefix "node_modules\@grove\sandcastle-runtime\dist"
    if (-not (Test-Path (Join-Path $packageRoot "sandcastle.js"))) {
        throw "Sandcastle runtime installation is incomplete."
    }

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

    Copy-Item -Path (Join-Path $tmp "$Binary.exe") -Destination (Join-Path $InstallDir "$Binary.exe") -Force

    $commands = @{
        "grove-sandcastle" = "sandcastle.js"
        "imp" = "orchestrator.js"
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

    # Add to user PATH if not already present
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        Write-Host "Added $InstallDir to user PATH."
        Write-Host "Restart your terminal for PATH changes to take effect."
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
