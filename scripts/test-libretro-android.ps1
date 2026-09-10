param(
    [string]$Serial = "",
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

& (Join-Path $PSScriptRoot "build-libretro-android.ps1") -Abi x86_64 -Api $Api
if ($LASTEXITCODE -ne 0) {
    throw "x86_64 core build failed"
}

$ndkRoot = if ($env:ANDROID_NDK_HOME) { $env:ANDROID_NDK_HOME } else {
    $ndkDirectory = Join-Path $sdk "ndk"
    (Get-ChildItem -Path $ndkDirectory -Directory | Sort-Object { [version]$_.Name } -Descending | Select-Object -First 1).FullName
}
$compiler = Join-Path $ndkRoot "toolchains\llvm\prebuilt\windows-x86_64\bin\x86_64-linux-android$Api-clang.cmd"
$smokeHost = Join-Path $repo "build\libretro\android\x86_64\aram-libretro-smoke"
& $compiler -std=c11 -Wall -Wextra -Werror -O2 -o $smokeHost (Join-Path $repo "cmd\aram-libretro\testhost\smoke.c") -ldl
if ($LASTEXITCODE -ne 0) {
    throw "Android libretro smoke host build failed"
}

$core = Join-Path $repo "build\libretro\android\x86_64\aram_libretro_android.so"
& $adb @adbArgs push $core /data/local/tmp/aram_libretro_android.so | Out-Host
& $adb @adbArgs push $smokeHost /data/local/tmp/aram-libretro-smoke | Out-Host
& $adb @adbArgs shell chmod 755 /data/local/tmp/aram-libretro-smoke
& $adb @adbArgs shell "mkdir -p /data/local/tmp/aram-libretro-save && /data/local/tmp/aram-libretro-smoke /data/local/tmp/aram_libretro_android.so"
if ($LASTEXITCODE -ne 0) {
    throw "Android libretro smoke test failed"
}
