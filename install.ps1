<#
.SYNOPSIS
    Install dtctl (Dynatrace CLI) on Windows.
.DESCRIPTION
    Downloads the latest dtctl release from GitHub and installs it to
    %LOCALAPPDATA%\dtctl. Adds the directory to the user PATH if needed.
.EXAMPLE
    irm https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.ps1 | iex
#>

$ErrorActionPreference = 'Stop'

# Determine architecture
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') {
    'arm64'
} else {
    'amd64'
}

$repoOwner = 'dynatrace-oss'
$repoName = 'dtctl'

# Get latest release
Write-Host "Fetching latest release..." -ForegroundColor Cyan
$release = Invoke-RestMethod "https://api.github.com/repos/$repoOwner/$repoName/releases/latest"
$tag = $release.tag_name
Write-Host "Latest release: $tag" -ForegroundColor Green

# Find download URL
$assetName = "dtctl_$($tag.TrimStart('v'))_windows_$arch.zip"
$asset = $release.assets | Where-Object { $_.name -eq $assetName }
if (-not $asset) {
    # Fallback: match by pattern
    $asset = $release.assets | Where-Object { $_.name -match "windows_$arch\.zip$" }
}
if (-not $asset) {
    Write-Error "Could not find Windows $arch asset in release $tag. Available assets: $($release.assets.name -join ', ')"
    exit 1
}

$downloadUrl = $asset.browser_download_url

# Install directory
$installDir = Join-Path $env:LOCALAPPDATA 'dtctl'
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

# Download and extract
$tempZip = Join-Path $env:TEMP "dtctl-$tag.zip"
Write-Host "Downloading $($asset.name)..." -ForegroundColor Cyan
Invoke-WebRequest -Uri $downloadUrl -OutFile $tempZip -UseBasicParsing
Write-Host "Extracting to $installDir..." -ForegroundColor Cyan
Expand-Archive -Path $tempZip -DestinationPath $installDir -Force
Remove-Item $tempZip -Force

# Verify the binary exists
$exePath = Join-Path $installDir 'dtctl.exe'
if (-not (Test-Path $exePath)) {
    Write-Error "dtctl.exe not found in $installDir after extraction."
    exit 1
}

# Add to PATH if not already present
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$installDir", 'User')
    $env:Path = "$env:Path;$installDir"
    Write-Host "Added $installDir to user PATH." -ForegroundColor Green
} else {
    Write-Host "$installDir is already in PATH." -ForegroundColor Green
}

# Verify
Write-Host ""
$version = & $exePath version 2>&1
Write-Host "Installed: $version" -ForegroundColor Green
Write-Host ""
Write-Host "Installation complete! Run 'dtctl --help' to get started." -ForegroundColor Green
Write-Host ""
Write-Host "Quick setup:" -ForegroundColor Cyan
Write-Host '  dtctl auth login --context my-env --environment "https://YOUR_ENV.apps.dynatrace.com"' -ForegroundColor White
