# Runs the whole-phone development entry point against an extracted firmware
# directory.
#
# The shipping product (cmd/aram) opens firmware on its own; this wrapper exists
# for the development flags that only matter while working on the system
# machine. It no longer builds a Go workspace overlay: the system-machine API
# lives in aram-core's default branch, so the ordinary workspace resolves it.

param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$FirmwarePath,

    [UInt64]$InstructionsPerFrame = 1000000,

    [ValidateSet("jit", "precise")]
    [string]$CPUBackend = "jit",

    [switch]$NoMediaPersistence
)

$ErrorActionPreference = "Stop"

$emuRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$firmwareRoot = (Resolve-Path $FirmwarePath).Path

if (-not (Test-Path -LiteralPath $firmwareRoot -PathType Container)) {
    throw "FirmwarePath must point to an extracted firmware directory: $firmwareRoot"
}

$arguments = @(
    "run",
    "./cmd/aram-system",
    "-instructions-per-frame", $InstructionsPerFrame,
    "-cpu", $CPUBackend
)
if ($NoMediaPersistence) {
    $arguments += "-no-media-persistence"
}
$arguments += $firmwareRoot

Push-Location $emuRoot
try {
    & go @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "aram-system exited with code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}
