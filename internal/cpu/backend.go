package cpu

import (
	"context"
	"errors"
)

var (
	ErrInvalidAddress = errors.New("invalid guest address")
	ErrStopped        = errors.New("CPU execution stopped")
)

type Architecture string

const (
	ARMv4T  Architecture = "armv4t"
	ARMv5TE Architecture = "armv5te"
)

type Mode uint8

const (
	ModeARM Mode = iota
	ModeThumb
)

type StopReason uint8

const (
	StopRequested StopReason = iota
	StopBreakpoint
	StopFault
	StopBudget
)

type Result struct {
	Reason       StopReason
	Instructions uint64
	PC           uint32
	Err          error
}

// Backend is intentionally smaller than Unicorn's API. Emulator services,
// profiles, and frontends must not depend on one CPU implementation.
type Backend interface {
	Architecture() Architecture
	Map(address, size uint32, permissions uint8) error
	ReadMemory(address uint32, destination []byte) error
	WriteMemory(address uint32, source []byte) error
	ReadRegister(id uint32) (uint32, error)
	WriteRegister(id, value uint32) error
	Run(context.Context, uint32, Mode, uint64) Result
	Stop() error
	SaveContext() ([]byte, error)
	RestoreContext([]byte) error
	Close() error
}
