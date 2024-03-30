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

func TestParseSizesRejectsInvalidTokens(t *testing.T) {
	t.Parallel()

	if _, err := parseSizes("16x,32"); err == nil {
		t.Fatal("expected parseSizes to reject invalid size token")
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
		Only: map[string]bool{
			"linux":  true,
			"pixmap": true,
			"ico":    true,
			"icns":   true,
		},
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

func TestConvertAlwaysGeneratesPixmap(t *testing.T) {
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
		Sizes:      []int{16, 32, 64, 256},
		Only: map[string]bool{
			"linux":  true,
			"pixmap": true,
			"ico":    true,
			"icns":   true,
		},
	}

	if err := Convert(opts); err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}

	pixmapPath := filepath.Join(tempDir, "pixmaps", "app.png")
	if _, err := os.Stat(pixmapPath); err != nil {
		t.Fatalf("expected pixmap artifact missing: %v", err)
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
		Only: map[string]bool{
			"linux": true,
		},
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
		Only: map[string]bool{
			"linux": true,
		},
	})
	if err == nil {
		t.Fatal("expected validation error for output path traversal")
	}
}

func TestConvertCanGenerateOnlyICNS(t *testing.T) {
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
		Sizes:      []int{16, 32, 128, 256},
		Only: map[string]bool{
			"icns": true,
		},
	}

	if err := Convert(opts); err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tempDir, "AppIcon.icns")); err != nil {
		t.Fatalf("expected icns artifact missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "app.ico")); !os.IsNotExist(err) {
		t.Fatalf("expected ico artifact to be omitted, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "icons")); !os.IsNotExist(err) {
		t.Fatalf("expected linux icons to be omitted, got err=%v", err)
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
