package eads

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

const HeaderSize = 0x30

var Magic = []byte("EADS")

type Image struct {
	RecordOffset uint32
	FormatWord   uint32
	BuildWord    uint32
	TextBase     uint32
	TextSize     uint32
	DataBase     uint32
	BSSSize      uint32
	Reserved     uint32
	Name         string
}

func (i Image) PayloadOffset() uint32 {
	return i.RecordOffset + HeaderSize
}

func (i Image) RecordEnd() uint32 {
	return i.PayloadOffset() + i.TextSize
}

type FormatError struct {
	Offset uint32
	Reason string
}

func (e *FormatError) Error() string {
	return fmt.Sprintf("EADS at 0x%x: %s", e.Offset, e.Reason)
}

func Parse(data []byte, recordOffset uint32) (Image, error) {
	fail := func(reason string) (Image, error) {
		return Image{}, &FormatError{Offset: recordOffset, Reason: reason}
	}
	if uint64(recordOffset)+HeaderSize > uint64(len(data)) {
		return fail("truncated header")
	}
	if !bytes.Equal(data[recordOffset:recordOffset+4], Magic) {
		return fail("magic mismatch")
	}

	image := Image{
		RecordOffset: recordOffset,
		FormatWord:   u32(data, recordOffset+4),
		BuildWord:    u32(data, recordOffset+8),
		TextBase:     u32(data, recordOffset+12),
		TextSize:     u32(data, recordOffset+16),
		DataBase:     u32(data, recordOffset+20),
		BSSSize:      u32(data, recordOffset+24),
		Reserved:     u32(data, recordOffset+28),
	}
	rawName := data[recordOffset+0x20 : recordOffset+0x30]
	if index := bytes.IndexByte(rawName, 0); index >= 0 {
		rawName = rawName[:index]
	}
	image.Name = string(rawName)

	payload := uint64(recordOffset) + HeaderSize
	if image.Name == "" || strings.IndexFunc(image.Name, func(r rune) bool {
		return r < 0x20 || r > 0x7e
	}) >= 0 {
		return fail("image name is not printable ASCII")
	}
	if image.TextBase&0xfff != 0 || image.DataBase&0xfff != 0 ||
		image.TextSize < 4 || image.BSSSize < 4 ||
		payload+uint64(image.TextSize) > uint64(len(data)) {
		return fail("invalid image geometry")
	}
	textEnd := uint64(image.TextBase) + uint64(image.TextSize)
	dataEnd := uint64(image.DataBase) + uint64(image.BSSSize)
	if textEnd > 1<<32 || dataEnd > 1<<32 ||
		(uint64(image.TextBase) < dataEnd && uint64(image.DataBase) < textEnd) {
		return fail("guest ranges overflow or overlap")
	}
	if !bytes.Equal(data[payload:payload+2], []byte{0x00, 0xb5}) {
		return fail("text does not start with a Thumb veneer")
	}
	return image, nil
}

func Inspect(data []byte) []Image {
	var images []Image
	for start := 0; start < len(data); {
		index := bytes.Index(data[start:], Magic)
		if index < 0 {
			break
		}
		offset := start + index
		image, err := Parse(data, uint32(offset))
		if err == nil {
			images = append(images, image)
		}
		start = offset + 1
	}
	return images
}

func u32(data []byte, offset uint32) uint32 {
	return binary.LittleEndian.Uint32(data[offset : offset+4])
}
