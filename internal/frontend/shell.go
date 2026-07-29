package frontend

import (
	"errors"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/ncruces/zenity"

	"github.com/mirusu400/aram-emu/internal/loader"
)

const (
	logicalWidth   = 960
	logicalHeight  = 720
	menuHeight     = 28
	statusHeight   = 24
	menuItemHeight = 26
	dropdownWidth  = 290
)

var (
	backgroundColor = color.RGBA{R: 0x18, G: 0x1b, B: 0x20, A: 0xff}
	menuColor       = color.RGBA{R: 0x27, G: 0x2b, B: 0x33, A: 0xff}
	menuActiveColor = color.RGBA{R: 0x3a, G: 0x61, B: 0x8f, A: 0xff}
	panelColor      = color.RGBA{R: 0x20, G: 0x24, B: 0x2b, A: 0xff}
	borderColor     = color.RGBA{R: 0x51, G: 0x58, B: 0x65, A: 0xff}
	disabledColor   = color.RGBA{R: 0x78, G: 0x7d, B: 0x86, A: 0xff}
)

type operation uint8

const (
	operationOpen operation = iota
	operationFirmware
	operationRecent
)

type dialogResult struct {
	operation operation
	path      string
	err       error
}

type inspectResult struct {
	report loader.Report
	err    error
}

type Shell struct {
	menus          []Menu
	settings       Settings
	activeMenu     int
	status         string
	report         *loader.Report
	firmwarePath   string
	dialogOpen     bool
	inspecting     bool
	quitting       bool
	dialogResults  chan dialogResult
	inspectResults chan inspectResult
}

func NewShell(initialPath string) *Shell {
	shell := &Shell{
		settings:       loadSettings(),
		activeMenu:     -1,
		status:         "Ready — use File > Open File... to inspect a title",
		dialogResults:  make(chan dialogResult, 2),
		inspectResults: make(chan inspectResult, 2),
	}
	shell.menus = defaultMenus()
	if initialPath != "" {
		shell.inspectPath(initialPath)
	}
	return shell
}

func Run(initialPath string) error {
	ebiten.SetWindowTitle("ARAM — Archived Runtime for ARM Mobiles")
	ebiten.SetWindowSize(logicalWidth, logicalHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	return ebiten.RunGame(NewShell(initialPath))
}

func (s *Shell) Update() error {
	if s.quitting {
		return ebiten.Termination
	}
	s.consumeResults()
	s.handleShortcuts()
	s.handleMouse()
	return nil
}

func (s *Shell) Draw(screen *ebiten.Image) {
	screen.Fill(backgroundColor)
	s.drawMenuBar(screen)
	s.drawWorkspace(screen)
	s.drawStatusBar(screen)
	if s.activeMenu >= 0 {
		s.drawDropdown(screen, s.activeMenu)
	}
}

func (s *Shell) Layout(int, int) (int, int) {
	return logicalWidth, logicalHeight
}

func (s *Shell) handleShortcuts() {
	control := ebiten.IsKeyPressed(ebiten.KeyControl) ||
		ebiten.IsKeyPressed(ebiten.KeyControlLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyControlRight)
	if control && inpututil.IsKeyJustPressed(ebiten.KeyO) {
		s.chooseFile()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		s.toggleFullscreen()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		s.activeMenu = -1
	}
}

func (s *Shell) handleMouse() {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	x, y := ebiten.CursorPosition()
	if y < menuHeight {
		offset := 0
		for index, width := range menuWidths(s.menus) {
			if x >= offset && x < offset+width {
				if s.activeMenu == index {
					s.activeMenu = -1
				} else {
					s.activeMenu = index
				}
				return
			}
			offset += width
		}
		s.activeMenu = -1
		return
	}
	if s.activeMenu < 0 {
		return
	}
	startX := menuStartX(s.menus, s.activeMenu)
	if x < startX || x >= startX+dropdownWidth || y < menuHeight {
		s.activeMenu = -1
		return
	}
	index := (y - menuHeight) / menuItemHeight
	commands := s.menus[s.activeMenu].Commands
	if index < 0 || index >= len(commands) {
		s.activeMenu = -1
		return
	}
	command := commands[index]
	s.activeMenu = -1
	if command.IsEnabled(s) && command.Action != nil {
		command.Action(s)
	}
}

func (s *Shell) consumeResults() {
	for {
		select {
		case result := <-s.dialogResults:
			s.dialogOpen = false
			if result.err != nil {
				if !errors.Is(result.err, zenity.ErrCanceled) {
					s.status = "File dialog: " + result.err.Error()
				}
				continue
			}
			switch result.operation {
			case operationOpen, operationRecent:
				s.inspectPath(result.path)
			case operationFirmware:
				s.firmwarePath = result.path
				s.settings.LastFirmwarePath = result.path
				_ = s.settings.save()
				s.status = "Firmware directory selected: " + result.path
			}
		case result := <-s.inspectResults:
			s.inspecting = false
			if result.err != nil {
				s.status = "Open failed: " + result.err.Error()
				continue
			}
			s.report = &result.report
			s.settings.addRecent(result.report.Path)
			_ = s.settings.save()
			ebiten.SetWindowTitle("ARAM — " + filepath.Base(result.report.Path))
			s.status = fmt.Sprintf(
				"Recognized %s · %d bytes · backend not connected",
				result.report.Kind,
				result.report.Size,
			)
		default:
			return
		}
	}
}

func (s *Shell) chooseFile() {
	if s.dialogOpen || s.inspecting {
		return
	}
	s.dialogOpen = true
	s.status = "Waiting for file selection..."
	go func() {
		path, err := zenity.SelectFile(
			zenity.Title("Open WIPI package or firmware"),
			zenity.FileFilters{
				{Name: "Supported inputs", Patterns: []string{"*.dat", "*.wbin", "*.wbt", "*.bin", "*.rom", "*.img", "*.mbn", "*.jar"}},
				{Name: "WIPI packages", Patterns: []string{"*.dat", "*.jar"}},
				{Name: "Firmware images", Patterns: []string{"*.wbin", "*.wbt", "*.bin", "*.rom", "*.img", "*.mbn"}},
				{Name: "All files", Patterns: []string{"*"}},
			},
		)
		s.dialogResults <- dialogResult{operation: operationOpen, path: path, err: err}
	}()
}

func (s *Shell) chooseFirmwareDirectory() {
	if s.dialogOpen || s.inspecting {
		return
	}
	s.dialogOpen = true
	s.status = "Waiting for firmware directory selection..."
	go func() {
		options := []zenity.Option{
			zenity.Title("Select firmware directory"),
			zenity.Directory(),
		}
		if s.settings.LastFirmwarePath != "" {
			options = append(options, zenity.Filename(s.settings.LastFirmwarePath))
		}
		path, err := zenity.SelectFile(options...)
		s.dialogResults <- dialogResult{operation: operationFirmware, path: path, err: err}
	}()
}

func (s *Shell) chooseRecent() {
	if s.dialogOpen || len(s.settings.RecentFiles) == 0 {
		return
	}
	s.dialogOpen = true
	recent := append([]string(nil), s.settings.RecentFiles...)
	go func() {
		path, err := zenity.List(
			"Choose a recent input",
			recent,
			zenity.Title("ARAM recent files"),
			zenity.Width(840),
			zenity.Height(420),
		)
		s.dialogResults <- dialogResult{operation: operationRecent, path: path, err: err}
	}()
}

func (s *Shell) inspectPath(path string) {
	if s.inspecting {
		return
	}
	s.inspecting = true
	s.status = "Inspecting " + path + "..."
	go func() {
		report, err := loader.InspectFile(path)
		s.inspectResults <- inspectResult{report: report, err: err}
	}()
}

func (s *Shell) closeInput() {
	s.report = nil
	ebiten.SetWindowTitle("ARAM — Archived Runtime for ARM Mobiles")
	s.status = "Title closed"
}

func (s *Shell) toggleFullscreen() {
	ebiten.SetFullscreen(!ebiten.IsFullscreen())
	if ebiten.IsFullscreen() {
		s.status = "Fullscreen enabled"
	} else {
		s.status = "Fullscreen disabled"
	}
}

func (s *Shell) toggleIntegerScaling() {
	s.settings.IntegerScaling = !s.settings.IntegerScaling
	_ = s.settings.save()
	s.status = fmt.Sprintf("Integer scaling: %t", s.settings.IntegerScaling)
}

func (s *Shell) toggleAspectRatio() {
	s.settings.PreserveAspect = !s.settings.PreserveAspect
	_ = s.settings.save()
	s.status = fmt.Sprintf("Preserve aspect ratio: %t", s.settings.PreserveAspect)
}

func (s *Shell) showAbout() {
	go func() {
		_ = zenity.Info(
			"ARAM\nArchived Runtime for ARM Mobiles\n\n"+
				"General-purpose Korean feature-phone emulator.\n"+
				"Execution backends are under construction.",
			zenity.Title("About ARAM"),
		)
	}()
}

func (s *Shell) drawMenuBar(screen *ebiten.Image) {
	ebitenutil.DrawRect(screen, 0, 0, logicalWidth, menuHeight, menuColor)
	offset := 0
	for index, menu := range s.menus {
		width := menuWidths(s.menus)[index]
		if s.activeMenu == index {
			ebitenutil.DrawRect(screen, float64(offset), 0, float64(width), menuHeight, menuActiveColor)
		}
		ebitenutil.DebugPrintAt(screen, menu.Label, offset+12, 8)
		offset += width
	}
}

func (s *Shell) drawDropdown(screen *ebiten.Image, menuIndex int) {
	commands := s.menus[menuIndex].Commands
	x := menuStartX(s.menus, menuIndex)
	height := len(commands) * menuItemHeight
	ebitenutil.DrawRect(screen, float64(x), menuHeight, dropdownWidth, float64(height), menuColor)
	ebitenutil.DrawRect(screen, float64(x), menuHeight, dropdownWidth, 1, borderColor)
	for index, command := range commands {
		y := menuHeight + index*menuItemHeight
		if !command.IsEnabled(s) {
			ebitenutil.DrawRect(screen, float64(x), float64(y), dropdownWidth, menuItemHeight, color.RGBA{R: 0x24, G: 0x27, B: 0x2d, A: 0xff})
			drawMutedText(screen, command.Label, x+12, y+8)
			if command.Shortcut != "" {
				drawMutedText(screen, command.Shortcut, x+210, y+8)
			}
			continue
		}
		ebitenutil.DebugPrintAt(screen, command.Label, x+12, y+8)
		if command.Shortcut != "" {
			ebitenutil.DebugPrintAt(screen, command.Shortcut, x+210, y+8)
		}
	}
	ebitenutil.DrawRect(screen, float64(x), float64(menuHeight+height-1), dropdownWidth, 1, borderColor)
}

func (s *Shell) drawWorkspace(screen *ebiten.Image) {
	contentTop := menuHeight + 20
	contentBottom := logicalHeight - statusHeight - 20
	viewportAreaWidth := 650
	ebitenutil.DrawRect(
		screen,
		20,
		float64(contentTop),
		float64(viewportAreaWidth),
		float64(contentBottom-contentTop),
		panelColor,
	)

	scale := 1
	if s.settings.IntegerScaling {
		scale = 2
	}
	phoneWidth, phoneHeight := 240*scale, 320*scale
	if phoneHeight > contentBottom-contentTop-40 {
		scale = 1
		phoneWidth, phoneHeight = 240, 320
	}
	phoneX := 20 + (viewportAreaWidth-phoneWidth)/2
	phoneY := contentTop + (contentBottom-contentTop-phoneHeight)/2
	ebitenutil.DrawRect(screen, float64(phoneX-5), float64(phoneY-5), float64(phoneWidth+10), float64(phoneHeight+10), borderColor)
	ebitenutil.DrawRect(screen, float64(phoneX), float64(phoneY), float64(phoneWidth), float64(phoneHeight), color.Black)

	if s.report == nil {
		label := "No title loaded\n\nFile > Open File...\nCtrl+O"
		ebitenutil.DebugPrintAt(screen, label, phoneX+28, phoneY+phoneHeight/2-24)
	} else {
		ebitenutil.DebugPrintAt(screen, "Input recognized\nExecution backend pending", phoneX+18, phoneY+phoneHeight/2-12)
	}

	panelX := 690
	ebitenutil.DrawRect(screen, float64(panelX), float64(contentTop), 250, float64(contentBottom-contentTop), panelColor)
	ebitenutil.DebugPrintAt(screen, "ARAM", panelX+16, contentTop+18)
	drawMutedText(screen, "Archived Runtime for ARM Mobiles", panelX+16, contentTop+38)

	lines := []string{
		"",
		"Mode: inspection",
		fmt.Sprintf("Integer scale: %t", s.settings.IntegerScaling),
		fmt.Sprintf("Aspect lock: %t", s.settings.PreserveAspect),
	}
	if s.firmwarePath != "" {
		lines = append(lines, "", "Firmware directory:", shorten(s.firmwarePath, 30))
	}
	if s.report != nil {
		lines = append(lines,
			"",
			"Selected input:",
			shorten(filepath.Base(s.report.Path), 30),
			"Format: "+string(s.report.Kind),
			fmt.Sprintf("Size: %d bytes", s.report.Size),
			"SHA-256:",
			shorten(s.report.SHA256, 30),
			fmt.Sprintf("Markers: %d", len(s.report.Markers)),
		)
	}
	ebitenutil.DebugPrintAt(screen, strings.Join(lines, "\n"), panelX+16, contentTop+58)
}

func (s *Shell) drawStatusBar(screen *ebiten.Image) {
	y := logicalHeight - statusHeight
	ebitenutil.DrawRect(screen, 0, float64(y), logicalWidth, statusHeight, menuColor)
	ebitenutil.DebugPrintAt(screen, shorten(s.status, 142), 10, y+7)
}

func menuWidths(menus []Menu) []int {
	widths := make([]int, len(menus))
	for index, menu := range menus {
		width := len(menu.Label)*8 + 28
		if width < 68 {
			width = 68
		}
		widths[index] = width
	}
	return widths
}

func menuStartX(menus []Menu, index int) int {
	offset := 0
	widths := menuWidths(menus)
	for current := 0; current < index; current++ {
		offset += widths[current]
	}
	return offset
}

func drawMutedText(screen *ebiten.Image, text string, x, y int) {
	// DebugPrint has no color parameter. A dim backing strip still makes the
	// disabled state unambiguous while keeping the built-in font dependency-free.
	ebitenutil.DrawRect(screen, float64(x-2), float64(y-1), float64(len(text)*6+4), 13, disabledColor)
	ebitenutil.DebugPrintAt(screen, text, x, y)
}

func shorten(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
