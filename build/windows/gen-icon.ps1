Add-Type -AssemblyName System.Drawing

$size = 32
$out  = Join-Path $PSScriptRoot "icon.ico"

$bmp = New-Object System.Drawing.Bitmap $size, $size
$g   = [System.Drawing.Graphics]::FromImage($bmp)
$g.SmoothingMode    = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit

# Dark navy background.
$bg = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(255, 20, 23, 32))
$g.FillRectangle($bg, 0, 0, $size, $size)

# Bright accent top bar — evokes "two paths".
$accent = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(255, 95, 184, 255))
$g.FillRectangle($accent, 0, 0, $size, [int]($size * 0.16))

# Double-arrow (U+21C4 ⇄) centered, white, bold.
$font  = New-Object System.Drawing.Font "Segoe UI Symbol", ([float]$size * 0.62), ([System.Drawing.FontStyle]::Bold)
$white = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::White)
$sf    = New-Object System.Drawing.StringFormat
$sf.Alignment     = [System.Drawing.StringAlignment]::Center
$sf.LineAlignment = [System.Drawing.StringAlignment]::Center
$rect  = New-Object System.Drawing.RectangleF 0, ([float]$size * 0.06), $size, $size
$g.DrawString([string][char]0x21C4, $font, $white, $rect, $sf)

$hicon = $bmp.GetHicon()
$icon  = [System.Drawing.Icon]::FromHandle($hicon)
$fs    = [System.IO.File]::OpenWrite($out)
$icon.Save($fs)
$fs.Close()

Write-Host "wrote $out ($size x $size)"
