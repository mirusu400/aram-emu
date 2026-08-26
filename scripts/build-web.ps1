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

# Locate wasm_exec.js in the active Go toolchain (Go >= 1.24 uses lib\wasm,
# older toolchains use misc\wasm).
$goRoot = (& go env GOROOT).Trim()
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
try {
    $env:GOOS = "js"
    $env:GOARCH = "wasm"
    $out = Join-Path $webDir "aram.wasm"
    Write-Host "building $out ..."
    & go build -trimpath -o $out ./cmd/aram-web
    if ($LASTEXITCODE -ne 0) { throw "go build failed ($LASTEXITCODE)" }
    $sizeMB = [math]::Round((Get-Item $out).Length / 1MB, 1)
    Write-Host "built web\aram.wasm ($sizeMB MB)"
} finally {
    Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
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
