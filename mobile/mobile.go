//go:build android || ios

// Package mobile exposes the integrated ARAM product to ebitenmobile.
package mobile

import (
	"errors"
	"fmt"
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
			&productBackend{Backend: integration.NewBackend(nil)},
			frontend.NewPlatformPicker(),
			"",
		)
	})
	return g.shell
}

// productBackend adds product installation to the integration backend. The
// native host owns the platform package installer, so the backend only hands
// the verified package over and reports that the installation continues there.
type productBackend struct {
	*integration.Backend
}

func (backend *productBackend) InstallProductUpdate(
	update frontend.ProductUpdate,
) error {
	host := currentHost()
	if host == nil {
		return errors.New("the native host is not attached")
	}
	if err := host.InstallPackage(update.ArchivePath); err != nil {
		return fmt.Errorf("open the %s package installer: %w", update.Channel, err)
	}
	return frontend.ErrProductInstallDeferred
}

// Host is implemented by the native application layer. The host owns the
// platform document picker and returns a private, backend-readable file path,
// and it opens the platform package installer for a downloaded product update.
type Host interface {
	RequestDocument(firmware bool)
	// InstallPackage hands a verified product package below the configured
	// update folder to the platform installer. An error reports that the
	// installer could not be opened; the installation itself finishes
	// outside the app.
	InstallPackage(path string) error
}

var hostBridge struct {
	sync.RWMutex
	host Host
}

func currentHost() Host {
	hostBridge.RLock()
	defer hostBridge.RUnlock()
	return hostBridge.host
}

var storageOnce sync.Once

func init() {
	mobile.SetGame(&game)
}

// ConfigureLocale declares the device language before the frontend loads user
// settings, which is where the first-run default comes from. Android starts Go
// without LANG or its relatives, so without this the shell would open in
// English on a Korean handset. Call it before ConfigureStorage; a language the
// user has already chosen is stored in settings and still wins.
func ConfigureLocale(tag string) {
	frontend.SetHostLocale(tag)
}

// ConfigureStorage sets an app-private root before the frontend loads user
// settings. Android does not populate HOME or XDG_CONFIG_HOME for Go code.
// Settings live below root/config and verified update packages below
// root/updates. A package handed to the platform installer must outlive the
// call that handed it over, so the previous launch's packages are reclaimed
// here, the first time a process configures its storage.
func ConfigureStorage(root string) {
	if root != "" {
		storageOnce.Do(func() {
			_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
			updates := filepath.Join(root, "updates")
			_ = os.RemoveAll(updates)
			frontend.SetUpdateDownloadRoot(updates)
		})
	}
	game.instance()
}

// SetHost connects the platform picker and package installer to the active
// native Activity.
func SetHost(host Host) {
	hostBridge.Lock()
	hostBridge.host = host
	hostBridge.Unlock()
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
