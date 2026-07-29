package frontend

import "testing"

func TestFrontendKeepsGenericEmulatorMenus(t *testing.T) {
	menus := defaultMenus()
	wantMenus := []string{"File", "Emulation", "View", "Tools", "Help"}
	if len(menus) != len(wantMenus) {
		t.Fatalf("menu count = %d, want %d", len(menus), len(wantMenus))
	}
	for index, want := range wantMenus {
		if menus[index].Label != want {
			t.Fatalf("menu[%d] = %q, want %q", index, menus[index].Label, want)
		}
	}

	required := map[string]bool{
		"file.open":          false,
		"file.open_firmware": false,
		"emu.save_state":     false,
		"emu.rewind":         false,
		"tools.cheats":       false,
		"tools.memory":       false,
		"tools.debugger":     false,
		"tools.controller":   false,
		"view.fullscreen":    false,
		"view.screenshot":    false,
	}
	for _, menu := range menus {
		for _, command := range menu.Commands {
			if _, ok := required[command.ID]; ok {
				required[command.ID] = true
			}
		}
	}
	for command, found := range required {
		if !found {
			t.Errorf("required frontend command %q is missing", command)
		}
	}
}
