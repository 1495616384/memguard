$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$src = Join-Path $root 'dist\MemGuard.exe'
if (-not (Test-Path $src)) { throw "Missing $src, run build.ps1 first" }

$target = Join-Path $env:LOCALAPPDATA 'MemGuard'
if (-not (Test-Path $target)) { New-Item -ItemType Directory -Path $target | Out-Null }

$dst = Join-Path $target 'MemGuard.exe'
Get-Process -Name MemGuard -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Copy-Item $src $dst -Force

$runKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
New-ItemProperty -Path $runKey -Name 'MemGuard' -Value "`"$dst`"" -PropertyType String -Force | Out-Null

# Start detached via WMI so the process does not die with the calling shell
$res = ([wmiclass]'Win32_Process').Create("`"$dst`"")
Write-Host "Start result: ReturnValue=$($res.ReturnValue) PID=$($res.ProcessId)"
Write-Host "Installed: $dst"
Write-Host "Autostart registered: HKCU\Software\Microsoft\Windows\CurrentVersion\Run\MemGuard"
Write-Host "Add $dst to Huorong trust list to avoid interception."
