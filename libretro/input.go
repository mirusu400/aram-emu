package libretro

const (
	JoypadB = iota
	JoypadY
	JoypadSelect
	JoypadStart
	JoypadUp
	JoypadDown
	JoypadLeft
	JoypadRight
	JoypadA
	JoypadX
	JoypadL
	JoypadR
	JoypadL2
	JoypadR2
	JoypadL3
	JoypadR3
)

var handsetControls = []string{
	"up", "down", "left", "right", "ok", "back", "soft-left", "soft-right",
	"menu", "send", "end", "star", "hash",
	"num0", "num1", "num2", "num3", "num4", "num5", "num6", "num7", "num8", "num9",
}

func mappedControls(buttons uint16) map[string]bool {
	pressed := func(id int) bool { return buttons&(1<<id) != 0 }
	controls := make(map[string]bool)
	if pressed(JoypadSelect) {
		mapPressed(controls, pressed(JoypadUp), "num2")
		mapPressed(controls, pressed(JoypadDown), "num8")
		mapPressed(controls, pressed(JoypadLeft), "num4")
		mapPressed(controls, pressed(JoypadRight), "num6")
		mapPressed(controls, pressed(JoypadA), "num5")
		mapPressed(controls, pressed(JoypadB), "num0")
		mapPressed(controls, pressed(JoypadX), "num1")
		mapPressed(controls, pressed(JoypadY), "num3")
		mapPressed(controls, pressed(JoypadL), "num7")
		mapPressed(controls, pressed(JoypadR), "num9")
		mapPressed(controls, pressed(JoypadL2), "star")
		mapPressed(controls, pressed(JoypadR2), "hash")
		return controls
	}
	mapPressed(controls, pressed(JoypadUp), "up")
	mapPressed(controls, pressed(JoypadDown), "down")
	mapPressed(controls, pressed(JoypadLeft), "left")
	mapPressed(controls, pressed(JoypadRight), "right")
	mapPressed(controls, pressed(JoypadA), "ok")
	mapPressed(controls, pressed(JoypadB), "back")
	mapPressed(controls, pressed(JoypadX), "soft-left")
	mapPressed(controls, pressed(JoypadY), "soft-right")
	mapPressed(controls, pressed(JoypadStart), "menu")
	mapPressed(controls, pressed(JoypadL), "star")
	mapPressed(controls, pressed(JoypadR), "hash")
	mapPressed(controls, pressed(JoypadL2), "send")
	mapPressed(controls, pressed(JoypadR2), "end")
	return controls
}

func mapPressed(controls map[string]bool, pressed bool, control string) {
	if pressed {
		controls[control] = true
	}
}
