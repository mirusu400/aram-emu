// Package remoteinput resolves integrity-pinned ARAM player links into
// persistent, backend-readable package files.
package remoteinput

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const MaxPackageBytes int64 = 32 * 1024 * 1024

var digestLocks sync.Map

// Spec is the trusted subset of an ARAM web-player permalink.
type Spec struct {
	URL    string
	Name   string
	SHA256 string
}

// Parse recognizes the public web-player permalink and the aram://open wrapper
// used by native desktop and mobile apps. Ordinary filesystem paths return
// ok=false so callers can preserve the existing file-open behavior.
func Parse(raw string) (spec Spec, ok bool, err error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return Spec{}, false, nil
	}
	switch strings.ToLower(parsed.Scheme) {
	case "aram":
		if !strings.EqualFold(parsed.Host, "open") || (parsed.Path != "" && parsed.Path != "/") {
			return Spec{}, true, errors.New("the ARAM link must use aram://open")
		}
		values := parsed.Query()
		wrapped, err := singleValue(values, "url", false)
		if err != nil {
			return Spec{}, true, err
		}
		if wrapped != "" {
			if values.Has("app") || values.Has("sha256") {
				return Spec{}, true, errors.New("the ARAM link must not mix url with app or sha256")
			}
			spec, recognized, err := parseWebLink(wrapped)
			if !recognized && err == nil {
				err = errors.New("the wrapped URL is not an ARAM player link")
			}
			return spec, true, err
		}
		spec, err := parseParameters(values)
		return spec, true, err
	case "https":
		return parseWebLink(raw)
	default:
		return Spec{}, false, nil
	}
}

func parseWebLink(raw string) (Spec, bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Spec{}, true, errors.New("the ARAM player link is not a valid URL")
	}
	if !strings.EqualFold(parsed.Scheme, "https") || !strings.EqualFold(parsed.Hostname(), "aram.mir.sh") || parsed.Port() != "" {
		return Spec{}, true, errors.New("the player link must use https://aram.mir.sh/player/")
	}
	if parsed.User != nil || (parsed.Path != "/player" && parsed.Path != "/player/") {
		return Spec{}, true, errors.New("the player link must use https://aram.mir.sh/player/")
	}
	returnSpec, err := parseParameters(parsed.Query())
	return returnSpec, true, err
}

func parseParameters(values url.Values) (Spec, error) {
	app, err := singleValue(values, "app", true)
	if err != nil {
		return Spec{}, err
	}
	digest, err := singleValue(values, "sha256", true)
	if err != nil {
		return Spec{}, err
	}
	digest = strings.ToLower(digest)
	if len(digest) != 64 {
		return Spec{}, errors.New("the sha256 parameter must be 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return Spec{}, errors.New("the sha256 parameter must be 64 hexadecimal characters")
	}
	packageURL, err := url.Parse(app)
	if err != nil || packageURL.Scheme == "" || packageURL.Host == "" {
		return Spec{}, errors.New("the app parameter is not a valid URL")
	}
	if !strings.EqualFold(packageURL.Scheme, "https") {
		return Spec{}, errors.New("the app URL must use HTTPS")
	}
	if packageURL.User != nil {
		return Spec{}, errors.New("the app URL must not contain credentials")
	}
	packageURL.Fragment = ""
	return Spec{
		URL:    packageURL.String(),
		Name:   packageName(packageURL),
		SHA256: digest,
	}, nil
}

func singleValue(values url.Values, name string, required bool) (string, error) {
	entries, present := values[name]
	if len(entries) > 1 {
		return "", fmt.Errorf("duplicate %s parameter", name)
	}
	if !present || len(entries) == 0 || strings.TrimSpace(entries[0]) == "" {
		if required {
			return "", fmt.Errorf("the %s parameter is required", name)
		}
		return "", nil
	}
	return strings.TrimSpace(entries[0]), nil
}

func packageName(packageURL *url.URL) string {
	name := path.Base(packageURL.Path)
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	return safeName(name)
}

func safeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`\\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		name = "application.zip"
	}
	for len(name) > 128 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name
}

// DefaultRoot returns the persistent app-private package library.
func DefaultRoot() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "ARAM", "downloads"), nil
}

// Resolve parses and downloads one native or web-player link.
func Resolve(ctx context.Context, client *http.Client, root, raw string) (string, Spec, error) {
	spec, ok, err := Parse(raw)
	if err != nil {
		return "", Spec{}, err
	}
	if !ok {
		return "", Spec{}, errors.New("not an ARAM player link")
	}
	if strings.TrimSpace(root) == "" {
		root, err = DefaultRoot()
		if err != nil {
			return "", Spec{}, fmt.Errorf("locate the ARAM download library: %w", err)
		}
	}
	file, err := Download(ctx, client, root, spec)
	return file, spec, err
}

// Download writes a verified package atomically into root and reuses an
// existing file only after checking its SHA-256 identity.
func Download(ctx context.Context, client *http.Client, root string, spec Spec) (string, error) {
	return download(ctx, client, root, spec, MaxPackageBytes)
}

func download(ctx context.Context, client *http.Client, root string, spec Spec, maxBytes int64) (string, error) {
	if maxBytes <= 0 {
		return "", errors.New("the package size limit must be positive")
	}
	if err := validateSpec(spec); err != nil {
		return "", err
	}
	lockValue, _ := digestLocks.LoadOrStore(spec.SHA256, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	target := packagePath(root, spec)
	if matchesDigest(target, spec.SHA256) {
		return target, nil
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("replace the corrupt cached package: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", fmt.Errorf("create the ARAM download library: %w", err)
	}

	temporary, err := os.CreateTemp(filepath.Dir(target), ".download-*.part")
	if err != nil {
		return "", fmt.Errorf("create the package staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create the package request: %w", err)
	}
	request.Header.Set("Accept", "application/zip, application/octet-stream;q=0.9, */*;q=0.1")
	response, err := secureClient(client).Do(request)
	if err != nil {
		return "", fmt.Errorf("download the package: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", fmt.Errorf("download the package: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxBytes {
		return "", fmt.Errorf("the package is too large (maximum %d bytes)", maxBytes)
	}

	hash := sha256.New()
	limited := io.LimitReader(response.Body, maxBytes+1)
	written, err := io.Copy(io.MultiWriter(temporary, hash), limited)
	if err != nil {
		return "", fmt.Errorf("download the package body: %w", err)
	}
	if written > maxBytes {
		return "", fmt.Errorf("the package is too large (maximum %d bytes)", maxBytes)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != spec.SHA256 {
		return "", fmt.Errorf("SHA-256 mismatch (expected %s, got %s)", spec.SHA256, actual)
	}
	if err := temporary.Sync(); err != nil {
		return "", fmt.Errorf("flush the downloaded package: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close the downloaded package: %w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return "", fmt.Errorf("activate the downloaded package: %w", err)
	}
	keep = true
	return target, nil
}

func validateSpec(spec Spec) error {
	parsed, err := url.Parse(spec.URL)
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Scheme, "https") {
		return errors.New("the app URL must use HTTPS")
	}
	if parsed.User != nil {
		return errors.New("the app URL must not contain credentials")
	}
	if len(spec.SHA256) != 64 {
		return errors.New("the package SHA-256 is invalid")
	}
	if _, err := hex.DecodeString(spec.SHA256); err != nil {
		return errors.New("the package SHA-256 is invalid")
	}
	return nil
}

func secureClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{
			Timeout:   30 * time.Second,
			Transport: publicInternetTransport(),
		}
	}
	clone := *base
	if clone.Timeout == 0 {
		clone.Timeout = 30 * time.Second
	}
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		if request.URL.User != nil || !strings.EqualFold(request.URL.Scheme, "https") {
			return errors.New("package redirects must use HTTPS without credentials")
		}
		return nil
	}
	return &clone
}

func publicInternetTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A link is untrusted input. Do not let it turn the installed app into an
	// HTTP client for loopback, link-local, or private LAN services. A direct
	// dial also prevents an environment proxy from bypassing this address check.
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, candidate := range addresses {
			if !publicAddress(candidate) {
				continue
			}
			connection, dialErr := dialer.DialContext(
				ctx,
				network,
				net.JoinHostPort(candidate.String(), port),
			)
			if dialErr == nil {
				return connection, nil
			}
			err = dialErr
		}
		if err != nil {
			return nil, err
		}
		return nil, errors.New("the package host has no public IP address")
	}
	return transport
}

func publicAddress(address netip.Addr) bool {
	return address.IsGlobalUnicast() &&
		!address.IsPrivate() &&
		!address.IsLoopback() &&
		!address.IsLinkLocalUnicast() &&
		!address.IsUnspecified()
}

func packagePath(root string, spec Spec) string {
	return filepath.Join(root, spec.SHA256[:2], spec.SHA256+"-"+safeName(spec.Name))
}

func matchesDigest(file, expected string) bool {
	input, err := os.Open(file)
	if err != nil {
		return false
	}
	defer input.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, input); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == expected
}
