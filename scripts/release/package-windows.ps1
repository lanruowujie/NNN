param(
    [Parameter(Mandatory = $true)][string]$Version,
    [switch]$Unsigned
)

$ErrorActionPreference = "Stop"
$Repository = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$Source = Join-Path $Repository "build/bin/NFCX-windows-amd64"
$Dist = Join-Path $Repository "dist"
$Suffix = if ($Unsigned) { "-unsigned" } else { "" }
$Destination = Join-Path $Dist "NFCX-$Version-windows-amd64$Suffix.zip"
if (-not (Test-Path (Join-Path $Source "NFCX.exe"))) {
    throw "Packaged Windows application is missing: $Source"
}
go build -o (Join-Path $Source "NFCX Updater.exe") ./cmd/nfcx-updater
New-Item -ItemType Directory -Force -Path $Dist | Out-Null
if (Test-Path $Destination) { Remove-Item -Force $Destination }
Compress-Archive -Path (Join-Path $Source "*") -DestinationPath $Destination -CompressionLevel Optimal
Write-Output "Created $Destination"
