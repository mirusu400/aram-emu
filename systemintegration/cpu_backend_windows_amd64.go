//go:build windows && amd64

package systemintegration

// Whole-phone cold boot is a mixed ARM/Thumb workload. On Windows/amd64 the
// Go JIT sustains handset speed while the native JIT, which wins application
// workloads, falls below it. Keep this product policy host- and workload-
// specific; users who explicitly select native still get native.
const fastestSystemCPUBackend = "jit"
