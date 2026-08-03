package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-core/cheat"
	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-frontend/frontend"
)

// The cheat database is a separate repository so cheats ship without a product
// release. Documents are per title and named by the input file's SHA-256.
const (
	defaultCheatDatabaseURL = "https://raw.githubusercontent.com/mirusu400/aram-cheat/main"
	cheatDatabaseEnv        = "ARAM_CHEAT_DATABASE"
	cheatDirectoryEnv       = "ARAM_CHEAT_DIR"
	maxCheatCatalogBytes    = 1 << 20
	cheatFetchTimeout       = 20 * time.Second
)

// ErrNoPublishedCheats reports that the cheat database has no document for the
// loaded title, which is an ordinary answer rather than a failure.
var ErrNoPublishedCheats = errors.New("the cheat database publishes no cheats for this title")

const (
	cheatActionRefresh = "refresh"
	cheatActionEnable  = "enable"
	cheatActionDisable = "disable"
	cheatFieldCheat    = "cheat"
)

// cheatCatalogStore resolves per-title catalogs from a local directory, a cache
// written by an earlier fetch, or the published cheat database.
type cheatCatalogStore struct {
	client    *http.Client
	baseURL   string
	cacheRoot string
	localDir  string
}

func newCheatCatalogStore() *cheatCatalogStore {
	baseURL := strings.TrimSpace(os.Getenv(cheatDatabaseEnv))
	if baseURL == "" {
		baseURL = defaultCheatDatabaseURL
	}
	return &cheatCatalogStore{
		client:   &http.Client{Timeout: cheatFetchTimeout},
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		localDir: strings.TrimSpace(os.Getenv(cheatDirectoryEnv)),
	}
}

// load prefers a document that is already on disk so opening the panel stays
// offline, and downloads only when nothing local answers for the title.
//
// Identities are tried in order. The loaded image identity comes first because
// it survives repackaging; a container hash answers for entries published
// before an image identity was known.
func (store *cheatCatalogStore) load(
	ctx context.Context,
	identities []string,
) (cheat.Catalog, string, error) {
	for _, identity := range identities {
		if store.localDir == "" {
			break
		}
		data, err := os.ReadFile(store.titlePath(store.localDir, identity))
		if err == nil {
			catalog, err := cheat.ParseCatalog(data)
			if err != nil {
				return cheat.Catalog{}, "", err
			}
			return catalog, "local directory", nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return cheat.Catalog{}, "", err
		}
	}
	for _, identity := range identities {
		cachePath, err := store.cachePath(identity)
		if err != nil {
			continue
		}
		data, readErr := os.ReadFile(cachePath)
		if readErr != nil {
			continue
		}
		if catalog, parseErr := cheat.ParseCatalog(data); parseErr == nil {
			return catalog, "cache", nil
		}
		// A cached document that no longer parses must not wedge the panel;
		// fall through and fetch a fresh copy.
		_ = os.Remove(cachePath)
	}
	catalog, err := store.fetch(ctx, identities)
	if err != nil {
		return cheat.Catalog{}, "", err
	}
	return catalog, "cheat database", nil
}

// fetch downloads and caches the published document for one title, trying each
// identity until the database answers with something other than a 404.
func (store *cheatCatalogStore) fetch(
	ctx context.Context,
	identities []string,
) (cheat.Catalog, error) {
	if len(identities) == 0 {
		return cheat.Catalog{}, errors.New("loaded input has no hash identity")
	}
	for index, identity := range identities {
		catalog, err := store.fetchOne(ctx, identity)
		if err == nil {
			return catalog, nil
		}
		if errors.Is(err, ErrNoPublishedCheats) && index < len(identities)-1 {
			continue
		}
		return cheat.Catalog{}, err
	}
	return cheat.Catalog{}, ErrNoPublishedCheats
}

func (store *cheatCatalogStore) fetchOne(
	ctx context.Context,
	sha256 string,
) (cheat.Catalog, error) {
	endpoint, err := store.titleURL(sha256)
	if err != nil {
		return cheat.Catalog{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return cheat.Catalog{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := store.client.Do(request)
	if err != nil {
		return cheat.Catalog{}, fmt.Errorf("reach the cheat database: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxCheatCatalogBytes))
		_ = response.Body.Close()
	}()
	if response.StatusCode == http.StatusNotFound {
		return cheat.Catalog{}, ErrNoPublishedCheats
	}
	if response.StatusCode != http.StatusOK {
		return cheat.Catalog{}, fmt.Errorf(
			"cheat database responded with %s",
			response.Status,
		)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCheatCatalogBytes+1))
	if err != nil {
		return cheat.Catalog{}, err
	}
	if len(data) > maxCheatCatalogBytes {
		return cheat.Catalog{}, fmt.Errorf(
			"cheat catalog exceeds the %d byte safety limit",
			maxCheatCatalogBytes,
		)
	}
	catalog, err := cheat.ParseCatalog(data)
	if err != nil {
		return cheat.Catalog{}, err
	}
	if !slices.ContainsFunc(catalog.Title.Identities(), func(identity string) bool {
		return strings.EqualFold(identity, sha256)
	}) {
		return cheat.Catalog{}, fmt.Errorf(
			"cheat catalog answers for %s but claims %s",
			sha256,
			strings.Join(catalog.Title.Identities(), ", "),
		)
	}
	store.writeCache(sha256, data)
	return catalog, nil
}

func (store *cheatCatalogStore) writeCache(sha256 string, data []byte) {
	path, err := store.cachePath(sha256)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

func (store *cheatCatalogStore) titleURL(sha256 string) (string, error) {
	if err := validateTitleHash(sha256); err != nil {
		return "", err
	}
	parsed, err := url.Parse(store.baseURL + "/titles/" + sha256 + ".json")
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
			return "", fmt.Errorf("cheat database %q must use HTTPS", store.baseURL)
		}
	default:
		return "", fmt.Errorf("cheat database %q must use HTTPS", store.baseURL)
	}
	return parsed.String(), nil
}

func (store *cheatCatalogStore) titlePath(root, sha256 string) string {
	return filepath.Join(root, "titles", sha256+".json")
}

func (store *cheatCatalogStore) cachePath(sha256 string) (string, error) {
	if err := validateTitleHash(sha256); err != nil {
		return "", err
	}
	root := store.cacheRoot
	if root == "" {
		configRoot, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(configRoot, "ARAM", "cheats")
	}
	return store.titlePath(root, sha256), nil
}

// validateTitleHash keeps a title identity from escaping the cache directory or
// the database URL path.
func validateTitleHash(sha256 string) error {
	if len(sha256) != 64 {
		return errors.New("loaded input has no SHA-256 identity")
	}
	for _, character := range sha256 {
		switch {
		case character >= '0' && character <= '9',
			character >= 'a' && character <= 'f':
		default:
			return fmt.Errorf("input SHA-256 %q is not lowercase hexadecimal", sha256)
		}
	}
	return nil
}

// attachCheats wraps a freshly loaded machine so hash-keyed cheats can be
// applied. A machine the cheat engine cannot describe is returned unchanged
// with the reason recorded for the panel.
func attachCheats(machine aramcore.Machine) (aramcore.Machine, *cheat.Library, string) {
	wrapped, err := application.AttachCheats(machine, cheat.Options{})
	if err != nil {
		return machine, nil, err.Error()
	}
	library, err := cheat.NewLibrary(wrapped.Cheats())
	if err != nil {
		return machine, nil, err.Error()
	}
	return wrapped, library, ""
}

// unwrapMachine reaches through wrappers such as the cheat machine so a host
// can probe the optional reporting interfaces the core machine implements.
// Only read-only probes may use it; running or mutating the guest must go
// through the published machine.
func unwrapMachine(machine aramcore.Machine) aramcore.Machine {
	for {
		unwrapper, ok := machine.(interface{ Unwrap() aramcore.Machine })
		if !ok {
			return machine
		}
		inner := unwrapper.Unwrap()
		if inner == nil {
			return machine
		}
		machine = inner
	}
}

func (backend *Backend) cheatLibrary() (*cheat.Library, string) {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	return backend.cheats, backend.cheatUnavailable
}

// ensureCheatCatalog imports the published catalog once per loaded title. A
// refresh always re-reads the database so a newly published cheat appears
// without reopening the application.
func (backend *Backend) ensureCheatCatalog(
	ctx context.Context,
	library *cheat.Library,
	refresh bool,
) (string, error) {
	backend.mu.RLock()
	store := backend.cheatStore
	imported := backend.cheatImported
	backend.mu.RUnlock()

	if store == nil {
		return "", errors.New("no cheat database is configured")
	}
	if imported && !refresh {
		return backend.cheatSource(), nil
	}
	identities := backend.titleIdentities()

	var (
		catalog cheat.Catalog
		source  string
		err     error
	)
	if refresh {
		catalog, err = store.fetch(ctx, identities)
		source = "cheat database"
	} else {
		catalog, source, err = store.load(ctx, identities)
	}
	if err != nil {
		return "", err
	}
	if err := library.Import(catalog); err != nil {
		return "", err
	}

	backend.mu.Lock()
	backend.cheatImported = true
	backend.cheatCatalogSource = source
	backend.mu.Unlock()
	return source, nil
}

func (backend *Backend) cheatSource() string {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	return backend.cheatCatalogSource
}

// titleIdentities lists the hashes a catalog may be published under, the
// loaded image first because it survives repackaging.
func (backend *Backend) titleIdentities() []string {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	identities := make([]string, 0, 2)
	if backend.imageSHA256 != "" {
		identities = append(identities, backend.imageSHA256)
	}
	if backend.input.SHA256 != "" {
		identities = append(identities, backend.input.SHA256)
	}
	return identities
}

func (backend *Backend) cheatSnapshot(
	ctx context.Context,
	refresh bool,
	status string,
) (frontend.ToolSnapshot, error) {
	library, unavailable := backend.cheatLibrary()
	if library == nil {
		reason := unavailable
		if reason == "" {
			reason = "no application is loaded"
		}
		return frontend.ToolSnapshot{}, backendError(
			frontend.FailureBackendUnavailable,
			fmt.Errorf("cheats are unavailable: %s", reason),
		)
	}

	source, err := backend.ensureCheatCatalog(ctx, library, refresh)
	if err != nil {
		if errors.Is(err, ErrNoPublishedCheats) {
			backend.mu.RLock()
			file := backend.input.SHA256
			image := backend.imageSHA256
			backend.mu.RUnlock()
			return frontend.ToolSnapshot{
				Title: "Cheat Manager",
				Lines: []string{
					"The cheat database publishes no cheats for this title yet.",
					"",
					"Image SHA-256: " + emptyFallback(image, "unavailable"),
					"File SHA-256: " + file,
					"Database: " + defaultCheatDatabaseURL,
				},
				Actions: []frontend.ToolAction{{
					ID:      cheatActionRefresh,
					Label:   "Check the cheat database",
					Enabled: true,
				}},
			}, nil
		}
		return frontend.ToolSnapshot{}, err
	}
	return backend.cheatPanel(library, source, status), nil
}

func (backend *Backend) cheatPanel(
	library *cheat.Library,
	source string,
	status string,
) frontend.ToolSnapshot {
	title := library.Title()
	entries := library.Entries()

	lines := []string{
		"Title: " + emptyFallback(title.Name, "unnamed"),
		"Image SHA-256: " + title.ImageSHA256,
		"Catalog: " + emptyFallback(source, "unknown source"),
		"",
	}
	if status != "" {
		lines = append(lines, status, "")
	}
	options := make([]frontend.ToolFieldOption, 0, len(entries))
	selected := ""
	for _, entry := range entries {
		mark := "off"
		if entry.Enabled {
			mark = "ON "
		}
		lines = append(lines, fmt.Sprintf(
			"[%s] %s (%s)",
			mark,
			entry.Cheat.Name,
			entry.Cheat.ID,
		))
		if entry.Cheat.Description != "" {
			lines = append(lines, "      "+entry.Cheat.Description)
		}
		if selected == "" {
			selected = entry.Cheat.ID
		}
		options = append(options, frontend.ToolFieldOption{
			Value: entry.Cheat.ID,
			Label: entry.Cheat.Name,
		})
	}
	sort.SliceStable(options, func(left, right int) bool {
		return options[left].Label < options[right].Label
	})
	lines = append(lines,
		"",
		"Cheats are bound to this title's SHA-256 and verify the original",
		"bytes before they are applied.",
	)

	return frontend.ToolSnapshot{
		Title: "Cheat Manager",
		Lines: lines,
		Fields: []frontend.ToolField{{
			ID:      cheatFieldCheat,
			Label:   "Cheat",
			Value:   selected,
			Options: options,
		}},
		Actions: []frontend.ToolAction{
			{ID: cheatActionEnable, Label: "Enable", Enabled: len(entries) != 0},
			{ID: cheatActionDisable, Label: "Disable", Enabled: len(entries) != 0},
			{ID: cheatActionRefresh, Label: "Update from cheat database", Enabled: true},
		},
	}
}

func (backend *Backend) executeCheatAction(
	ctx context.Context,
	request frontend.ToolRequest,
) (frontend.ToolSnapshot, error) {
	switch request.Action {
	case cheatActionRefresh:
		return backend.cheatSnapshot(ctx, true, "Catalog updated.")
	case cheatActionEnable, cheatActionDisable:
		library, _ := backend.cheatLibrary()
		if library == nil {
			return backend.cheatSnapshot(ctx, false, "")
		}
		id := strings.TrimSpace(request.Fields[cheatFieldCheat])
		if id == "" {
			return backend.cheatSnapshot(ctx, false, "Select a cheat first.")
		}
		enable := request.Action == cheatActionEnable
		if err := library.SetEnabled(id, enable); err != nil {
			return backend.cheatSnapshot(ctx, false, "Failed: "+err.Error())
		}
		verb := "Enabled"
		if !enable {
			verb = "Disabled"
		}
		return backend.cheatSnapshot(ctx, false, verb+" "+id+".")
	default:
		return frontend.ToolSnapshot{}, fmt.Errorf(
			"unknown cheat action %q",
			request.Action,
		)
	}
}
