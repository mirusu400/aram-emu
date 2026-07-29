package frontend

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestAddRecentDeduplicatesAndLimits(t *testing.T) {
	settings := defaultSettings()
	for index := 0; index < recentFileLimit+3; index++ {
		settings.addRecent(filepath.Join("games", fmt.Sprintf("%02d.dat", index)))
	}
	if len(settings.RecentFiles) != recentFileLimit {
		t.Fatalf("recent count = %d, want %d", len(settings.RecentFiles), recentFileLimit)
	}
	newest := filepath.Clean(filepath.Join("games", "12.dat"))
	if !filepath.IsAbs(settings.RecentFiles[0]) {
		t.Fatalf("recent path is not absolute: %q", settings.RecentFiles[0])
	}
	if filepath.Base(settings.RecentFiles[0]) != filepath.Base(newest) {
		t.Fatalf("newest = %q", settings.RecentFiles[0])
	}

	settings.addRecent(settings.RecentFiles[4])
	if len(settings.RecentFiles) != recentFileLimit {
		t.Fatalf("deduplication changed count to %d", len(settings.RecentFiles))
	}
}
