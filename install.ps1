# KernelView Go - Windows Installation & Update Script
# Supported Architectures: x64 (amd64), ARM64 (arm64), x86 (386)
# PowerShell: Windows PowerShell 5.1+ and PowerShell 7+ (pwsh)
#
# Quick Run:
#   irm https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.ps1 | iex
#
# Non-interactive / Force Update:
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.ps1))) -Yes
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.ps1))) -Force

[CmdletBinding()]
param(
    [Alias("f")]
    [switch]$Force,

    [Alias("y")]
    [switch]$Yes,

    [Alias("b")]
    [switch]$Beta,

    [string]$Channel = "",

    [Alias("d")]
    [string]$InstallDir = "",

    [Alias("v")]
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"

# Enable TLS 1.2 for secure GitHub downloads on legacy PowerShell 5.1
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
} catch {
    # Ignore if not configurable
}

$repo = "codedbysoumyajit/KernelView-Go"
$fallbackTag = "v1.4.0"
$exeName = "kernelview.exe"

# Architecture detection
$arch = "amd64"
$rawArch = $env:PROCESSOR_ARCHITECTURE
if ($rawArch -eq "ARM64") {
    $arch = "arm64"
} elseif ($rawArch -eq "x86") {
    $arch = "386"
}

Write-Host "==> Detected architecture: Windows ($arch)" -ForegroundColor Cyan

# Determine if running in an interactive console
$isInteractive = $true
try {
    if (-not [Environment]::UserInteractive -or [Console]::IsInputRedirected) {
        $isInteractive = $false
    }
} catch {
    $isInteractive = $true
}

# Determine Target Release Tag (detect stable vs pre-release / beta / nightly)
$tag = $Version
if ([string]::IsNullOrWhiteSpace($tag)) {
    Write-Host "==> Checking for KernelView releases on GitHub..." -ForegroundColor Cyan
    $stableTag = ""
    $prereleaseTag = ""

    try {
        $releases = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases?per_page=15" -UseBasicParsing
        foreach ($rel in $releases) {
            if ($rel.prerelease -and [string]::IsNullOrWhiteSpace($prereleaseTag)) {
                $prereleaseTag = $rel.tag_name
            } elseif (-not $rel.prerelease -and -not $rel.draft -and [string]::IsNullOrWhiteSpace($stableTag)) {
                $stableTag = $rel.tag_name
            }
            if ($stableTag -and $prereleaseTag) { break }
        }
    } catch {
        # Fallback to single latest endpoint if available
        try {
            $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -UseBasicParsing
            $stableTag = $latest.tag_name
        } catch {
            $stableTag = $fallbackTag
        }
    }

    if ([string]::IsNullOrWhiteSpace($stableTag)) {
        $stableTag = $fallbackTag
    }

    # If a pre-release is detected, ask user for Stable vs Beta (Nightly)
    if (-not [string]::IsNullOrWhiteSpace($prereleaseTag)) {
        if ($Beta -or $Channel -eq "beta" -or $Channel -eq "nightly") {
            $tag = $prereleaseTag
            Write-Host "==> Pre-release detected. Selected Beta/Nightly channel: $tag" -ForegroundColor Cyan
        } elseif ($Channel -eq "stable") {
            $tag = $stableTag
            Write-Host "==> Selected Stable channel: $tag" -ForegroundColor Cyan
        } elseif ($isInteractive -and -not $Yes) {
            Write-Host ""
            Write-Host "==> Pre-release detected on GitHub!" -ForegroundColor Cyan
            Write-Host "  [1] Stable:       $stableTag (Recommended)" -ForegroundColor White
            Write-Host "  [2] Beta/Nightly: $prereleaseTag" -ForegroundColor White
            Write-Host ""
            $choice = Read-Host "Which version would you like to install? [1=Stable, 2=Beta] (Default: 1)"
            if ($choice -match '^(2|b|beta|nightly)$') {
                $tag = $prereleaseTag
                Write-Host "==> Selected Beta/Nightly release channel ($tag)." -ForegroundColor Cyan
            } else {
                $tag = $stableTag
                Write-Host "==> Selected Stable release channel ($tag)." -ForegroundColor Cyan
            }
            Write-Host ""
        } else {
            $tag = $stableTag
        }
    } else {
        $tag = $stableTag
    }
}

if (-not $tag.StartsWith("v")) {
    $tag = "v$tag"
}
$tagClean = $tag.TrimStart('v')

# Check for existing installation (compatible with PS 5.1 and PS 7+)
$existingBin = $null
if (-not [string]::IsNullOrWhiteSpace($InstallDir) -and (Test-Path (Join-Path $InstallDir $exeName))) {
    $existingBin = Join-Path $InstallDir $exeName
} else {
    $existingCmd = Get-Command "kernelview" -ErrorAction SilentlyContinue
    if ($existingCmd) {
        if ($existingCmd.Path) {
            $existingBin = $existingCmd.Path
        } elseif ($existingCmd.Source) {
            $existingBin = $existingCmd.Source
        } elseif ($existingCmd.Definition) {
            $existingBin = $existingCmd.Definition
        }
    }
}

if (-not $existingBin -or -not (Test-Path $existingBin)) {
    $candidatePaths = @(
        "$env:LOCALAPPDATA\Programs\KernelView\$exeName",
        "$env:USERPROFILE\AppData\Local\Microsoft\WindowsApps\$exeName",
        "$env:ProgramFiles\KernelView\$exeName"
    )
    foreach ($cand in $candidatePaths) {
        if (Test-Path $cand) {
            $existingBin = $cand
            break
        }
    }
}

$installedVer = ""
if ($existingBin -and (Test-Path $existingBin)) {
    try {
        $rawVer = & $existingBin -v 2>&1 | Out-String
        if ($rawVer -match 'v?\d+\.\d+(\.\d+)?') {
            $installedVer = $Matches[0]
            if (-not $installedVer.StartsWith("v")) {
                $installedVer = "v$installedVer"
            }
        }
    } catch {
        # Ignore version parse errors
    }
}

# Prompt user if already installed
if ($existingBin -and (Test-Path $existingBin)) {
    Write-Host ""
    Write-Host "==> Found existing KernelView installation at: $existingBin" -ForegroundColor Cyan
    if ($installedVer) {
        Write-Host "==> Currently installed version: $installedVer" -ForegroundColor Cyan
    }
    Write-Host "==> Latest available version:    $tag" -ForegroundColor Cyan
    Write-Host ""

    if ($installedVer -eq $tag -and -not $Force) {
        Write-Host "==> KernelView is already up to date ($tag)!" -ForegroundColor Green
        if ($isInteractive -and -not $Yes) {
            $reply = Read-Host "Would you like to reinstall / overwrite anyway? [y/N]"
            if ($reply -notmatch '^(y|yes)$') {
                Write-Host "==> No changes made. You're all set with KernelView $installedVer!" -ForegroundColor Green
                return
            }
            Write-Host "==> Proceeding with reinstall..." -ForegroundColor Cyan
        } else {
            Write-Host "==> Use -Force to reinstall. Exiting." -ForegroundColor Green
            return
        }
    } elseif (-not $Force) {
        if ($isInteractive -and -not $Yes) {
            $reply = Read-Host "Would you like to update KernelView to latest ($tag)? [Y/n]"
            if ($reply -match '^(n|no)$') {
                Write-Host "==> Update cancelled by user. Existing version remains unchanged." -ForegroundColor Yellow
                return
            }
        }
        Write-Host "==> Updating KernelView to $tag..." -ForegroundColor Cyan
    }
}

# Determine target install directory
$targetDir = $InstallDir
if ([string]::IsNullOrWhiteSpace($targetDir)) {
    if ($existingBin -and (Test-Path $existingBin)) {
        # Update in-place in existing directory
        $targetDir = Split-Path -Parent $existingBin
    } else {
        # Modern standard user-local directory (no admin rights required)
        $targetDir = "$env:LOCALAPPDATA\Programs\KernelView"
    }
}

# Verify directory permissions; fallback to user-local if elevation required
if (Test-Path $targetDir) {
    $testFile = Join-Path $targetDir (".kv_write_test_" + [Guid]::NewGuid().ToString().Substring(0, 8))
    $canWrite = $false
    try {
        [IO.File]::WriteAllText($testFile, "test")
        Remove-Item -Path $testFile -Force -ErrorAction SilentlyContinue
        $canWrite = $true
    } catch {
        $canWrite = $false
    }

    if (-not $canWrite) {
        Write-Warning "Directory $targetDir is not writable without Administrator privileges."
        Write-Host "Falling back to user directory: $env:LOCALAPPDATA\Programs\KernelView" -ForegroundColor Yellow
        $targetDir = "$env:LOCALAPPDATA\Programs\KernelView"
    }
}

if (-not (Test-Path $targetDir)) {
    New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
}

# Stop any running kernelview processes before overwriting
$running = Get-Process -Name "kernelview" -ErrorAction SilentlyContinue
if ($running) {
    Write-Host "==> Stopping running KernelView process..." -ForegroundColor Yellow
    $running | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 500
}

# Download release archive
$archiveName = "kernelview_${tagClean}_windows_${arch}.zip"
$downloadUrl = "https://github.com/$repo/releases/download/$tag/$archiveName"
$tempDir = Join-Path $env:TEMP ("kernelview_install_" + [Guid]::NewGuid().ToString().Substring(0, 8))
$zipFile = Join-Path $tempDir $archiveName
$extractDir = Join-Path $tempDir "extracted"

New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

try {
    Write-Host "==> Downloading KernelView $tag for Windows ($arch)..." -ForegroundColor Cyan
    Invoke-WebRequest -Uri $downloadUrl -OutFile $zipFile -UseBasicParsing

    Write-Host "==> Extracting files..." -ForegroundColor Cyan
    Expand-Archive -Path $zipFile -DestinationPath $extractDir -Force

    $foundExe = Get-ChildItem -Path $extractDir -Filter $exeName -Recurse | Select-Object -First 1
    if (-not $foundExe) {
        throw "Could not find $exeName in the downloaded archive."
    }

    $destPath = Join-Path $targetDir $exeName
    Write-Host "==> Installing to $destPath..." -ForegroundColor Cyan
    Copy-Item -Path $foundExe.FullName -Destination $destPath -Force

    # Configure User PATH if needed
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$targetDir*") {
        Write-Host "==> Adding $targetDir to User PATH environment variable..." -ForegroundColor Cyan
        $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $targetDir } else { "$userPath;$targetDir" }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        $env:PATH = "$env:PATH;$targetDir"
    }

    # Verify installation
    if (Test-Path $destPath) {
        Write-Host ""
        $verCheck = & $destPath -v 2>&1 | Out-String
        Write-Host "==> SUCCESS: KernelView installed successfully!" -ForegroundColor Green
        if ($verCheck) {
            Write-Host "==> $verCheck" -ForegroundColor Green
        }
        Write-Host ""
        Write-Host "Quick start: Run 'kernelview' in your terminal." -ForegroundColor White
        Write-Host "Live mode:   Run 'kernelview -l' for real-time monitoring." -ForegroundColor White
    } else {
        throw "Installation failed: $destPath does not exist after copying."
    }
} catch {
    Write-Error "KernelView installation failed: $_"
} finally {
    # Cleanup temp directory
    if (Test-Path $tempDir) {
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
