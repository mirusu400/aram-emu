package loader

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Kind string

const (
	KindUnknown  Kind = "unknown"
	KindDAT      Kind = "wipi-dat"
	KindABHS     Kind = "abhs"
	KindEADS     Kind = "eads"
	KindELF      Kind = "elf"
	KindJava     Kind = "java-archive"
	KindWBIN     Kind = "samsung-wbin"
	KindWBT      Kind = "samsung-wbt"
	KindFont     Kind = "samsung-font"
	KindFirmware Kind = "firmware-image"
)

type Marker struct {
	Magic  string `json:"magic"`
	Offset int64  `json:"offset"`
}

type Report struct {
	Path    string   `json:"path"`
	Size    int64    `json:"size"`
	SHA256  string   `json:"sha256"`
	Kind    Kind     `json:"kind"`
	Markers []Marker `json:"markers,omitempty"`
}

func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func InspectFile(path string) (Report, error) {
	file, err := os.Open(path)
	if err != nil {
		return Report{}, fmt.Errorf("open %q: %w", path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Report{}, fmt.Errorf("stat %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Report{}, fmt.Errorf("%q is not a regular file", path)
	}

	hash := sha256.New()
	reader := bufio.NewReaderSize(io.TeeReader(file, hash), 1024*1024)
	var (
		first   []byte
		carry   []byte
		offset  int64
		markers []Marker
	)
	buffer := make([]byte, 1024*1024)
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			chunk := append(append([]byte(nil), carry...), buffer[:count]...)
			if len(first) < 64 {
				needed := 64 - len(first)
				if needed > count {
					needed = count
				}
				first = append(first, buffer[:needed]...)
			}
			base := offset - int64(len(carry))
			markers = appendMarkers(markers, chunk, base)
			if len(chunk) > 3 {
				carry = append(carry[:0], chunk[len(chunk)-3:]...)
			} else {
				carry = append(carry[:0], chunk...)
			}
			offset += int64(count)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return Report{}, fmt.Errorf("read %q: %w", path, readErr)
		}
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	return Report{
		Path:    absolute,
		Size:    info.Size(),
		SHA256:  hex.EncodeToString(hash.Sum(nil)),
		Kind:    detectKind(path, first, markers),
		Markers: markers,
	}, nil
}

func appendMarkers(markers []Marker, data []byte, base int64) []Marker {
	if len(markers) >= 64 {
		return markers
	}
	for _, magic := range []string{"ABHS", "EADS"} {
		search := []byte(magic)
		start := 0
		for len(markers) < 64 {
			index := bytes.Index(data[start:], search)
			if index < 0 {
				break
			}
			position := start + index
			absolute := base + int64(position)
			if absolute >= 0 && !hasMarker(markers, magic, absolute) {
				markers = append(markers, Marker{Magic: magic, Offset: absolute})
			}
			start = position + 1
		}
	}
	return markers
}

func hasMarker(markers []Marker, magic string, offset int64) bool {
	for _, marker := range markers {
		if marker.Magic == magic && marker.Offset == offset {
			return true
		}
	}
	return false
}

func detectKind(path string, first []byte, markers []Marker) Kind {
	switch {
	case bytes.HasPrefix(first, []byte("ABHS")):
		return KindABHS
	case bytes.HasPrefix(first, []byte("EADS")):
		return KindEADS
	case bytes.HasPrefix(first, []byte{0x7f, 'E', 'L', 'F'}):
		return KindELF
	case bytes.HasPrefix(first, []byte{'P', 'K', 3, 4}):
		return KindJava
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".dat":
		return KindDAT
	case ".wbin":
		return KindWBIN
	case ".wbt":
		return KindWBT
	case ".fnt":
		return KindFont
	case ".jar":
		return KindJava
	case ".bin", ".rom", ".img", ".mbn":
		return KindFirmware
	}

	for _, marker := range markers {
		switch marker.Magic {
		case "ABHS":
			return KindABHS
		case "EADS":
			return KindEADS
		}
	}
	return KindUnknown
}
