package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSizes(t *testing.T) {
	t.Parallel()

	sizes, err := parseSizes("256,16,32,16")
	if err != nil {
		t.Fatalf("parseSizes returned error: %v", err)
	}

	expected := []int{16, 32, 256}
	if len(sizes) != len(expected) {
		t.Fatalf("unexpected size count: got %d want %d", len(sizes), len(expected))
	}

	for i, size := range expected {
		if sizes[i] != size {
			t.Fatalf("unexpected sizes[%d]: got %d want %d", i, sizes[i], size)
		}
	}
}

func TestConvertGeneratesExpectedArtifacts(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.png")
	if err := writeSamplePNG(inputPath, 512, 512); err != nil {
		t.Fatalf("writeSamplePNG: %v", err)
	}

	opts := Options{
		InputPath:  inputPath,
		OutputName: "app.png",
		ICOName:    "app.ico",
		ICNSName:   "AppIcon.icns",
		OutputDir:  tempDir,
		Clean:      true,
		Sizes:      []int{16, 32, 128, 256, 512},
	}

	if err := Convert(opts); err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}

	for _, path := range []string{
		filepath.Join(tempDir, "icons", "hicolor", "16x16", "apps", "app.png"),
		filepath.Join(tempDir, "icons", "hicolor", "32x32", "apps", "app.png"),
		filepath.Join(tempDir, "icons", "hicolor", "128x128", "apps", "app.png"),
		filepath.Join(tempDir, "icons", "hicolor", "256x256", "apps", "app.png"),
		filepath.Join(tempDir, "icons", "hicolor", "512x512", "apps", "app.png"),
		filepath.Join(tempDir, "pixmaps", "app.png"),
		filepath.Join(tempDir, "app.ico"),
		filepath.Join(tempDir, "AppIcon.icns"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected artifact missing: %s: %v", path, err)
		}
		if info.Size() == 0 {
			t.Fatalf("artifact is empty: %s", path)
		}
	}
}

func TestConvertRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	err := Convert(Options{
		InputPath:  "input.jpg",
		OutputName: "output.png",
		ICOName:    "app.ico",
		ICNSName:   "AppIcon.icns",
		OutputDir:  ".",
		Sizes:      []int{16},
	})
	if err == nil {
		t.Fatal("expected validation error for non-PNG input")
	}
}

func TestConvertRejectsOutputPathTraversal(t *testing.T) {
	t.Parallel()

	err := Convert(Options{
		InputPath:  "input.png",
		OutputName: "../output.png",
		ICOName:    "app.ico",
		ICNSName:   "AppIcon.icns",
		OutputDir:  ".",
		Sizes:      []int{16},
	})
	if err == nil {
		t.Fatal("expected validation error for output path traversal")
	}
}

func writeSamplePNG(path string, width, height int) error {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.NRGBA{
				R: uint8(x % 255),
				G: uint8(y % 255),
				B: uint8((x + y) % 255),
				A: 255,
			})
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}
