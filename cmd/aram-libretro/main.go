package main

/*
#include <stddef.h>
#include <stdint.h>
*/
import "C"

import (
	"context"
	"errors"
	"sync"
	"unsafe"

	aramlibretro "github.com/mirusu400/aram-emu/libretro"
)

const maxCGoBytes = uint64(1<<31 - 1)

var bridge struct {
	sync.Mutex
	core      *aramlibretro.Core
	last      aramlibretro.RunResult
	lastError string
}

func setBridgeError(err error) C.int {
	if err == nil {
		bridge.lastError = ""
		return 1
	}
	bridge.lastError = err.Error()
	return 0
}

//export aram_initialize
func aram_initialize() {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil {
		bridge.core = aramlibretro.New(aramlibretro.Config{})
	}
}

//export aram_deinitialize
func aram_deinitialize() {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core != nil {
		_ = bridge.core.Close()
	}
	bridge.core = nil
	bridge.last = aramlibretro.RunResult{}
	bridge.lastError = ""
}

//export aram_set_save_directory
func aram_set_save_directory(directory *C.char) {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil {
		bridge.core = aramlibretro.New(aramlibretro.Config{})
	}
	if directory == nil {
		bridge.core.SetSaveDirectory("")
		return
	}
	bridge.core.SetSaveDirectory(C.GoString(directory))
}

//export aram_load_game
func aram_load_game(name *C.char, data unsafe.Pointer, size C.size_t) C.int {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil {
		bridge.core = aramlibretro.New(aramlibretro.Config{})
	}
	if data == nil || uint64(size) == 0 || uint64(size) > maxCGoBytes {
		return setBridgeError(errors.New("load game: invalid content buffer"))
	}
	content := C.GoBytes(data, C.int(size))
	displayName := "content"
	if name != nil {
		displayName = C.GoString(name)
	}
	bridge.last = aramlibretro.RunResult{}
	return setBridgeError(bridge.core.LoadGame(context.Background(), displayName, content))
}

//export aram_unload_game
func aram_unload_game() C.int {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil {
		return 1
	}
	bridge.last = aramlibretro.RunResult{}
	return setBridgeError(bridge.core.Unload())
}

//export aram_reset
func aram_reset() C.int {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil {
		return setBridgeError(errors.New("reset: core is not initialized"))
	}
	bridge.last = aramlibretro.RunResult{}
	return setBridgeError(bridge.core.Reset(context.Background()))
}

//export aram_run
func aram_run(buttons C.uint16_t) C.int {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil {
		return setBridgeError(errors.New("run: core is not initialized"))
	}
	result, err := bridge.core.Run(context.Background(), uint16(buttons))
	if err != nil {
		return setBridgeError(err)
	}
	bridge.last = result
	return setBridgeError(nil)
}

//export aram_av_width
func aram_av_width() C.uint {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil {
		return 240
	}
	return C.uint(bridge.core.AVInfo().Width)
}

//export aram_av_height
func aram_av_height() C.uint {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil {
		return 320
	}
	return C.uint(bridge.core.AVInfo().Height)
}

//export aram_av_max_width
func aram_av_max_width() C.uint { return 4096 }

//export aram_av_max_height
func aram_av_max_height() C.uint { return 4096 }

//export aram_av_fps
func aram_av_fps() C.double {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil {
		return 60
	}
	return C.double(bridge.core.AVInfo().FPS)
}

//export aram_av_sample_rate
func aram_av_sample_rate() C.double { return 44100 }

//export aram_frame_width
func aram_frame_width() C.uint {
	bridge.Lock()
	defer bridge.Unlock()
	return C.uint(bridge.last.Frame.Width)
}

//export aram_frame_height
func aram_frame_height() C.uint {
	bridge.Lock()
	defer bridge.Unlock()
	return C.uint(bridge.last.Frame.Height)
}

//export aram_frame_pitch
func aram_frame_pitch() C.size_t {
	bridge.Lock()
	defer bridge.Unlock()
	return C.size_t(bridge.last.Frame.Pitch)
}

//export aram_video_pixels
func aram_video_pixels() C.size_t {
	bridge.Lock()
	defer bridge.Unlock()
	return C.size_t(len(bridge.last.Frame.Pixels))
}

//export aram_copy_video
func aram_copy_video(output *C.uint32_t, capacity C.size_t) C.int {
	bridge.Lock()
	defer bridge.Unlock()
	if output == nil || uint64(capacity) < uint64(len(bridge.last.Frame.Pixels)) {
		return 0
	}
	destination := unsafe.Slice((*uint32)(unsafe.Pointer(output)), len(bridge.last.Frame.Pixels))
	copy(destination, bridge.last.Frame.Pixels)
	return 1
}

//export aram_audio_samples
func aram_audio_samples() C.size_t {
	bridge.Lock()
	defer bridge.Unlock()
	return C.size_t(len(bridge.last.Audio.PCM16))
}

//export aram_audio_frames
func aram_audio_frames() C.size_t {
	bridge.Lock()
	defer bridge.Unlock()
	return C.size_t(bridge.last.Audio.Frames)
}

//export aram_copy_audio
func aram_copy_audio(output *C.int16_t, capacity C.size_t) C.int {
	bridge.Lock()
	defer bridge.Unlock()
	if output == nil || uint64(capacity) < uint64(len(bridge.last.Audio.PCM16)) {
		return 0
	}
	destination := unsafe.Slice((*int16)(unsafe.Pointer(output)), len(bridge.last.Audio.PCM16))
	copy(destination, bridge.last.Audio.PCM16)
	return 1
}

//export aram_serialize_size
func aram_serialize_size() C.size_t {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil {
		return C.size_t(aramlibretro.DefaultSerializeSize)
	}
	return C.size_t(bridge.core.SerializeSize())
}

//export aram_serialize
func aram_serialize(output unsafe.Pointer, size C.size_t) C.int {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil || output == nil || uint64(size) > uint64(^uint(0)>>1) {
		return setBridgeError(errors.New("serialize: invalid output buffer"))
	}
	buffer := unsafe.Slice((*byte)(output), int(size))
	return setBridgeError(bridge.core.Serialize(buffer))
}

//export aram_unserialize
func aram_unserialize(input unsafe.Pointer, size C.size_t) C.int {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.core == nil || input == nil || uint64(size) > uint64(^uint(0)>>1) {
		return setBridgeError(errors.New("unserialize: invalid input buffer"))
	}
	buffer := unsafe.Slice((*byte)(input), int(size))
	return setBridgeError(bridge.core.Unserialize(buffer))
}

//export aram_copy_last_error
func aram_copy_last_error(output *C.char, capacity C.size_t) {
	bridge.Lock()
	defer bridge.Unlock()
	if output == nil || capacity == 0 || uint64(capacity) > uint64(^uint(0)>>1) {
		return
	}
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(output)), int(capacity))
	clear(buffer)
	if len(buffer) > 1 {
		copy(buffer[:len(buffer)-1], bridge.lastError)
	}
}

func main() {}
