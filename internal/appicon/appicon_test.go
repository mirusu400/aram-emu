package appicon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestResizeNearestPreservesPixelBlocks(t *testing.T) {
	source := testIcon()
	resized := ResizeNearest(source, 4, 4)
	if resized == nil {
		t.Fatal("ResizeNearest returned nil")
	}

	checks := []struct {
		point image.Point
		want  color.NRGBA
	}{
		{point: image.Pt(0, 0), want: color.NRGBA{R: 255, A: 255}},
		{point: image.Pt(1, 1), want: color.NRGBA{R: 255, A: 255}},
		{point: image.Pt(2, 0), want: color.NRGBA{G: 255, A: 255}},
		{point: image.Pt(0, 3), want: color.NRGBA{B: 255, A: 255}},
		{point: image.Pt(3, 3), want: color.NRGBA{R: 255, G: 255, A: 128}},
	}
	for _, check := range checks {
		if got := resized.NRGBAAt(check.point.X, check.point.Y); got != check.want {
			t.Errorf("pixel %v = %#v, want %#v", check.point, got, check.want)
		}
	}
}

func TestWriteICOContainsEveryWindowsSize(t *testing.T) {
	var output bytes.Buffer
	if err := WriteICO(&output, testIcon()); err != nil {
		t.Fatal(err)
	}
	data := output.Bytes()
	if got := binary.LittleEndian.Uint16(data[2:4]); got != 1 {
		t.Fatalf("ICO type = %d, want 1", got)
	}
	if got := int(binary.LittleEndian.Uint16(data[4:6])); got != len(windowsSizes) {
		t.Fatalf("ICO image count = %d, want %d", got, len(windowsSizes))
	}

	for index, wantSize := range windowsSizes {
		entry := data[6+index*16 : 6+(index+1)*16]
		gotSize := int(entry[0])
		if gotSize == 0 {
			gotSize = 256
		}
		if gotSize != wantSize || entry[1] != entry[0] {
			t.Errorf("ICO entry %d dimensions = %dx%d, want %dx%d", index, gotSize, gotSize, wantSize, wantSize)
		}
		length := int(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int(binary.LittleEndian.Uint32(entry[12:16]))
		decoded, err := png.Decode(bytes.NewReader(data[offset : offset+length]))
		if err != nil {
			t.Fatalf("decode ICO entry %d: %v", index, err)
		}
		if got := decoded.Bounds().Dx(); got != wantSize {
			t.Errorf("ICO PNG %d width = %d, want %d", index, got, wantSize)
		}
	}
}

func TestWriteICNSContainsEveryMacOSSize(t *testing.T) {
	var output bytes.Buffer
	if err := WriteICNS(&output, testIcon()); err != nil {
		t.Fatal(err)
	}
	data := output.Bytes()
	if got := string(data[:4]); got != "icns" {
		t.Fatalf("ICNS signature = %q, want icns", got)
	}
	if got := int(binary.BigEndian.Uint32(data[4:8])); got != len(data) {
		t.Fatalf("ICNS length = %d, want %d", got, len(data))
	}

	offset := 8
	for _, target := range macOSSizes {
		if got := string(data[offset : offset+4]); got != target.typeCode {
			t.Fatalf("ICNS entry at %d = %q, want %q", offset, got, target.typeCode)
		}
		length := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		decoded, err := png.Decode(bytes.NewReader(data[offset+8 : offset+length]))
		if err != nil {
			t.Fatalf("decode ICNS %s: %v", target.typeCode, err)
		}
		if got := decoded.Bounds().Dx(); got != target.size {
			t.Errorf("ICNS %s width = %d, want %d", target.typeCode, got, target.size)
		}
		offset += length
	}
	if offset != len(data) {
		t.Fatalf("parsed %d ICNS bytes, file has %d", offset, len(data))
	}
}

func TestWriteAndroidResourcesCreatesDensityAndAdaptiveImages(t *testing.T) {
	root := t.TempDir()
	if err := WriteAndroidResources(root, testIcon()); err != nil {
		t.Fatal(err)
	}
	for _, density := range androidDensities {
		launcher := decodePNGFile(t, filepath.Join(
			root,
			"mipmap-"+density.name,
			"ic_aram.png",
		))
		if got := launcher.Bounds().Dx(); got != density.launcherSize {
			t.Errorf("%s launcher width = %d, want %d", density.name, got, density.launcherSize)
		}

		foreground := decodePNGFile(t, filepath.Join(
			root,
			"drawable-"+density.name,
			"ic_aram_foreground.png",
		))
		if got := foreground.Bounds().Dx(); got != density.foregroundSize {
			t.Errorf("%s foreground width = %d, want %d", density.name, got, density.foregroundSize)
		}
		_, _, _, alpha := foreground.At(0, 0).RGBA()
		if alpha != 0 {
			t.Errorf("%s adaptive foreground corner alpha = %d, want 0", density.name, alpha)
		}
	}
}

func testIcon() *image.NRGBA {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	source.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	source.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	source.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, A: 128})
	return source
}

func decodePNGFile(t *testing.T, path string) image.Image {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
