//go:build android || ios

// Package mobile exposes the integrated ARAM product to ebitenmobile.
package mobile

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/mobile"

	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-frontend/frontend"
)

var game productGame

type productGame struct {
	once  sync.Once
	shell *frontend.Shell
}

func (g *productGame) Update() error {
	return g.instance().Update()
}

func (g *productGame) Draw(screen *ebiten.Image) {
	g.instance().Draw(screen)
}

func (g *productGame) Layout(width, height int) (int, int) {
	return g.instance().Layout(width, height)
}

func (g *productGame) instance() *frontend.Shell {
	g.once.Do(func() {
		g.shell = frontend.NewShell(
			integration.NewBackend(nil),
			frontend.NewPlatformPicker(),
			"",
		)
	})
	return g.shell
}

// Host is implemented by the native application layer. The host owns the
// platform document picker and returns a private, backend-readable file path.
type Host interface {
	RequestDocument(firmware bool)
}

func init() {
	mobile.SetGame(&game)
}

// ConfigureStorage sets an app-private root before the frontend loads user
// settings. Android does not populate HOME or XDG_CONFIG_HOME for Go code.
func ConfigureStorage(root string) {
	if root != "" {
		_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	}
	game.instance()
}

// SetHost connects the platform picker to the active native Activity.
func SetHost(host Host) {
	frontend.SetNativePickerHost(host)
}

// OpenDocument opens a package imported by the native host.
func OpenDocument(path, displayName string) {
	game.instance().OpenExternalDocument(path, displayName, false)
}

// OpenFirmware opens firmware imported by the native host.
func OpenFirmware(path, displayName string) {
	game.instance().OpenExternalDocument(path, displayName, true)
}

// DocumentSelectionCanceled restores the frontend after a native picker is
// dismissed without a selection.
func DocumentSelectionCanceled() {
	game.instance().CancelExternalDocumentSelection()
}

// Command invokes a stable frontend command ID from the native host.
func Command(commandID string) {
	game.instance().DispatchExternalCommand(commandID)
}

// Pause and Resume mirror native Activity lifecycle transitions.
func Pause() {
	game.instance().SetHostActive(false)
}

func Resume() {
	game.instance().SetHostActive(true)
}

// AudioFocus mirrors Android audio focus callbacks.
func AudioFocus(active bool) {
	game.instance().SetHostActive(active)
}

// Dummy forces gomobile/ebitenmobile to bind this package.
func Dummy() {}
