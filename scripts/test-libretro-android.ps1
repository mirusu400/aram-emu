param(
    [string]$Serial = "",
    [ValidateSet("armeabi-v7a", "arm64-v8a", "x86_64")]
    [string]$Abi = "x86_64",
    [ValidateRange(21, 35)]
    [int]$Api = 21
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$sdk = if ($env:ANDROID_HOME) { $env:ANDROID_HOME } elseif ($env:ANDROID_SDK_ROOT) { $env:ANDROID_SDK_ROOT } else { Join-Path $env:LOCALAPPDATA "Android\Sdk" }
$adb = Join-Path $sdk "platform-tools\adb.exe"
if (-not (Test-Path $adb)) {
    throw "adb was not found at $adb"
}
$adbArgs = @()
if ($Serial) {
    $adbArgs += @("-s", $Serial)
}

function Invoke-SmokeAdb {
    param([string[]]$Arguments)
    # Windows PowerShell treats successful adb progress on stderr as an error
    # when callers redirect the smoke log. adb's exit code is authoritative.
    $ErrorActionPreference = "Continue"
    & $adb @adbArgs @Arguments 2>&1 | ForEach-Object { "$_" } | Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw "adb $($Arguments[0]) failed with exit code $LASTEXITCODE"
    }
}

& (Join-Path $PSScriptRoot "build-libretro-android.ps1") -Abi $Abi -Api $Api
if ($LASTEXITCODE -ne 0) {
    throw "$Abi core build failed"
}

$ndkRoot = if ($env:ANDROID_NDK_HOME) { $env:ANDROID_NDK_HOME } else {
    $ndkDirectory = Join-Path $sdk "ndk"
    (Get-ChildItem -Path $ndkDirectory -Directory | Sort-Object { [version]$_.Name } -Descending | Select-Object -First 1).FullName
}
$triple = switch ($Abi) {
    "armeabi-v7a" { "armv7a-linux-androideabi" }
    "arm64-v8a" { "aarch64-linux-android" }
    "x86_64" { "x86_64-linux-android" }
}
$compiler = Join-Path $ndkRoot "toolchains\llvm\prebuilt\windows-x86_64\bin\$triple$Api-clang.cmd"
$smokeHost = Join-Path $repo "build\libretro\android\$Abi\aram-libretro-smoke"
& $compiler -std=c11 -Wall -Wextra -Werror -O2 -o $smokeHost (Join-Path $repo "cmd\aram-libretro\testhost\smoke.c") -ldl
if ($LASTEXITCODE -ne 0) {
    throw "Android libretro smoke host build failed"
}

$core = Join-Path $repo "build\libretro\android\$Abi\aram_libretro_android.so"
foreach ($push in @(
    @{ Source = $core; Target = "/data/local/tmp/aram_libretro_android.so" },
    @{ Source = $smokeHost; Target = "/data/local/tmp/aram-libretro-smoke" }
)) {
    Invoke-SmokeAdb -Arguments @("push", $push.Source, $push.Target)
}
Invoke-SmokeAdb -Arguments @("shell", "chmod", "755", "/data/local/tmp/aram-libretro-smoke")
Invoke-SmokeAdb -Arguments @("shell", "mkdir -p /data/local/tmp/aram-libretro-save && /data/local/tmp/aram-libretro-smoke /data/local/tmp/aram_libretro_android.so")
