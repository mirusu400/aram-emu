//go:build (windows || linux || darwin) && !android && !ios

package main

import "testing"

func TestParseArgumentsPreservesInputAndInternalOpenRequest(t *testing.T) {
	path, open := parseArguments([]string{
		openAfterInstallArgument,
		"game.zip",
	})
	if path != "game.zip" || !open {
		t.Fatalf("parseArguments() = %q, %t", path, open)
	}
}

func TestParseArgumentsDefaultsToOrdinaryLaunch(t *testing.T) {
	path, open := parseArguments(nil)
	if path != "" || open {
		t.Fatalf("parseArguments(nil) = %q, %t", path, open)
	}
}
