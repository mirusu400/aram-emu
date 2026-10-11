# Builds the ARAM web/WebAssembly product into .\web.
#
#   pwsh .\scripts\build-web.ps1            # build web\aram.wasm + copy wasm_exec.js
#   pwsh .\scripts\build-web.ps1 -Serve     # also serve .\web on http://localhost:8080
#
# Output (web\aram.wasm, web\wasm_exec.js, web\index.html) is a static bundle:
# copy it into aram-website, or serve the folder with any static file server.
param(
    [switch]$Serve,
    [int]$Port = 8080
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$webDir = Join-Path $repoRoot "web"
New-Item -ItemType Directory -Force -Path $webDir | Out-Null

# Keep the compiler and JS runtime on the same patched toolchain as web CI.
# Go 1.26.0 can crash during WASM GC (golang/go#80472; fixed in 1.26.6).
function Invoke-WebGo {
    $previousToolchain = $env:GOTOOLCHAIN
    try {
        $env:GOTOOLCHAIN = "go1.26.9"
        & go @args
        if ($LASTEXITCODE -ne 0) { throw "web Go command failed ($LASTEXITCODE)" }
    } finally {
        $env:GOTOOLCHAIN = $previousToolchain
    }
}

$goRoot = (Invoke-WebGo env GOROOT).Trim()
$wasmExec = Join-Path $goRoot "lib\wasm\wasm_exec.js"
if (-not (Test-Path $wasmExec)) {
    $wasmExec = Join-Path $goRoot "misc\wasm\wasm_exec.js"
}
if (-not (Test-Path $wasmExec)) {
    throw "wasm_exec.js not found under $goRoot (lib\wasm or misc\wasm)."
}
Copy-Item $wasmExec (Join-Path $webDir "wasm_exec.js") -Force
Write-Host "copied wasm_exec.js from $wasmExec"

Push-Location $repoRoot
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
try {
    $env:GOOS = "js"
    $env:GOARCH = "wasm"
    $out = Join-Path $webDir "aram.wasm"
    Write-Host "building $out ..."
    Invoke-WebGo build -trimpath -o $out ./cmd/aram-web
    $sizeMB = [math]::Round((Get-Item $out).Length / 1MB, 1)
    Write-Host "built web\aram.wasm ($sizeMB MB)"
} finally {
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    Pop-Location
}

if ($Serve) {
    Write-Host "serving $webDir on http://localhost:$Port  (Ctrl+C to stop)"
    # Static server via Go so no extra tooling is required.
    $serverSrc = @'
package main
import ("log";"net/http";"os")
func main(){
    dir:=os.Args[1]; addr:=os.Args[2]
    log.Printf("serving %s on %s", dir, addr)
    log.Fatal(http.ListenAndServe(addr, http.FileServer(http.Dir(dir))))
}
'@
    $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("aram-web-serve-" + [System.Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    Set-Content -Path (Join-Path $tmp "main.go") -Value $serverSrc -Encoding utf8
    try {
        & go run (Join-Path $tmp "main.go") $webDir ("localhost:" + $Port)
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}
