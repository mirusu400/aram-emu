//go:build js && wasm

// Command aram-web is the browser/WebAssembly entry point for the ARAM
// product. It wires the same aram-core integration backend the desktop
// cmd/aram uses to the shared frontend, which Ebitengine renders onto a
// browser <canvas>. Build with:
//
//	GOOS=js GOARCH=wasm go build -o web/aram.wasm ./cmd/aram-web
//
// then serve web/index.html (which loads wasm_exec.js and aram.wasm). The web
// frontend picker reads a chosen file into memory and loads it through
// OpenRequest.Data, so no server-side filesystem is involved.
package main

import (
	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-frontend/frontend"
)

func main() {
	backend := integration.NewBackend(nil)
	if err := frontend.Run(backend, ""); err != nil {
		panic(err)
	}
}
