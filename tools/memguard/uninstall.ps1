$ErrorActionPreference = 'Continue'

$target = Join-Path $env:LOCALAPPDATA 'MemGuard'
$runKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'

Get-Process -Name MemGuard -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Remove-ItemProperty -Path $runKey -Name 'MemGuard' -ErrorAction SilentlyContinue
Write-Host "Autostart entry removed and process stopped"

if ($args -contains '-Purge') {
    if (Test-Path $target) { Remove-Item $target -Recurse -Force }
    Write-Host "Removed $target"
} else {
    Write-Host "Data directory kept: $target (use -Purge to delete)"
}
