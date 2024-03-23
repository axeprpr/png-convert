package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/jackmordaunt/icns/v3"
)

var version = "dev"

type Options struct {
	InputPath  string
	OutputName string
	ICOName    string
	ICNSName   string
	OutputDir  string
	Clean      bool
	Sizes      []int
}

type icondir struct {
	Reserved  uint16
	ImageType uint16
	NumImages uint16
}

type icondirentry struct {
	ImageWidth   uint8
	ImageHeight  uint8
	NumColors    uint8
	Reserved     uint8
	ColorPlanes  uint16
	BitsPerPixel uint16
	SizeInBytes  uint32
	Offset       uint32
}

func newIcondir(numImages uint16) icondir {
	return icondir{
		ImageType: 1,
		NumImages: numImages,
	}
}

func newIcondirentry() icondirentry {
	return icondirentry{
		ColorPlanes:  1,
		BitsPerPixel: 32,
	}
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	opts, err := parseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid arguments: %v\n", err)
		os.Exit(2)
	}
	if *showVersion {
		fmt.Println(version)
		return
	}

	if err := Convert(opts); err != nil {
		fmt.Fprintf(os.Stderr, "conversion failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Conversion completed.")
}

func parseFlags() (Options, error) {
	var sizesArg string

	opts := Options{}
	flag.StringVar(&opts.InputPath, "i", "input.png", "input PNG file path")
	flag.StringVar(&opts.OutputName, "o", "output.png", "PNG filename written into icon directories")
	flag.StringVar(&opts.ICOName, "w", "app.ico", "ICO output filename")
	flag.StringVar(&opts.ICNSName, "m", "AppIcon.icns", "ICNS output filename")
	flag.StringVar(&opts.OutputDir, "d", ".", "base output directory")
	flag.BoolVar(&opts.Clean, "clean", false, "remove generated output directories before regenerating")
	flag.StringVar(&sizesArg, "sizes", "16,24,32,48,64,96,128,256,512", "comma separated icon sizes")
	flag.Parse()

	sizes, err := parseSizes(sizesArg)
	if err != nil {
		return Options{}, err
	}
	opts.Sizes = sizes

	return opts, validateOptions(opts)
}

func parseSizes(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	sizes := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		var size int
		if _, err := fmt.Sscanf(part, "%d", &size); err != nil {
			return nil, fmt.Errorf("parse size %q: %w", part, err)
		}
		if size <= 0 || size > 512 {
			return nil, fmt.Errorf("size %d must be between 1 and 512", size)
		}
		if _, ok := seen[size]; ok {
			continue
		}
		seen[size] = struct{}{}
		sizes = append(sizes, size)
	}

	if len(sizes) == 0 {
		return nil, errors.New("at least one icon size is required")
	}

	sort.Ints(sizes)
	return sizes, nil
}

func validateOptions(opts Options) error {
	if opts.InputPath == "" {
		return errors.New("input path is required")
	}
	if filepath.Ext(opts.InputPath) != ".png" {
		return errors.New("input file must be a PNG")
	}
	if opts.OutputName == "" || filepath.Ext(opts.OutputName) != ".png" {
		return errors.New("output PNG name must end with .png")
	}
	if opts.ICOName == "" || filepath.Ext(opts.ICOName) != ".ico" {
		return errors.New("ICO output name must end with .ico")
	}
	if opts.ICNSName == "" || filepath.Ext(opts.ICNSName) != ".icns" {
		return errors.New("ICNS output name must end with .icns")
	}
	if opts.OutputDir == "" {
		return errors.New("output directory is required")
	}
	if len(opts.Sizes) == 0 {
		return errors.New("at least one size is required")
	}
	return nil
}

func Convert(opts Options) error {
	if err := validateOptions(opts); err != nil {
		return err
	}

	srcImage, err := imaging.Open(opts.InputPath)
	if err != nil {
		return fmt.Errorf("open input image: %w", err)
	}

	iconsRoot := filepath.Join(opts.OutputDir, "icons", "hicolor")
	pixmapsRoot := filepath.Join(opts.OutputDir, "pixmaps")

	if opts.Clean {
		for _, path := range []string{iconsRoot, pixmapsRoot} {
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("remove %s: %w", path, err)
			}
		}
	}

	if err := os.MkdirAll(iconsRoot, 0o755); err != nil {
		return fmt.Errorf("create icons root: %w", err)
	}
	if err := os.MkdirAll(pixmapsRoot, 0o755); err != nil {
		return fmt.Errorf("create pixmaps root: %w", err)
	}

	for _, size := range opts.Sizes {
		sizeDir := filepath.Join(iconsRoot, fmt.Sprintf("%dx%d", size, size), "apps")
		if err := os.MkdirAll(sizeDir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", sizeDir, err)
		}

		outputPath := filepath.Join(sizeDir, opts.OutputName)
		resizedImage := imaging.Resize(srcImage, size, size, imaging.Lanczos)
		if err := imaging.Save(resizedImage, outputPath); err != nil {
			return fmt.Errorf("save resized image %s: %w", outputPath, err)
		}

		if size == 128 {
			pixmapPath := filepath.Join(pixmapsRoot, opts.OutputName)
			if err := imaging.Save(resizedImage, pixmapPath); err != nil {
				return fmt.Errorf("save pixmap %s: %w", pixmapPath, err)
			}
		}
	}

	icoPath := filepath.Join(opts.OutputDir, opts.ICOName)
	if err := writeICO(icoPath, iconsRoot, opts.OutputName, filterICOSizes(opts.Sizes)); err != nil {
		return err
	}

	icnsPath := filepath.Join(opts.OutputDir, opts.ICNSName)
	if err := writeICNS(icnsPath, srcImage); err != nil {
		return err
	}

	return nil
}

func filterICOSizes(sizes []int) []int {
	filtered := make([]int, 0, len(sizes))
	for _, size := range sizes {
		if size <= 256 {
			filtered = append(filtered, size)
		}
	}
	return filtered
}

func writeICO(outputPath, iconsRoot, outputName string, sizes []int) error {
	if len(sizes) == 0 {
		return errors.New("ICO generation requires at least one size <= 256")
	}

	iconDir := newIcondir(uint16(len(sizes)))
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, iconDir); err != nil {
		return fmt.Errorf("write ico header: %w", err)
	}

	globalOffset := uint32(6 + len(sizes)*16)
	pngAll := new(bytes.Buffer)

	for _, size := range sizes {
		pngPath := filepath.Join(iconsRoot, fmt.Sprintf("%dx%d", size, size), "apps", outputName)
		img, err := readPNG(pngPath)
		if err != nil {
			return err
		}

		ide := newIcondirentry()
		pngBuf := new(bytes.Buffer)
		pngWriter := bufio.NewWriter(pngBuf)
		if err := png.Encode(pngWriter, img); err != nil {
			return fmt.Errorf("encode png %s: %w", pngPath, err)
		}
		if err := pngWriter.Flush(); err != nil {
			return fmt.Errorf("flush png writer %s: %w", pngPath, err)
		}

		ide.SizeInBytes = uint32(pngBuf.Len())
		bounds := img.Bounds()
		ide.ImageWidth = pngDimensionByte(bounds.Dx())
		ide.ImageHeight = pngDimensionByte(bounds.Dy())
		ide.Offset = globalOffset
		globalOffset += ide.SizeInBytes

		if err := binary.Write(buf, binary.LittleEndian, ide); err != nil {
			return fmt.Errorf("write ico entry for %s: %w", pngPath, err)
		}
		if _, err := pngAll.Write(pngBuf.Bytes()); err != nil {
			return fmt.Errorf("buffer ico png data for %s: %w", pngPath, err)
		}
	}

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create ico file: %w", err)
	}
	defer outputFile.Close()

	if _, err := outputFile.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("write ico metadata: %w", err)
	}
	if _, err := outputFile.Write(pngAll.Bytes()); err != nil {
		return fmt.Errorf("write ico image payload: %w", err)
	}

	return nil
}

func pngDimensionByte(value int) uint8 {
	if value >= 256 {
		return 0
	}
	return uint8(value)
}

func readPNG(path string) (image.Image, error) {
	pngFile, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open png %s: %w", path, err)
	}
	defer pngFile.Close()

	img, err := png.Decode(pngFile)
	if err != nil {
		return nil, fmt.Errorf("decode png %s: %w", path, err)
	}

	return img, nil
}

func writeICNS(outputPath string, srcImage image.Image) error {
	icnsFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create icns file: %w", err)
	}
	defer icnsFile.Close()

	if err := icns.Encode(icnsFile, srcImage); err != nil {
		return fmt.Errorf("encode icns: %w", err)
	}

	return nil
}
