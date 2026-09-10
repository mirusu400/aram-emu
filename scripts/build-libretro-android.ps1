param(
    [ValidateSet("arm64-v8a", "x86_64")]
    [string[]]$Abi = @("arm64-v8a"),
    [ValidateRange(21, 35)]
    [int]$Api = 21
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot

function Resolve-NdkRoot {
    foreach ($candidate in @($env:ANDROID_NDK_HOME, $env:ANDROID_NDK_ROOT)) {
        if ($candidate -and (Test-Path $candidate)) {
            return (Resolve-Path $candidate).Path
        }
    }
    $sdk = $env:ANDROID_HOME
    if (-not $sdk) {
        $sdk = $env:ANDROID_SDK_ROOT
    }
    if (-not $sdk) {
        $sdk = Join-Path $env:LOCALAPPDATA "Android\Sdk"
    }
    $ndkDirectory = Join-Path $sdk "ndk"
    $installed = Get-ChildItem -Path $ndkDirectory -Directory -ErrorAction SilentlyContinue |
        Sort-Object { [version]$_.Name } -Descending
    if (-not $installed) {
        throw "Android NDK not found. Set ANDROID_NDK_HOME or install an NDK below $ndkDirectory."
    }
    return $installed[0].FullName
}

$ndk = Resolve-NdkRoot
$toolchain = Join-Path $ndk "toolchains\llvm\prebuilt\windows-x86_64\bin"
$previous = @{
    GOOS = $env:GOOS
    GOARCH = $env:GOARCH
    CGO_ENABLED = $env:CGO_ENABLED
    CC = $env:CC
    CXX = $env:CXX
}

try {
    foreach ($target in $Abi) {
        switch ($target) {
            "arm64-v8a" {
                $goarch = "arm64"
                $triple = "aarch64-linux-android"
            }
            "x86_64" {
                $goarch = "amd64"
                $triple = "x86_64-linux-android"
            }
        }
        $cc = Join-Path $toolchain "$triple$Api-clang.cmd"
        $cxx = Join-Path $toolchain "$triple$Api-clang++.cmd"
        if (-not (Test-Path $cc) -or -not (Test-Path $cxx)) {
            throw "NDK compiler for $target API $Api was not found below $toolchain."
        }
        $outputDirectory = Join-Path $repo "build\libretro\android\$target"
        New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
        $output = Join-Path $outputDirectory "aram_libretro_android.so"

        $env:GOOS = "android"
        $env:GOARCH = $goarch
        $env:CGO_ENABLED = "1"
        $env:CC = $cc
        $env:CXX = $cxx
        Push-Location $repo
        try {
            & go build -trimpath -buildmode=c-shared "-ldflags=-s -w" -o $output .\cmd\aram-libretro
            if ($LASTEXITCODE -ne 0) {
                throw "go build failed for $target"
            }
        } finally {
            Pop-Location
        }
        Write-Host "$target -> $output"
    }
} finally {
    $env:GOOS = $previous.GOOS
    $env:GOARCH = $previous.GOARCH
    $env:CGO_ENABLED = $previous.CGO_ENABLED
    $env:CC = $previous.CC
    $env:CXX = $previous.CXX
}
