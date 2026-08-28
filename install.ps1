# PowerShell script to install KernelView Go on Windows without Administrator privileges
$ErrorActionPreference = "Stop"

$repo = "codedbysoumyajit/KernelView-Go"
$releases = "https://api.github.com/repos/$repo/releases/latest"

Write-Host "Fetching latest release version..."
try {
    $tag = (Invoke-RestMethod -Uri $releases).tag_name
} catch {
    $tag = "v1.3.2" # Fallback
}
$tagClean = $tag.TrimStart('v')

# Detect architecture
$arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
    $arch = "arm64"
}

$url = "https://github.com/$repo/releases/download/$tag/kernelview_${tagClean}_windows_${arch}.zip"
$zipFile = "$env:TEMP\kernelview.zip"
$destDir = "$env:TEMP\kernelview_extracted"

Write-Host "Downloading KernelView Go $tag for Windows ($arch)..."
Invoke-WebRequest -Uri $url -OutFile $zipFile

Write-Host "Extracting files..."
if (Test-Path $destDir) {
    Remove-Item -Path $destDir -Recurse -Force
}
Expand-Archive -Path $zipFile -DestinationPath $destDir -Force

# Locate executable
$exe = Get-ChildItem -Path $destDir -Filter "kernelview.exe" -Recurse | Select-Object -First 1

if ($exe) {
    $installDir = "$env:USERPROFILE\AppData\Local\Microsoft\WindowsApps"
    if (!(Test-Path $installDir)) {
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    }
    Write-Host "Installing to $installDir..."
    Copy-Item -Path $exe.FullName -Destination "$installDir\kernelview.exe" -Force

    # Verify if PATH contains $installDir, and add it if missing
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $fullPath = $userPath + ";" + $machinePath
    if ($fullPath -notlike "*$installDir*") {
        Write-Host "Adding $installDir to User PATH environment variable..."
        [Environment]::SetEnvironmentVariable("Path", $userPath + ";" + $installDir, "User")
        $env:PATH += ";$installDir"
    }

    Write-Host "KernelView Go installed successfully! Run 'kernelview' in any shell window."
} else {
    Write-Error "Could not find kernelview.exe in the downloaded archive."
}

# Cleanup
if (Test-Path $zipFile) {
    Remove-Item -Path $zipFile -Force
}
if (Test-Path $destDir) {
    Remove-Item -Path $destDir -Recurse -Force
}
