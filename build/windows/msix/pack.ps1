<#
.SYNOPSIS
Stages an MSIX layout and packs it with makeappx.

The package is left unsigned: the Microsoft Store signs it on ingestion. Pass
-SignThumbprint only to produce a locally installable package for testing.
#>
param(
    [Parameter(Mandatory = $true)][ValidateSet('x64', 'arm64', 'x86')][string]$Arch,
    [Parameter(Mandatory = $true)][string]$Version,
    [Parameter(Mandatory = $true)][string]$Executable,
    [Parameter(Mandatory = $true)][string]$Out,
    [string]$SignThumbprint
)

$ErrorActionPreference = 'Stop'

if ($Version -notmatch '^\d+\.\d+\.\d+\.0$') {
    throw "Store packages require a four-part version ending in 0, got '$Version'"
}

function Find-SdkTool([string]$Name) {
    $roots = @("${env:ProgramFiles(x86)}\Windows Kits\10\bin", "${env:ProgramFiles}\Windows Kits\10\bin")
    $found = $roots | Where-Object { Test-Path $_ } | ForEach-Object {
        Get-ChildItem -Path $_ -Filter $Name -Recurse -ErrorAction SilentlyContinue
    } | Where-Object { $_.FullName -match '\\x64\\' } | Sort-Object FullName -Descending
    if (-not $found) { throw "$Name not found. Install the Windows 10/11 SDK." }
    return $found[0].FullName
}

$msixDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$layout = Join-Path ([System.IO.Path]::GetTempPath()) "msix-layout-$Arch"

if (Test-Path $layout) { Remove-Item -Recurse -Force $layout }
New-Item -ItemType Directory -Path $layout | Out-Null

Copy-Item -Path (Join-Path $msixDir 'assets') -Destination (Join-Path $layout 'Assets') -Recurse
Copy-Item -Path $Executable -Destination (Join-Path $layout 'reManager.exe')

(Get-Content (Join-Path $msixDir 'app_manifest.xml') -Raw).
    Replace('__VERSION__', $Version).
    Replace('__ARCH__', $Arch) |
    Set-Content -Path (Join-Path $layout 'AppxManifest.xml') -Encoding UTF8

$outDir = Split-Path -Parent $Out
if ($outDir -and -not (Test-Path $outDir)) { New-Item -ItemType Directory -Path $outDir | Out-Null }

& (Find-SdkTool 'makeappx.exe') pack /d $layout /p $Out /o
if ($LASTEXITCODE -ne 0) { throw "makeappx failed with exit code $LASTEXITCODE" }

if ($SignThumbprint) {
    & (Find-SdkTool 'signtool.exe') sign /fd SHA256 /sha1 $SignThumbprint $Out
    if ($LASTEXITCODE -ne 0) { throw "signtool failed with exit code $LASTEXITCODE" }
}

Write-Host "Packed $Out"
