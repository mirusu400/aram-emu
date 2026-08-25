param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$FirmwarePath,

    [string]$CorePath = (Join-Path $PSScriptRoot "..\..\aram-core-magichole-system"),

    [UInt64]$InstructionsPerFrame = 1000000,

    [ValidateSet("jit", "precise")]
    [string]$CPUBackend = "jit",

    [switch]$NoMediaPersistence
)

$ErrorActionPreference = "Stop"

$emuRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$projectRoot = (Resolve-Path (Join-Path $emuRoot "..")).Path
$frontendRoot = (Resolve-Path (Join-Path $projectRoot "aram-frontend")).Path
$authdRoot = (Resolve-Path (Join-Path $projectRoot "aram-authd")).Path
$systemCoreRoot = (Resolve-Path $CorePath).Path
$firmwareRoot = (Resolve-Path $FirmwarePath).Path

if (-not (Test-Path -LiteralPath $firmwareRoot -PathType Container)) {
    throw "FirmwarePath must point to an extracted firmware directory: $firmwareRoot"
}

$temporaryBase = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$workspaceRoot = Join-Path $temporaryBase ("aram-system-work-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $workspaceRoot | Out-Null

$oldGoWork = $env:GOWORK
try {
    Push-Location $workspaceRoot
    & go work init $emuRoot $frontendRoot $authdRoot
    if ($LASTEXITCODE -ne 0) {
        throw "go work init failed with exit code $LASTEXITCODE"
    }
    & go work edit ("-replace=github.com/mirusu400/aram-core=" + $systemCoreRoot)
    if ($LASTEXITCODE -ne 0) {
        throw "go work edit failed with exit code $LASTEXITCODE"
    }
    $env:GOWORK = Join-Path $workspaceRoot "go.work"
    Pop-Location

    Push-Location $emuRoot
    $arguments = @(
        "run",
        "-tags=system_firmware",
        "./cmd/aram-system",
        "-instructions-per-frame", $InstructionsPerFrame,
        "-cpu", $CPUBackend,
        $firmwareRoot
    )
    if ($NoMediaPersistence) {
        $arguments = @(
            "run",
            "-tags=system_firmware",
            "./cmd/aram-system",
            "-instructions-per-frame", $InstructionsPerFrame,
            "-cpu", $CPUBackend,
            "-no-media-persistence",
            $firmwareRoot
        )
    }
    & go @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "aram-system exited with code $LASTEXITCODE"
    }
} finally {
    while ((Get-Location).Path -eq $emuRoot -or (Get-Location).Path -eq $workspaceRoot) {
        Pop-Location
    }
    $env:GOWORK = $oldGoWork
    $resolvedWorkspace = [IO.Path]::GetFullPath($workspaceRoot)
    if ($resolvedWorkspace.StartsWith($temporaryBase, [StringComparison]::OrdinalIgnoreCase) -and
        $resolvedWorkspace -ne $temporaryBase -and
        (Test-Path -LiteralPath $resolvedWorkspace -PathType Container)) {
        Remove-Item -LiteralPath $resolvedWorkspace -Recurse -Force
    }
}
