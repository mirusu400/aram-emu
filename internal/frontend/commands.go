package frontend

type Command struct {
	ID       string
	Label    string
	Shortcut string
	Enabled  func(*Shell) bool
	Action   func(*Shell)
}

func (c Command) IsEnabled(shell *Shell) bool {
	return c.Enabled == nil || c.Enabled(shell)
}

type Menu struct {
	Label    string
	Commands []Command
}

func defaultMenus() []Menu {
	disabled := func(*Shell) bool { return false }
	hasInput := func(shell *Shell) bool { return shell.report != nil }
	hasRecent := func(shell *Shell) bool { return len(shell.settings.RecentFiles) > 0 }

	return []Menu{
		{
			Label: "File",
			Commands: []Command{
				{ID: "file.open", Label: "Open File...", Shortcut: "Ctrl+O", Action: (*Shell).chooseFile},
				{ID: "file.open_firmware", Label: "Open Firmware Directory...", Action: (*Shell).chooseFirmwareDirectory},
				{ID: "file.recent", Label: "Open Recent...", Enabled: hasRecent, Action: (*Shell).chooseRecent},
				{ID: "file.close", Label: "Close Title", Enabled: hasInput, Action: (*Shell).closeInput},
				{ID: "file.exit", Label: "Exit", Action: func(shell *Shell) { shell.quitting = true }},
			},
		},
		{
			Label: "Emulation",
			Commands: []Command{
				{ID: "emu.start", Label: "Start", Shortcut: "F5", Enabled: disabled},
				{ID: "emu.pause", Label: "Pause / Resume", Shortcut: "F6", Enabled: disabled},
				{ID: "emu.stop", Label: "Stop", Shortcut: "F8", Enabled: disabled},
				{ID: "emu.reset", Label: "Reset", Shortcut: "Ctrl+R", Enabled: disabled},
				{ID: "emu.frame", Label: "Frame Advance", Enabled: disabled},
				{ID: "emu.fast_forward", Label: "Fast Forward", Enabled: disabled},
				{ID: "emu.load_state", Label: "Load State...", Shortcut: "F9", Enabled: disabled},
				{ID: "emu.save_state", Label: "Save State...", Shortcut: "F10", Enabled: disabled},
				{ID: "emu.rewind", Label: "Rewind", Enabled: disabled},
			},
		},
		{
			Label: "View",
			Commands: []Command{
				{ID: "view.fullscreen", Label: "Toggle Fullscreen", Shortcut: "F11", Action: (*Shell).toggleFullscreen},
				{ID: "view.integer", Label: "Integer Scaling", Action: (*Shell).toggleIntegerScaling},
				{ID: "view.aspect", Label: "Preserve Aspect Ratio", Action: (*Shell).toggleAspectRatio},
				{ID: "view.fit", Label: "Fit Window", Enabled: disabled},
				{ID: "view.rotation", Label: "Rotation", Enabled: disabled},
				{ID: "view.layout", Label: "Screen Layout", Enabled: disabled},
				{ID: "view.filter", Label: "Filter", Enabled: disabled},
				{ID: "view.screenshot", Label: "Screenshot", Enabled: disabled},
			},
		},
		{
			Label: "Tools",
			Commands: []Command{
				{ID: "tools.cheats", Label: "Cheat Manager", Enabled: disabled},
				{ID: "tools.memory", Label: "Memory Search", Enabled: disabled},
				{ID: "tools.patches", Label: "Patch Manager", Enabled: disabled},
				{ID: "tools.debugger", Label: "Debugger", Enabled: disabled},
				{ID: "tools.controller", Label: "Controller Settings", Enabled: disabled},
				{ID: "tools.audio", Label: "Audio Settings", Enabled: disabled},
				{ID: "tools.properties", Label: "Title Properties", Enabled: disabled},
				{ID: "tools.compatibility", Label: "Compatibility Report", Enabled: disabled},
				{ID: "tools.logs", Label: "Logs", Enabled: disabled},
			},
		},
		{
			Label: "Help",
			Commands: []Command{
				{ID: "help.documentation", Label: "Documentation", Enabled: disabled},
				{ID: "help.issue", Label: "Report Issue", Enabled: disabled},
				{ID: "help.about", Label: "About ARAM", Action: (*Shell).showAbout},
			},
		},
	}
}
