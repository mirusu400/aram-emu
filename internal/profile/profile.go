package profile

import "fmt"

type Carrier string

const (
	CarrierUnknown Carrier = "unknown"
	CarrierKTF     Carrier = "ktf"
	CarrierSKT     Carrier = "skt"
	CarrierLGT     Carrier = "lgt"
)

type Screen struct {
	Width       int
	Height      int
	Orientation string
}

type Profile struct {
	ID           string
	Manufacturer string
	Model        string
	Carrier      Carrier
	Screen       Screen
	Properties   map[string]string
	Quirks       map[string]bool
}

func (p Profile) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("profile ID is empty")
	}
	if p.Screen.Width <= 0 || p.Screen.Height <= 0 {
		return fmt.Errorf("profile %q has invalid screen geometry", p.ID)
	}
	return nil
}
