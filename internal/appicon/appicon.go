// Package appicon builds platform packaging resources from ARAM's canonical
// frontend icon without smoothing its pixel art.
package appicon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

var windowsSizes = []int{16, 24, 32, 48, 64, 128, 256}

var macOSSizes = []struct {
	typeCode string
	size     int
}{
	{typeCode: "icp4", size: 16},
	{typeCode: "icp5", size: 32},
	{typeCode: "icp6", size: 64},
	{typeCode: "ic07", size: 128},
	{typeCode: "ic08", size: 256},
	{typeCode: "ic09", size: 512},
	{typeCode: "ic10", size: 1024},
}

var androidDensities = []struct {
	name           string
	launcherSize   int
	foregroundSize int
	artworkSize    int
}{
	{name: "mdpi", launcherSize: 48, foregroundSize: 108, artworkSize: 66},
	{name: "hdpi", launcherSize: 72, foregroundSize: 162, artworkSize: 99},
	{name: "xhdpi", launcherSize: 96, foregroundSize: 216, artworkSize: 132},
	{name: "xxhdpi", launcherSize: 144, foregroundSize: 324, artworkSize: 198},
	{name: "xxxhdpi", launcherSize: 192, foregroundSize: 432, artworkSize: 264},
}

// ResizeNearest scales source without introducing colors between source
// pixels. It returns nil for an empty source or a non-positive target size.
func ResizeNearest(source image.Image, width, height int) *image.NRGBA {
	if source == nil || width <= 0 || height <= 0 {
		return nil
	}
	bounds := source.Bounds()
	if bounds.Empty() {
		return nil
	}

	resized := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		sourceY := bounds.Min.Y + y*bounds.Dy()/height
		for x := range width {
			sourceX := bounds.Min.X + x*bounds.Dx()/width
			resized.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	return resized
}

// WriteICO writes a Windows icon containing PNG-compressed images at the
// sizes Windows uses in Explorer, the taskbar, Alt-Tab, and shortcuts.
func WriteICO(writer io.Writer, source image.Image) error {
	images := make([][]byte, 0, len(windowsSizes))
	for _, size := range windowsSizes {
		encoded, err := encodePNG(ResizeNearest(source, size, size))
		if err != nil {
			return fmt.Errorf("encode %dx%d Windows icon: %w", size, size, err)
		}
		images = append(images, encoded)
	}

	if err := binary.Write(writer, binary.LittleEndian, uint16(0)); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, uint16(len(images))); err != nil {
		return err
	}

	offset := uint32(6 + len(images)*16)
	for index, encoded := range images {
		size := windowsSizes[index]
		dimension := byte(size)
		if size == 256 {
			dimension = 0
		}
		entry := []byte{dimension, dimension, 0, 0}
		if _, err := writer.Write(entry); err != nil {
			return err
		}
		for _, value := range []any{
			uint16(1), uint16(32), uint32(len(encoded)), offset,
		} {
			if err := binary.Write(writer, binary.LittleEndian, value); err != nil {
				return err
			}
		}
		offset += uint32(len(encoded))
	}

	for _, encoded := range images {
		if _, err := writer.Write(encoded); err != nil {
			return err
		}
	}
	return nil
}

// WriteICNS writes a modern macOS icon family using PNG-backed ICNS entries.
func WriteICNS(writer io.Writer, source image.Image) error {
	type iconEntry struct {
		typeCode string
		encoded  []byte
	}
	entries := make([]iconEntry, 0, len(macOSSizes))
	totalSize := uint32(8)
	for _, target := range macOSSizes {
		encoded, err := encodePNG(ResizeNearest(source, target.size, target.size))
		if err != nil {
			return fmt.Errorf("encode %dx%d macOS icon: %w", target.size, target.size, err)
		}
		entries = append(entries, iconEntry{typeCode: target.typeCode, encoded: encoded})
		totalSize += uint32(8 + len(encoded))
	}

	if _, err := writer.Write([]byte("icns")); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.BigEndian, totalSize); err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := writer.Write([]byte(entry.typeCode)); err != nil {
			return err
		}
		if err := binary.Write(
			writer,
			binary.BigEndian,
			uint32(8+len(entry.encoded)),
		); err != nil {
			return err
		}
		if _, err := writer.Write(entry.encoded); err != nil {
			return err
		}
	}
	return nil
}

// WriteAndroidResources writes density-specific legacy launcher images and
// safely padded adaptive-icon foregrounds beneath root.
func WriteAndroidResources(root string, source image.Image) error {
	if source == nil || source.Bounds().Empty() {
		return fmt.Errorf("source icon is empty")
	}
	for _, density := range androidDensities {
		launcher := ResizeNearest(
			source,
			density.launcherSize,
			density.launcherSize,
		)
		launcherPath := filepath.Join(
			root,
			"mipmap-"+density.name,
			"ic_aram.png",
		)
		if err := writePNG(launcherPath, launcher); err != nil {
			return fmt.Errorf("write Android %s launcher icon: %w", density.name, err)
		}

		foreground := image.NewNRGBA(image.Rect(
			0,
			0,
			density.foregroundSize,
			density.foregroundSize,
		))
		artwork := ResizeNearest(
			source,
			density.artworkSize,
			density.artworkSize,
		)
		offset := (density.foregroundSize - density.artworkSize) / 2
		draw.Draw(
			foreground,
			image.Rect(
				offset,
				offset,
				offset+density.artworkSize,
				offset+density.artworkSize,
			),
			artwork,
			image.Point{},
			draw.Over,
		)
		foregroundPath := filepath.Join(
			root,
			"drawable-"+density.name,
			"ic_aram_foreground.png",
		)
		if err := writePNG(foregroundPath, foreground); err != nil {
			return fmt.Errorf("write Android %s adaptive icon: %w", density.name, err)
		}
	}
	return nil
}

func encodePNG(source image.Image) ([]byte, error) {
	if source == nil || source.Bounds().Empty() {
		return nil, fmt.Errorf("source icon is empty")
	}
	var buffer bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&buffer, source); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writePNG(path string, source image.Image) error {
	encoded, err := encodePNG(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}
