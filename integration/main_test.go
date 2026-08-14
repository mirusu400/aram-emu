package integration

import (
	"os"
	"testing"
)

// Opening a backend resolves the published cheat catalog, so every test must
// stay off the live database: point it at a closed local port that refuses
// instantly. Tests that need a database run their own server and override the
// store's base URL.
func TestMain(m *testing.M) {
	os.Setenv(cheatDatabaseEnv, "https://127.0.0.1:1")
	os.Exit(m.Run())
}
