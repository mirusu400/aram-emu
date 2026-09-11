//go:build (windows || linux || darwin) && !android && !ios

package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseDesktopArgumentsProfile(t *testing.T) {
	for _, args := range [][]string{
		{"--profile", "j2me-1.0/lgt/generic", "game.zip"},
		{"game.zip", "--profile=j2me-1.0/lgt/generic"},
		{openAfterInstallArgument, "--profile=j2me-1.0/lgt/generic", "game.zip"},
	} {
		got, err := parseDesktopArguments(args)
		if err != nil || got.initialArgument != "game.zip" || got.profileID != "j2me-1.0/lgt/generic" {
			t.Fatalf("parseDesktopArguments(%q) = %+v, %v", args, got, err)
		}
		request := got.localOpenRequest()
		if request.Path != "game.zip" || request.ProfileID != got.profileID {
			t.Fatalf("local request = %+v", request)
		}
		if want := []string{"--profile", got.profileID, "game.zip"}; !reflect.DeepEqual(got.relaunchArguments(), want) {
			t.Fatalf("relaunch = %q, want %q", got.relaunchArguments(), want)
		}
	}
}

func TestParseDesktopArgumentsRejectsInvalidProfile(t *testing.T) {
	for _, args := range [][]string{
		{"--profile"}, {"game.zip", "--profile"},
		{"--profile="}, {"--profile", "", "game.zip"},
		{"--profile", "  ", "game.zip"}, {"--profile", "--other", "game.zip"},
		{"--profile", "p"},
		{"--profile=p", "--profile=q", "game.zip"},
		{"--profile=p", "--profile", "p", "game.zip"},
		{"--profile=p", "a.zip", "b.zip"},
		{"--profile=p", "aram://open?app=invalid"},
		{"--profile=p", "https://aram.mir.sh/player/?app=invalid"},
	} {
		if got, err := parseDesktopArguments(args); err == nil {
			t.Errorf("parseDesktopArguments(%q) = %+v, expected error", args, got)
		}
	}
}

func TestDesktopArgumentsOrdinaryAndRemoteLaunch(t *testing.T) {
	for _, argument := range []string{"", "game.zip", "aram://open?app=invalid"} {
		var args []string
		if argument != "" {
			args = []string{argument}
		}
		got, err := parseDesktopArguments(args)
		if err != nil || got.initialArgument != argument || got.profileID != "" {
			t.Fatalf("parseDesktopArguments(%q) = %+v, %v", args, got, err)
		}
	}
	first, _ := parseDesktopArguments([]string{"--profile=p", "a.zip"})
	next, _ := parseDesktopArguments([]string{"b.zip"})
	if first.localOpenRequest().ProfileID != "p" || next.localOpenRequest().ProfileID != "" {
		t.Fatal("profile must belong only to its initial request")
	}
}

func TestUpdateRelaunchProfileScope(t *testing.T) {
	initial := []string{"--profile", "p", "a.zip"}
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"", initial}, {"a.zip", initial}, {"b.zip", []string{"b.zip"}},
	} {
		if got := updateRelaunchArguments(initial, tc.path); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("updateRelaunchArguments(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

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
