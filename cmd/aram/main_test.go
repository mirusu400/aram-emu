//go:build (windows || linux || darwin) && !android && !ios

package main

import (
	"os"
	"strings"
	"testing"
)

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

func TestPlatformPackagesRegisterARAMLinks(t *testing.T) {
	manifest, err := os.ReadFile("../../android/app/src/main/AndroidManifest.xml")
	if err != nil {
		t.Fatal(err)
	}
	manifestText := string(manifest)
	for _, required := range []string{
		`android:autoVerify="true"`,
		`android:scheme="aram"`,
		`android:host="open"`,
		`android:host="aram.mir.sh"`,
		`android:pathPrefix="/player"`,
	} {
		if !strings.Contains(manifestText, required) {
			t.Errorf("Android manifest is missing %s", required)
		}
	}

	plist, err := os.ReadFile("../../packaging/macos/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	plistText := string(plist)
	if !strings.Contains(plistText, "CFBundleURLTypes") ||
		!strings.Contains(plistText, "<string>aram</string>") {
		t.Fatal("macOS bundle does not register the aram URL scheme")
	}
}
