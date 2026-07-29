package eads

import (
	"encoding/binary"
	"testing"
)

func TestParseSyntheticImage(t *testing.T) {
	data := make([]byte, HeaderSize+8)
	copy(data, Magic)
	binary.LittleEndian.PutUint32(data[4:8], 1)
	binary.LittleEndian.PutUint32(data[8:12], 2)
	binary.LittleEndian.PutUint32(data[12:16], 0xf4000000)
	binary.LittleEndian.PutUint32(data[16:20], 8)
	binary.LittleEndian.PutUint32(data[20:24], 0xf5000000)
	binary.LittleEndian.PutUint32(data[24:28], 0x1000)
	copy(data[0x20:0x30], []byte("Synthetic"))
	copy(data[HeaderSize:], []byte{0x00, 0xb5, 0x70, 0x47})

	image, err := Parse(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if image.Name != "Synthetic" {
		t.Fatalf("name = %q", image.Name)
	}
	if image.RecordEnd() != HeaderSize+8 {
		t.Fatalf("record end = %#x", image.RecordEnd())
	}
}

func TestParseRejectsOverlappingRanges(t *testing.T) {
	data := make([]byte, HeaderSize+8)
	copy(data, Magic)
	binary.LittleEndian.PutUint32(data[12:16], 0x1000)
	binary.LittleEndian.PutUint32(data[16:20], 8)
	binary.LittleEndian.PutUint32(data[20:24], 0x1000)
	binary.LittleEndian.PutUint32(data[24:28], 0x1000)
	copy(data[0x20:0x30], []byte("Overlap"))
	copy(data[HeaderSize:], []byte{0x00, 0xb5, 0x70, 0x47})
	if _, err := Parse(data, 0); err == nil {
		t.Fatal("Parse accepted overlapping guest ranges")
	}
}
