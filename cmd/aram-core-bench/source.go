package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// openBenchmarkSource keeps private inputs on the operator's host. Android
// runners can consume the same bytes over an explicit adb-reversed loopback
// port; only generated executables are pushed to the device. Existing size and
// SHA-256 validation still apply before any measured guest execution begins.
func openBenchmarkSource(ctx context.Context, path, rawURL string) (io.ReadCloser, error) {
	if (path == "") == (rawURL == "") {
		return nil, errors.New("exactly one benchmark source required")
	}
	if rawURL == "" {
		return os.Open(path)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Fragment != "" ||
		(parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "::1") {
		return nil, errors.New("benchmark URL must use literal HTTP loopback")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("benchmark URL requires an explicit valid port")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("invalid benchmark request")
	}
	request.Close = true
	client := &http.Client{
		Timeout: 30 * time.Second,
		// Keep this stream local regardless of environment proxy settings or
		// changes to the process-wide default transport.
		Transport: &http.Transport{Proxy: nil},
		// No redirects, including another loopback endpoint. A private input
		// must never turn into an outbound request or leak a URL's credentials.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("benchmark redirects are forbidden")
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("benchmark loopback request failed")
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, errors.New("benchmark loopback source unavailable")
	}
	return response.Body, nil
}
