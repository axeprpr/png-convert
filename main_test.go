package main

import (
	"archive/zip"
	"encoding/json"
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
		Fit:      "stretch",
		Manifest: "manifest.json",
		Archive:  "artifacts.zip",
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

	manifestData, err := os.ReadFile(filepath.Join(tempDir, "manifest.json"))
	if err != nil {
		t.Fatalf("expected manifest missing: %v", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(manifest.Outputs["ico"]) != 1 {
		t.Fatalf("unexpected ico manifest entries: %v", manifest.Outputs["ico"])
	}

	archiveReader, err := zip.OpenReader(filepath.Join(tempDir, "artifacts.zip"))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer archiveReader.Close()

	foundManifest := false
	foundICO := false
	for _, file := range archiveReader.File {
		if file.Name == "manifest.json" {
			foundManifest = true
		}
		if file.Name == "app.ico" {
			foundICO = true
		}
	}
	if !foundManifest || !foundICO {
		t.Fatalf("archive missing expected files: manifest=%v ico=%v", foundManifest, foundICO)
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
		Fit: "stretch",
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
		Fit: "stretch",
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
		Fit: "stretch",
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
		Fit: "stretch",
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

func TestConvertContainFitPreservesAspectRatio(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.png")
	if err := writeSamplePNG(inputPath, 300, 100); err != nil {
		t.Fatalf("writeSamplePNG: %v", err)
	}

	opts := Options{
		InputPath:  inputPath,
		OutputName: "app.png",
		ICOName:    "app.ico",
		ICNSName:   "AppIcon.icns",
		OutputDir:  tempDir,
		Clean:      true,
		Sizes:      []int{128},
		Only: map[string]bool{
			"linux": true,
		},
		Fit:        "contain",
		Background: color.NRGBA{0, 0, 0, 0},
	}

	if err := Convert(opts); err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}

	outputPath := filepath.Join(tempDir, "icons", "hicolor", "128x128", "apps", "app.png")
	file, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("open output png: %v", err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode output png: %v", err)
	}

	topLeft := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	center := color.NRGBAModel.Convert(img.At(64, 64)).(color.NRGBA)
	if topLeft.A != 0 {
		t.Fatalf("expected transparent padding in contain mode, got alpha=%d", topLeft.A)
	}
	if center.A == 0 {
		t.Fatal("expected image content in center of contain mode output")
	}
}

func TestConvertContainFitCanUseSolidBackground(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.png")
	if err := writeSamplePNG(inputPath, 300, 100); err != nil {
		t.Fatalf("writeSamplePNG: %v", err)
	}

	opts := Options{
		InputPath:  inputPath,
		OutputName: "app.png",
		ICOName:    "app.ico",
		ICNSName:   "AppIcon.icns",
		OutputDir:  tempDir,
		Clean:      true,
		Sizes:      []int{128},
		Only: map[string]bool{
			"linux": true,
		},
		Fit:        "contain",
		Background: color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff},
	}

	if err := Convert(opts); err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}

	outputPath := filepath.Join(tempDir, "icons", "hicolor", "128x128", "apps", "app.png")
	file, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("open output png: %v", err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode output png: %v", err)
	}

	topLeft := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	if topLeft != (color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff}) {
		t.Fatalf("unexpected contain background color: %#v", topLeft)
	}
}

func TestConvertCoverFitFillsCorners(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.png")
	if err := writeSamplePNG(inputPath, 300, 100); err != nil {
		t.Fatalf("writeSamplePNG: %v", err)
	}

	opts := Options{
		InputPath:  inputPath,
		OutputName: "app.png",
		ICOName:    "app.ico",
		ICNSName:   "AppIcon.icns",
		OutputDir:  tempDir,
		Clean:      true,
		Sizes:      []int{128},
		Only: map[string]bool{
			"linux": true,
		},
		Fit:        "cover",
		Background: color.NRGBA{0, 0, 0, 0},
	}

	if err := Convert(opts); err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}

	outputPath := filepath.Join(tempDir, "icons", "hicolor", "128x128", "apps", "app.png")
	file, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("open output png: %v", err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode output png: %v", err)
	}

	topLeft := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	if topLeft.A == 0 {
		t.Fatal("expected cover mode to fill output corners with image content")
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
