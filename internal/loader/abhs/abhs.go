package abhs

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	Version         = 0x1000
	RelocationMagic = 0xf0123456
	maxDescriptors  = 16
)

var Magic = []byte("ABHS")

type Descriptor struct {
	Type       uint32
	Size       uint32
	FileOffset uint32
}

type Module struct {
	RecordOffset      uint32
	RecordSize        uint32
	Version           uint32
	Descriptors       []Descriptor
	Metadata          Descriptor
	Code              Descriptor
	Relocations       Descriptor
	RelocationOffsets []uint32
	EntryOffset       uint32
	MetadataMode      uint32
}

func (m Module) RecordEnd() uint32 {
	return m.RecordOffset + m.RecordSize
}

type FormatError struct {
	Offset uint32
	Reason string
}

func (e *FormatError) Error() string {
	return fmt.Sprintf("ABHS at 0x%x: %s", e.Offset, e.Reason)
}

func Parse(data []byte, recordOffset uint32) (Module, error) {
	fail := func(reason string) (Module, error) {
		return Module{}, &FormatError{Offset: recordOffset, Reason: reason}
	}
	if !validRange(uint64(recordOffset), 16, uint64(len(data))) {
		return fail("truncated header")
	}
	if !bytes.Equal(data[recordOffset:recordOffset+4], Magic) {
		return fail("magic mismatch")
	}

	version := u32(data, recordOffset+4)
	tableRelative := u32(data, recordOffset+8)
	count := u32(data, recordOffset+12)
	if version != Version {
		return fail(fmt.Sprintf("unsupported version 0x%08x", version))
	}
	if tableRelative < 16 || count == 0 || count > maxDescriptors {
		return fail("invalid descriptor table geometry")
	}

	tableOffset := uint64(recordOffset) + uint64(tableRelative)
	if !validRange(tableOffset, uint64(count)*12, uint64(len(data))) {
		return fail("descriptor table exceeds input")
	}

	module := Module{
		RecordOffset: recordOffset,
		Version:      version,
		Descriptors:  make([]Descriptor, 0, count),
	}
	found := map[uint32]bool{}
	recordEnd := tableOffset + uint64(count)*12
	for index := uint32(0); index < count; index++ {
		position := uint32(tableOffset) + index*12
		descriptor := Descriptor{
			Type:       u32(data, position),
			Size:       u32(data, position+4),
			FileOffset: u32(data, position+8),
		}
		if descriptor.Type > 2 || found[descriptor.Type] {
			return fail("invalid or duplicate descriptor type")
		}
		absolute := uint64(recordOffset) + uint64(descriptor.FileOffset)
		if !validRange(absolute, uint64(descriptor.Size), uint64(len(data))) {
			return fail(fmt.Sprintf("section %d exceeds input", descriptor.Type))
		}
		if end := absolute + uint64(descriptor.Size); end > recordEnd {
			recordEnd = end
		}
		found[descriptor.Type] = true
		module.Descriptors = append(module.Descriptors, descriptor)
		switch descriptor.Type {
		case 0:
			module.Metadata = descriptor
		case 1:
			module.Code = descriptor
		case 2:
			module.Relocations = descriptor
		}
	}
	if len(found) != 3 {
		return fail("metadata, code, and relocation sections are required")
	}
	if module.Metadata.Size < 60 || module.Code.Size < 4 || module.Relocations.Size < 20 {
		return fail("section is smaller than its fixed header")
	}

	relocationStart := recordOffset + module.Relocations.FileOffset
	kind := u32(data, relocationStart)
	relocationCount := u32(data, relocationStart+4)
	encodedSize := u32(data, relocationStart+8)
	magic := u32(data, relocationStart+12)
	expectedSize := uint64(16) + uint64(relocationCount)*4 + 4
	if kind != 1 || encodedSize != module.Relocations.Size ||
		magic != RelocationMagic || expectedSize != uint64(module.Relocations.Size) {
		return fail("invalid relocation header")
	}
	if u32(data, relocationStart+16+relocationCount*4) != 0xffffffff {
		return fail("missing relocation terminator")
	}
	module.RelocationOffsets = make([]uint32, relocationCount)
	for index := uint32(0); index < relocationCount; index++ {
		value := u32(data, relocationStart+16+index*4)
		if value&3 != 0 || value > module.Code.Size-4 {
			return fail(fmt.Sprintf("relocation 0x%x is outside aligned code", value))
		}
		module.RelocationOffsets[index] = value
	}

	metadataStart := recordOffset + module.Metadata.FileOffset
	module.MetadataMode = u32(data, metadataStart+52)
	module.EntryOffset = u32(data, metadataStart+56)
	if module.EntryOffset&^uint32(1) >= module.Code.Size {
		return fail("entry offset exceeds code")
	}
	module.RecordSize = uint32(recordEnd - uint64(recordOffset))
	return module, nil
}

func Inspect(data []byte) []Module {
	var modules []Module
	for start := 0; start < len(data); {
		index := bytes.Index(data[start:], Magic)
		if index < 0 {
			break
		}
		offset := start + index
		module, err := Parse(data, uint32(offset))
		if err == nil {
			modules = append(modules, module)
			start = int(module.RecordEnd())
			continue
		}
		start = offset + 1
	}
	return modules
}

func validRange(offset, size, total uint64) bool {
	return offset <= total && size <= total-offset
}

func u32(data []byte, offset uint32) uint32 {
	return binary.LittleEndian.Uint32(data[offset : offset+4])
}
