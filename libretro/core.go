package libretro

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"

	authd "github.com/mirusu400/aram-authd"
	"github.com/mirusu400/aram-core/application"
	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-core/loader"
	"github.com/mirusu400/aram-emu/carrier"
)

const (
	DefaultSerializeSize = 64 << 20
	stateHeaderSize      = 48
	stateVersion         = uint32(1)
	defaultSampleRate    = 44100
)

var stateMagic = [8]byte{'A', 'R', 'A', 'M', 'L', 'R', '1', 0}

type Config struct {
	Factory       aramcore.Factory
	SaveDirectory string
	SerializeSize int
}

type AVInfo struct {
	Width      int
	Height     int
	MaxWidth   int
	MaxHeight  int
	FPS        float64
	SampleRate float64
}

type Frame struct {
	Pixels []uint32
	Width  int
	Height int
	Pitch  int
}

type Audio struct {
	PCM16      []int16
	Frames     int
	SampleRate int
}

type RunResult struct {
	Frame Frame
	Audio Audio
}

type Core struct {
	factory       aramcore.Factory
	machine       aramcore.Machine
	saveDirectory string
	serializeSize int
	sourceSHA256  string
	started       bool
	controls      map[string]bool
	framePixels   []uint32
}

func New(config Config) *Core {
	factory := config.Factory
	if factory == nil {
		applicationFactory := application.NewFactory()
		applicationFactory.FrameRunBudget = application.DefaultHandsetRunBudget
		applicationFactory.KTFRunBudget = application.DefaultKTFHandsetRunBudget
		applicationFactory.OutputSampleRate = defaultSampleRate
		applicationFactory.OutputChannels = 2
		applicationFactory.RaptorNet = carrier.AuthdRaptorNet(authd.Grant{})
		factory = applicationFactory
	}
	serializeSize := config.SerializeSize
	if serializeSize < stateHeaderSize {
		serializeSize = DefaultSerializeSize
	}
	return &Core{
		factory:       factory,
		saveDirectory: config.SaveDirectory,
		serializeSize: serializeSize,
		controls:      make(map[string]bool),
	}
}

func (c *Core) Loaded() bool { return c.machine != nil }

func (c *Core) SetSaveDirectory(directory string) {
	c.saveDirectory = directory
}

func (c *Core) LoadGame(ctx context.Context, name string, data []byte) error {
	if len(data) == 0 {
		return errors.New("load game: content is empty")
	}
	if err := c.Unload(); err != nil {
		return fmt.Errorf("replace loaded game: %w", err)
	}
	if name == "" {
		name = "content"
	}
	report, err := loader.InspectBytes(name, data)
	if err != nil {
		return fmt.Errorf("inspect content: %w", err)
	}
	if report.Kind == loader.KindUnknown || report.Kind == loader.KindFirmware || report.Kind == loader.KindFont {
		return fmt.Errorf("unsupported application content %q", report.Kind)
	}
	source := aramcore.Source{
		Name:     filepath.Base(name),
		Path:     name,
		Format:   string(report.Kind),
		SHA256:   report.SHA256,
		ReaderAt: bytes.NewReader(data),
		Size:     int64(len(data)),
	}
	machine, err := c.factory.Create(ctx, source)
	if err != nil {
		return fmt.Errorf("create machine: %w", err)
	}
	if machine == nil {
		return errors.New("create machine: factory returned nil")
	}
	c.machine = machine
	c.sourceSHA256 = report.SHA256
	c.started = false
	clear(c.controls)
	if err := c.loadPersistentData(); err != nil {
		_ = machine.Close()
		c.machine = nil
		c.sourceSHA256 = ""
		return err
	}
	return nil
}

func (c *Core) Run(ctx context.Context, buttons uint16) (RunResult, error) {
	if c.machine == nil {
		return RunResult{}, errors.New("run: no content is loaded")
	}
	if err := c.queueControls(mappedControls(buttons)); err != nil {
		return RunResult{}, err
	}
	var err error
	if !c.started {
		err = c.machine.Start(ctx)
		if err == nil {
			c.started = machineCanContinue(c.machine.State())
		}
	} else if machineCanContinue(c.machine.State()) {
		err = c.machine.StepFrame(ctx)
	}
	if err != nil {
		return RunResult{}, fmt.Errorf("advance machine: %w", err)
	}
	if c.machine.State() == aramcore.StateStopped {
		c.started = false
		_ = c.persistSaveData()
	}
	frame, err := c.currentFrame()
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Frame: frame, Audio: c.drainAudio()}, nil
}

func (c *Core) Reset(ctx context.Context) error {
	if c.machine == nil {
		return errors.New("reset: no content is loaded")
	}
	if err := c.releaseControls(); err != nil {
		return err
	}
	if err := c.machine.Reset(ctx); err != nil {
		return fmt.Errorf("reset machine: %w", err)
	}
	c.started = false
	return nil
}

func (c *Core) AVInfo() AVInfo {
	info := AVInfo{Width: 240, Height: 320, MaxWidth: 4096, MaxHeight: 4096, FPS: 60, SampleRate: defaultSampleRate}
	if c.machine == nil {
		return info
	}
	if frame := c.machine.Framebuffer(); frame != nil {
		bounds := frame.Bounds()
		if bounds.Dx() > 0 && bounds.Dy() > 0 {
			info.Width = bounds.Dx()
			info.Height = bounds.Dy()
		}
	}
	if provider, ok := unwrapMachine(c.machine).(interface{ FrameQuantum() time.Duration }); ok {
		quantum := provider.FrameQuantum()
		if quantum > 0 {
			info.FPS = float64(time.Second) / float64(quantum)
		}
	}
	return info
}

func (c *Core) SerializeSize() int { return c.serializeSize }

func (c *Core) Serialize(output []byte) error {
	if c.machine == nil {
		return errors.New("serialize: no content is loaded")
	}
	if len(output) < c.serializeSize {
		return fmt.Errorf("serialize: buffer has %d bytes, need %d", len(output), c.serializeSize)
	}
	var state bytes.Buffer
	if err := c.machine.SaveState(&state); err != nil {
		return fmt.Errorf("serialize machine: %w", err)
	}
	if state.Len() > c.serializeSize-stateHeaderSize {
		return fmt.Errorf("serialize: state size %d exceeds capacity %d", state.Len(), c.serializeSize-stateHeaderSize)
	}
	digest, err := hex.DecodeString(c.sourceSHA256)
	if err != nil || len(digest) != sha256.Size {
		return errors.New("serialize: loaded content identity is unavailable")
	}
	clear(output[:c.serializeSize])
	copy(output[:8], stateMagic[:])
	binary.LittleEndian.PutUint32(output[8:12], stateVersion)
	binary.LittleEndian.PutUint32(output[12:16], uint32(state.Len()))
	copy(output[16:48], digest)
	copy(output[stateHeaderSize:], state.Bytes())
	return nil
}

func (c *Core) Unserialize(input []byte) error {
	if c.machine == nil {
		return errors.New("unserialize: no content is loaded")
	}
	if len(input) < stateHeaderSize || !bytes.Equal(input[:8], stateMagic[:]) {
		return errors.New("unserialize: invalid state header")
	}
	if binary.LittleEndian.Uint32(input[8:12]) != stateVersion {
		return errors.New("unserialize: unsupported state version")
	}
	length := int(binary.LittleEndian.Uint32(input[12:16]))
	if length < 0 || length > len(input)-stateHeaderSize {
		return errors.New("unserialize: invalid state length")
	}
	digest, err := hex.DecodeString(c.sourceSHA256)
	if err != nil || !bytes.Equal(input[16:48], digest) {
		return errors.New("unserialize: state belongs to different content")
	}
	if err := c.releaseControls(); err != nil {
		return err
	}
	if err := c.machine.LoadState(bytes.NewReader(input[stateHeaderSize : stateHeaderSize+length])); err != nil {
		return fmt.Errorf("unserialize machine: %w", err)
	}
	c.started = machineCanContinue(c.machine.State())
	return nil
}

func (c *Core) Unload() error {
	if c.machine == nil {
		return nil
	}
	var errs []error
	if err := c.releaseControls(); err != nil {
		errs = append(errs, err)
	}
	if c.machine.State() == aramcore.StateRunning {
		if err := c.machine.Pause(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := c.persistSaveData(); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, c.machine.Close())
	c.machine = nil
	c.sourceSHA256 = ""
	c.started = false
	c.framePixels = nil
	clear(c.controls)
	return errors.Join(errs...)
}

func (c *Core) Close() error { return c.Unload() }

func (c *Core) currentFrame() (Frame, error) {
	var source image.Image
	if presenter, ok := unwrapMachine(c.machine).(interface {
		VideoPresentation() aramcore.VideoPresentation
	}); ok {
		source = presenter.VideoPresentation().Image
	} else {
		source = c.machine.Framebuffer()
	}
	if source == nil || source.Bounds().Dx() <= 0 || source.Bounds().Dy() <= 0 {
		return Frame{}, errors.New("present frame: machine returned no image")
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	needed := width * height
	if cap(c.framePixels) < needed {
		c.framePixels = make([]uint32, needed)
	} else {
		c.framePixels = c.framePixels[:needed]
	}
	if rgba, ok := source.(*image.RGBA); ok {
		for y := 0; y < height; y++ {
			row := rgba.Pix[(y+bounds.Min.Y-rgba.Rect.Min.Y)*rgba.Stride:]
			for x := 0; x < width; x++ {
				offset := (x + bounds.Min.X - rgba.Rect.Min.X) * 4
				c.framePixels[y*width+x] = uint32(row[offset])<<16 | uint32(row[offset+1])<<8 | uint32(row[offset+2])
			}
		}
	} else {
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				r, g, b, _ := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				c.framePixels[y*width+x] = uint32(r>>8)<<16 | uint32(g>>8)<<8 | uint32(b>>8)
			}
		}
	}
	return Frame{Pixels: c.framePixels, Width: width, Height: height, Pitch: width * 4}, nil
}

func (c *Core) drainAudio() Audio {
	var pcm []int16
	sampleRate := defaultSampleRate
	for range 128 {
		chunk := c.machine.DrainAudio()
		if len(chunk.PCM16) == 0 {
			break
		}
		if chunk.SampleRate > 0 {
			sampleRate = chunk.SampleRate
		}
		pcm = append(pcm, stereoPCM(chunk.PCM16, chunk.Channels)...)
	}
	return Audio{PCM16: pcm, Frames: len(pcm) / 2, SampleRate: sampleRate}
}

func (c *Core) queueControls(next map[string]bool) error {
	for _, control := range handsetControls {
		pressed := next[control]
		if c.controls[control] == pressed {
			continue
		}
		if err := c.machine.QueueInput(aramcore.InputEvent{Control: control, Pressed: pressed}); err != nil {
			return fmt.Errorf("queue input %q: %w", control, err)
		}
		if pressed {
			c.controls[control] = true
		} else {
			delete(c.controls, control)
		}
	}
	return nil
}

func (c *Core) releaseControls() error {
	if c.machine == nil || len(c.controls) == 0 {
		clear(c.controls)
		return nil
	}
	return c.queueControls(map[string]bool{})
}

func (c *Core) savePath() string {
	if c.saveDirectory == "" || c.sourceSHA256 == "" {
		return ""
	}
	return filepath.Join(c.saveDirectory, "aram", c.sourceSHA256+".aramsave")
}

func (c *Core) loadPersistentData() error {
	path := c.savePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read persistent save: %w", err)
	}
	importer, ok := unwrapMachine(c.machine).(interface{ ImportSaveData([]byte) error })
	if !ok {
		return nil
	}
	if err := importer.ImportSaveData(data); err != nil {
		return fmt.Errorf("import persistent save: %w", err)
	}
	return nil
}

func (c *Core) persistSaveData() error {
	path := c.savePath()
	if path == "" || c.machine == nil {
		return nil
	}
	exporter, ok := unwrapMachine(c.machine).(interface{ ExportSaveData() ([]byte, error) })
	if !ok {
		return nil
	}
	data, err := exporter.ExportSaveData()
	if err != nil {
		return fmt.Errorf("export persistent save: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create save directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "aram-save-*.tmp")
	if err != nil {
		return fmt.Errorf("create persistent save: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write persistent save: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync persistent save: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close persistent save: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(path)
		if retryErr := os.Rename(temporaryPath, path); retryErr != nil {
			return fmt.Errorf("replace persistent save after %v: %w", err, retryErr)
		}
	}
	committed = true
	return nil
}

func machineCanContinue(state aramcore.State) bool {
	return state == aramcore.StateReady || state == aramcore.StateRunning || state == aramcore.StatePaused
}

func unwrapMachine(machine aramcore.Machine) aramcore.Machine {
	for {
		wrapper, ok := machine.(interface{ UnwrapMachine() aramcore.Machine })
		if !ok {
			return machine
		}
		next := wrapper.UnwrapMachine()
		if next == nil || next == machine {
			return machine
		}
		machine = next
	}
}
