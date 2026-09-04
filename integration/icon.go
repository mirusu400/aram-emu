package integration

import "github.com/mirusu400/aram-core/loader"

// Icon returns a PNG-encoded application icon for the package at path, or an
// error (loader.ErrNoIcon for formats that carry none). It is path-keyed and
// stateless, so the launcher can request an icon before any title is opened.
// Icon parsing lives in aram-core; the adapter only translates.
func (backend *Backend) Icon(path string) ([]byte, error) {
	return loader.Icon(path)
}
