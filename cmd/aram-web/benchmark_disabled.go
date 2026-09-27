//go:build js && wasm && !aram_benchmark

package main

import (
	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-frontend/frontend"
)

func configureBenchmark(backend *integration.Backend) frontend.Backend { return backend }
