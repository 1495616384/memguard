$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root

$assets = Join-Path $root 'assets'
if (-not (Test-Path $assets)) { New-Item -ItemType Directory -Path $assets | Out-Null }
$icoPath = Join-Path $assets 'icon.ico'

# Generate a 32x32 icon: dark background with a white M
Add-Type -AssemblyName System.Drawing
$bmp = [System.Drawing.Bitmap]::new(32, 32)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::AntiAlias
$g.Clear([System.Drawing.Color]::FromArgb(22, 26, 34))
$pen = [System.Drawing.Pen]::new([System.Drawing.Color]::FromArgb(64, 156, 255), [single]2)
$g.DrawRectangle($pen, 1, 1, 29, 29)
$font = [System.Drawing.Font]::new('Segoe UI', [single]16, [System.Drawing.FontStyle]::Bold, [System.Drawing.GraphicsUnit]::Pixel)
$fmt = [System.Drawing.StringFormat]::new()
$fmt.Alignment = [System.Drawing.StringAlignment]::Center
$fmt.LineAlignment = [System.Drawing.StringAlignment]::Center
$rect = [System.Drawing.RectangleF]::new(0, 0, 32, 32)
$g.DrawString('M', $font, [System.Drawing.Brushes]::White, $rect, $fmt)
$g.Dispose()

$hIcon = $bmp.GetHicon()
$icon = [System.Drawing.Icon]::FromHandle($hIcon)
$fs = [System.IO.File]::Create($icoPath)
$icon.Save($fs)
$fs.Close()
$icon.Dispose()
$bmp.Dispose()
Write-Host "Icon generated: $icoPath"

Write-Host "==> go vet"
go vet ./...
if ($LASTEXITCODE -ne 0) { throw "go vet failed" }

Write-Host "==> go build"
$dist = Join-Path $root 'dist'
if (-not (Test-Path $dist)) { New-Item -ItemType Directory -Path $dist | Out-Null }
go build -trimpath -ldflags "-H windowsgui -s -w" -o (Join-Path $dist 'MemGuard.exe') .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

$exe = Join-Path $dist 'MemGuard.exe'
$size = [math]::Round((Get-Item $exe).Length / 1KB, 1)
Write-Host "Build OK: $exe ($size KB)"
