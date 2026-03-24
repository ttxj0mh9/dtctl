---
layout: docs
title: Windows
---

Everything you need to get dtctl running on Windows.

## Quick Install

Open PowerShell and run:

```powershell
# Download the latest release
$arch = if ([Environment]::Is64BitOperatingSystem -and
  [Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$release = Invoke-RestMethod 'https://api.github.com/repos/dynatrace-oss/dtctl/releases/latest'
$asset = $release.assets | Where-Object { $_.name -match "windows_$arch\.zip$" }
Invoke-WebRequest $asset.browser_download_url -OutFile dtctl.zip

# Extract and add to PATH
Expand-Archive dtctl.zip -DestinationPath "$env:LOCALAPPDATA\dtctl\bin" -Force
$binPath = "$env:LOCALAPPDATA\dtctl\bin"
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$binPath*") {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$binPath", 'User')
}
$env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' +
  [Environment]::GetEnvironmentVariable('Path', 'User')

# Verify
dtctl version
```

## Manual Install

1. Download the zip for your architecture from the [releases page](https://github.com/dynatrace-oss/dtctl/releases/latest):

   | Architecture | File |
   |---|---|
   | 64-bit (x86_64) | `dtctl_VERSION_windows_amd64.zip` |
   | ARM64 | `dtctl_VERSION_windows_arm64.zip` |

2. Extract the zip (right-click > **Extract All**, or use `Expand-Archive` in PowerShell).

3. Add the extracted directory to your user PATH:

   **PowerShell:**

   ```powershell
   $binPath = "$env:LOCALAPPDATA\dtctl\bin"
   $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
   if ($userPath -notlike "*$binPath*") {
       [Environment]::SetEnvironmentVariable('Path', "$userPath;$binPath", 'User')
   }
   ```

   **Or via UI:** `Win + R` > `sysdm.cpl` > **Advanced** > **Environment Variables** > edit user `Path`.

4. Restart your terminal and verify:

   ```powershell
   dtctl version
   ```

## Building from Source

Requires **Go 1.24+** and **Git**.

```powershell
git clone https://github.com/dynatrace-oss/dtctl.git
cd dtctl
go build -o bin\dtctl.exe .
.\bin\dtctl.exe version
```

## Shell Completion (PowerShell)

Add to your PowerShell profile for persistent tab completion:

```powershell
# Create profile if it doesn't exist
if (!(Test-Path -Path $PROFILE)) { New-Item -ItemType File -Path $PROFILE -Force }

# Add completion
Add-Content $PROFILE 'dtctl completion powershell | Out-String | Invoke-Expression'
```

Restart PowerShell. You can now use `Tab` to complete commands and flags.

For **Git Bash**, use bash completions instead:

```bash
dtctl completion bash > ~/.dtctl-completion.bash
echo 'source ~/.dtctl-completion.bash' >> ~/.bashrc
```

## Configuration

dtctl stores files under `%LOCALAPPDATA%\dtctl`. Credentials are stored in **Windows Credential Manager**.

```powershell
# OAuth login (recommended)
dtctl auth login --context my-env --environment "https://abc12345.apps.dynatrace.com"

# Or token-based
dtctl config set-context my-env `
  --environment "https://abc12345.apps.dynatrace.com" `
  --token-ref my-token

dtctl config set-credentials my-token `
  --token "dt0s16.XXXXXXXX.YYYYYYYY"

# Verify
dtctl doctor
```

Use the backtick (`` ` ``) for line continuation in PowerShell, not backslash.

## PowerShell Tips

### Quoting DQL queries

PowerShell can mangle double quotes inside strings. Use **here-strings** to avoid issues:

```powershell
dtctl query -f - -o json @'
fetch logs
| filter status = "ERROR"
| limit 10
'@
```

Or save queries to `.dql` files:

```powershell
dtctl query -f query.dql
```

### JSON piping

```powershell
$workflows = dtctl get workflows -o json | ConvertFrom-Json
$workflows | Where-Object { $_.title -like "*daily*" }
```

### Environment variables

```powershell
# Current session
$env:DTCTL_ENVIRONMENT = "https://abc12345.apps.dynatrace.com"

# Persistent
[Environment]::SetEnvironmentVariable('DTCTL_ENVIRONMENT', 'https://abc12345.apps.dynatrace.com', 'User')
```

## Other Shells

| Shell | Notes |
|---|---|
| **Windows Terminal** | Full support in all profiles (PowerShell, cmd, Git Bash, WSL) |
| **cmd.exe** | Works, but no tab completion |
| **WSL** | Install the Linux binary inside WSL -- see [Installation]({{ '/docs/installation/' | relative_url }}) |

## Updating

Re-download the latest zip and extract to the same location:

```powershell
Expand-Archive dtctl.zip -DestinationPath "$env:LOCALAPPDATA\dtctl\bin" -Force
```

## Uninstalling

```powershell
# Remove binary
Remove-Item -Recurse -Force "$env:LOCALAPPDATA\dtctl\bin"

# Remove from PATH
$binPath = "$env:LOCALAPPDATA\dtctl\bin"
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$newPath = ($userPath -split ';' | Where-Object { $_ -ne $binPath }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $newPath, 'User')

# Remove config (optional)
Remove-Item -Recurse -Force "$env:LOCALAPPDATA\dtctl"
```

## Troubleshooting

### "dtctl is not recognized"

The binary is not in your PATH. Check with:

```powershell
$env:Path -split ';' | Select-String dtctl
```

Restart your terminal after modifying PATH.

### Antivirus blocks dtctl.exe

Verify the checksum against the official `checksums.txt` from the release, then add an exclusion for `dtctl.exe` in your antivirus settings.

### PowerShell profile won't load

Your execution policy may block profile scripts:

```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

---

Next: [Quick Start]({{ '/docs/quick-start/' | relative_url }}) -- configure your environment and run your first commands.
