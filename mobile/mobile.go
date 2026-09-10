//go:build android || ios

// Package mobile exposes the integrated ARAM product to ebitenmobile.
package mobile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/mobile"

	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-emu/internal/remoteinput"
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
	// RequestDocument presents the platform picker for one document kind -
	// frontend.DocumentKindInput, DocumentKindFirmware or
	// DocumentKindSaveBackup. The answer comes back through OpenDocument,
	// OpenFirmware, OpenSaveBackup or DocumentSelectionCanceled.
	RequestDocument(kind string)
	// ShareFile hands one file below the app's private storage to another
	// app. A save backup is written into private storage, so this is the only
	// way it reaches a place that survives uninstalling the app.
	ShareFile(path, mimeType, title string) error
	// InstallPackage hands a verified product package below the configured
	// update folder to the platform installer. An error reports that the
	// installer could not be opened; the installation itself finishes
	// outside the app.
	InstallPackage(path string) error
	// RequestTextInput presents the platform text editor for one form field.
	// Ebitengine raises no soft keyboard on a handset, so the host types the
	// text and answers once with SubmitTextInput or CancelTextInput.
	RequestTextInput(requestID int64, label, hint, text string)
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

// SetHost connects the platform picker, package installer, and text editor to
// the active native Activity.
func SetHost(host Host) {
	hostBridge.Lock()
	hostBridge.host = host
	hostBridge.Unlock()
	if host == nil {
		frontend.SetNativePickerHost(nil)
		frontend.SetNativeTextInputHost(nil)
		frontend.SetNativeShareHost(nil)
		return
	}
	frontend.SetNativePickerHost(host)
	frontend.SetNativeTextInputHost(host)
	frontend.SetNativeShareHost(host)
}

// SubmitTextInput reports the text the native editor accepted for the field
// identified by requestID.
func SubmitTextInput(requestID int64, text string) {
	frontend.SubmitNativeTextInput(requestID, text)
}

// CancelTextInput reports that the native editor was dismissed unchanged.
func CancelTextInput(requestID int64) {
	frontend.CancelNativeTextInput(requestID)
}

// OpenDocument opens a package imported by the native host.
func OpenDocument(path, displayName string) {
	game.instance().OpenExternalDocument(path, displayName, false)
}

// OpenLink downloads, verifies, persists, and opens an ARAM web-player or
// aram:// deep link. Native hosts should call it away from their UI thread.
// The returned string is empty on success and suitable for a platform error UI.
func OpenLink(raw string) string {
	shell := game.instance()
	shell.ReportExternalOpenStatus("Downloading and verifying linked package...")
	path, spec, err := remoteinput.Resolve(context.Background(), nil, "", raw)
	if err != nil {
		message := "Open link: " + err.Error()
		shell.ReportExternalOpenStatus(message)
		return message
	}
	shell.OpenExternalRequest(frontend.OpenRequest{
		Path:           path,
		DisplayName:    spec.Name,
		ExpectedSHA256: spec.SHA256,
	})
	return ""
}

// OpenFirmware opens firmware imported by the native host.
func OpenFirmware(path, displayName string) {
	game.instance().OpenExternalDocument(path, displayName, true)
}

// OpenSaveBackup restores a save backup file the native host imported into
// app-private storage.
func OpenSaveBackup(path string) {
	game.instance().ImportExternalSaveBackup(path)
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

// PressControl reports that a control on a second physical panel - a keypad
// Activity on the handset's other display - was pressed or released. The
// control names match the on-screen deck (dpad, soft keys, num0-9, ...), so
// the press runs through the shared frontend's ordinary input path.
func PressControl(control string, down bool) {
	game.instance().SetHostControl(control, down)
}

// SetSecondaryKeypadActive tells the frontend whether a second physical panel
// is currently showing the keypad. While active the shell hides its on-screen
// control deck and keypad so the game panel is unobstructed.
func SetSecondaryKeypadActive(active bool) {
	game.instance().SetSecondaryKeypadActive(active)
}

// SetControllerConnected reports whether the host sees a physical controller.
// On a touch layout the frontend then hides its on-screen controls by default,
// since the player has real buttons; a settings toggle keeps them if wanted.
func SetControllerConnected(connected bool) {
	game.instance().SetControllerConnected(connected)
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
