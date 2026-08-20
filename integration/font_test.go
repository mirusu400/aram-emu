package integration

import (
	"strings"
	"testing"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-frontend/frontend"
)

// minimalBDF is a one-glyph BDF used to exercise custom-font registration.
const minimalBDF = `STARTFONT 2.1
FONT test
SIZE 16 75 75
FONTBOUNDINGBOX 12 12 0 0
STARTPROPERTIES 2
FONT_ASCENT 9
FONT_DESCENT 3
ENDPROPERTIES
CHARS 1
STARTCHAR A
ENCODING 65
DWIDTH 6 0
BBX 5 7 0 0
BITMAP
70
88
88
F8
88
88
88
ENDCHAR
ENDFONT
`

// TestConfigureFontAppliesToFactory verifies the frontend font selection
// reaches the core factory so the next opened title renders with it. An empty
// selection is left for the core to default (galmuri9); an explicit name is
// applied verbatim.
func TestConfigureFontAppliesToFactory(t *testing.T) {
	backend := NewBackend(nil)

	if got := factoryFallbackFont(t, backend); got != "" {
		t.Fatalf("initial factory FallbackFont = %q, want empty (core default)", got)
	}

	for _, name := range []string{"neodgm", "galmuri9", "mulmaru"} {
		if err := backend.ConfigureFont(frontend.FontSettings{Name: name}); err != nil {
			t.Fatalf("ConfigureFont(%q): %v", name, err)
		}
		if got := factoryFallbackFont(t, backend); got != name {
			t.Fatalf("after ConfigureFont(%q) factory FallbackFont = %q", name, got)
		}
	}
}

// TestConfigureFontRegistersCustomFile verifies a user-supplied font file is
// built, registered, and selected under a content-addressed name.
func TestConfigureFontRegistersCustomFile(t *testing.T) {
	backend := NewBackend(nil)
	if err := backend.ConfigureFont(frontend.FontSettings{
		Name: "custom",
		Data: []byte(minimalBDF),
	}); err != nil {
		t.Fatalf("ConfigureFont(custom): %v", err)
	}
	got := factoryFallbackFont(t, backend)
	if !strings.HasPrefix(got, "custom:") {
		t.Fatalf("factory FallbackFont = %q, want a custom: name", got)
	}

	// A malformed font is reported, not silently accepted.
	if err := backend.ConfigureFont(frontend.FontSettings{
		Name: "custom",
		Data: []byte("this is not a font"),
	}); err == nil {
		t.Fatal("ConfigureFont with garbage data returned nil error")
	}
}

func factoryFallbackFont(t *testing.T, backend *Backend) string {
	t.Helper()
	factory, ok := backend.factoryForCreate().(application.Factory)
	if !ok {
		t.Fatal("default factory is not application.Factory")
	}
	return factory.FallbackFont
}
