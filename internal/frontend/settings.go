package frontend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const recentFileLimit = 10

type Settings struct {
	RecentFiles      []string `json:"recent_files"`
	IntegerScaling   bool     `json:"integer_scaling"`
	PreserveAspect   bool     `json:"preserve_aspect"`
	LastFirmwarePath string   `json:"last_firmware_path,omitempty"`
}

func defaultSettings() Settings {
	return Settings{
		IntegerScaling: true,
		PreserveAspect: true,
	}
}

func loadSettings() Settings {
	settings := defaultSettings()
	path, err := settingsPath()
	if err != nil {
		return settings
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return settings
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return defaultSettings()
	}
	if len(settings.RecentFiles) > recentFileLimit {
		settings.RecentFiles = settings.RecentFiles[:recentFileLimit]
	}
	return settings
}

func (s *Settings) addRecent(path string) {
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	path = filepath.Clean(path)
	recent := []string{path}
	for _, existing := range s.RecentFiles {
		if strings.EqualFold(filepath.Clean(existing), path) {
			continue
		}
		recent = append(recent, existing)
		if len(recent) == recentFileLimit {
			break
		}
	}
	s.RecentFiles = recent
}

func (s Settings) save() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func settingsPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "ARAM", "settings.json"), nil
}
