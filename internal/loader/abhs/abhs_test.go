package abhs

import (
	"encoding/binary"
	"testing"
)

func TestParseSyntheticModule(t *testing.T) {
	data := syntheticModule()
	module, err := Parse(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if module.Code.Size != 8 {
		t.Fatalf("code size = %d, want 8", module.Code.Size)
	}
	if len(module.RelocationOffsets) != 1 || module.RelocationOffsets[0] != 0 {
		t.Fatalf("relocations = %#v", module.RelocationOffsets)
	}
	if module.EntryOffset != 1 {
		t.Fatalf("entry offset = %#x, want 1", module.EntryOffset)
	}
}

func TestParseRejectsBadRelocation(t *testing.T) {
	data := syntheticModule()
	binary.LittleEndian.PutUint32(data[0x94:0x98], 2)
	if _, err := Parse(data, 0); err == nil {
		t.Fatal("Parse accepted an unaligned relocation")
	}
}

func syntheticModule() []byte {
	data := make([]byte, 0xac)
	copy(data, Magic)
	binary.LittleEndian.PutUint32(data[4:8], Version)
	binary.LittleEndian.PutUint32(data[8:12], 0x10)
	binary.LittleEndian.PutUint32(data[12:16], 3)

	putDescriptor := func(offset, kind, size, fileOffset uint32) {
		binary.LittleEndian.PutUint32(data[offset:offset+4], kind)
		binary.LittleEndian.PutUint32(data[offset+4:offset+8], size)
		binary.LittleEndian.PutUint32(data[offset+8:offset+12], fileOffset)
	}
	putDescriptor(0x10, 0, 60, 0x40)
	putDescriptor(0x1c, 1, 8, 0x80)
	putDescriptor(0x28, 2, 24, 0x90)
	binary.LittleEndian.PutUint32(data[0x40+52:0x40+56], 1)
	binary.LittleEndian.PutUint32(data[0x40+56:0x40+60], 1)
	binary.LittleEndian.PutUint32(data[0x90:0x94], 1)
	binary.LittleEndian.PutUint32(data[0x94:0x98], 1)
	binary.LittleEndian.PutUint32(data[0x98:0x9c], 24)
	binary.LittleEndian.PutUint32(data[0x9c:0xa0], RelocationMagic)
	binary.LittleEndian.PutUint32(data[0xa0:0xa4], 0)
	binary.LittleEndian.PutUint32(data[0xa4:0xa8], 0xffffffff)
	return data
}
