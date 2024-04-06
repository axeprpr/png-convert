package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	Only       map[string]bool
	Fit        string
	Manifest   string
	Archive    string
	Background color.NRGBA
}

type Manifest struct {
	InputPath string              `json:"input_path"`
	OutputDir string              `json:"output_dir"`
	Outputs   map[string][]string `json:"outputs"`
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
	var onlyArg string

	opts := Options{}
	flag.StringVar(&opts.InputPath, "i", "input.png", "input PNG file path")
	flag.StringVar(&opts.OutputName, "o", "output.png", "PNG filename written into icon directories")
	flag.StringVar(&opts.ICOName, "w", "app.ico", "ICO output filename")
	flag.StringVar(&opts.ICNSName, "m", "AppIcon.icns", "ICNS output filename")
	flag.StringVar(&opts.OutputDir, "d", ".", "base output directory")
	flag.BoolVar(&opts.Clean, "clean", false, "remove generated output directories before regenerating")
	flag.StringVar(&sizesArg, "sizes", "16,24,32,48,64,96,128,256,512", "comma separated icon sizes")
	flag.StringVar(&onlyArg, "only", "linux,pixmap,ico,icns", "comma separated outputs: linux,pixmap,ico,icns")
	flag.StringVar(&opts.Fit, "fit", "stretch", "resize mode: stretch, contain, or cover")
	flag.StringVar(&opts.Manifest, "manifest", "", "optional JSON manifest filename to write under output directory")
	flag.StringVar(&opts.Archive, "archive", "", "optional ZIP filename to package generated artifacts under output directory")
	backgroundArg := flag.String("background", "transparent", "background color for contain mode, use transparent or #RRGGBB[AA]")
	flag.Parse()

	sizes, err := parseSizes(sizesArg)
	if err != nil {
		return Options{}, err
	}
	opts.Sizes = sizes

	only, err := parseOnly(onlyArg)
	if err != nil {
		return Options{}, err
	}
	opts.Only = only

	background, err := parseBackground(*backgroundArg)
	if err != nil {
		return Options{}, err
	}
	opts.Background = background

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

		size, err := strconv.Atoi(part)
		if err != nil {
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
	if filepath.Base(opts.OutputName) != opts.OutputName {
		return errors.New("output PNG name must not contain path separators")
	}
	if opts.ICOName == "" || filepath.Ext(opts.ICOName) != ".ico" {
		return errors.New("ICO output name must end with .ico")
	}
	if filepath.Base(opts.ICOName) != opts.ICOName {
		return errors.New("ICO output name must not contain path separators")
	}
	if opts.ICNSName == "" || filepath.Ext(opts.ICNSName) != ".icns" {
		return errors.New("ICNS output name must end with .icns")
	}
	if filepath.Base(opts.ICNSName) != opts.ICNSName {
		return errors.New("ICNS output name must not contain path separators")
	}
	if opts.OutputDir == "" {
		return errors.New("output directory is required")
	}
	if len(opts.Sizes) == 0 {
		return errors.New("at least one size is required")
	}
	if len(opts.Only) == 0 {
		return errors.New("at least one output target is required")
	}
	if opts.Fit != "stretch" && opts.Fit != "contain" && opts.Fit != "cover" {
		return fmt.Errorf("unsupported fit mode %q", opts.Fit)
	}
	if opts.Manifest != "" {
		if filepath.Base(opts.Manifest) != opts.Manifest {
			return errors.New("manifest filename must not contain path separators")
		}
		if filepath.Ext(opts.Manifest) != ".json" {
			return errors.New("manifest filename must end with .json")
		}
	}
	if opts.Archive != "" {
		if filepath.Base(opts.Archive) != opts.Archive {
			return errors.New("archive filename must not contain path separators")
		}
		if filepath.Ext(opts.Archive) != ".zip" {
			return errors.New("archive filename must end with .zip")
		}
	}
	return nil
}

func parseOnly(raw string) (map[string]bool, error) {
	valid := map[string]bool{
		"linux":  true,
		"pixmap": true,
		"ico":    true,
		"icns":   true,
	}

	parts := strings.Split(raw, ",")
	selected := make(map[string]bool, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		if part == "all" {
			return map[string]bool{
				"linux":  true,
				"pixmap": true,
				"ico":    true,
				"icns":   true,
			}, nil
		}
		if !valid[part] {
			return nil, fmt.Errorf("unsupported output target %q", part)
		}
		selected[part] = true
	}

	if len(selected) == 0 {
		return nil, errors.New("at least one output target is required")
	}

	return selected, nil
}

func parseBackground(raw string) (color.NRGBA, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "transparent" {
		return color.NRGBA{0, 0, 0, 0}, nil
	}
	if !strings.HasPrefix(raw, "#") {
		return color.NRGBA{}, fmt.Errorf("background %q must be transparent or start with #", raw)
	}
	hex := strings.TrimPrefix(raw, "#")
	switch len(hex) {
	case 6:
		var rgb uint32
		if _, err := fmt.Sscanf(hex, "%06x", &rgb); err != nil {
			return color.NRGBA{}, fmt.Errorf("parse background %q: %w", raw, err)
		}
		return color.NRGBA{
			R: uint8(rgb >> 16),
			G: uint8(rgb >> 8),
			B: uint8(rgb),
			A: 255,
		}, nil
	case 8:
		var rgba uint32
		if _, err := fmt.Sscanf(hex, "%08x", &rgba); err != nil {
			return color.NRGBA{}, fmt.Errorf("parse background %q: %w", raw, err)
		}
		return color.NRGBA{
			R: uint8(rgba >> 24),
			G: uint8(rgba >> 16),
			B: uint8(rgba >> 8),
			A: uint8(rgba),
		}, nil
	default:
		return color.NRGBA{}, fmt.Errorf("background %q must use #RRGGBB or #RRGGBBAA", raw)
	}
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
		for _, path := range cleanPaths(opts, iconsRoot, pixmapsRoot) {
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("remove %s: %w", path, err)
			}
		}
	}

	if opts.Only["linux"] {
		if err := os.MkdirAll(iconsRoot, 0o755); err != nil {
			return fmt.Errorf("create icons root: %w", err)
		}
	}
	if opts.Only["pixmap"] {
		if err := os.MkdirAll(pixmapsRoot, 0o755); err != nil {
			return fmt.Errorf("create pixmaps root: %w", err)
		}
	}

	manifest := Manifest{
		InputPath: opts.InputPath,
		OutputDir: opts.OutputDir,
		Outputs:   map[string][]string{},
	}

	if opts.Only["linux"] || opts.Only["ico"] {
		for _, size := range opts.Sizes {
			sizeDir := filepath.Join(iconsRoot, fmt.Sprintf("%dx%d", size, size), "apps")
			if err := os.MkdirAll(sizeDir, 0o755); err != nil {
				return fmt.Errorf("create %s: %w", sizeDir, err)
			}

			outputPath := filepath.Join(sizeDir, opts.OutputName)
			resizedImage := resizeSquare(srcImage, size, opts.Fit, opts.Background)
			if err := imaging.Save(resizedImage, outputPath); err != nil {
				return fmt.Errorf("save resized image %s: %w", outputPath, err)
			}
			if opts.Only["linux"] {
				manifest.Outputs["linux"] = append(manifest.Outputs["linux"], outputPath)
			}
		}
	}

	if opts.Only["pixmap"] {
		pixmapPath := filepath.Join(pixmapsRoot, opts.OutputName)
		pixmapImage := resizeSquare(srcImage, 128, opts.Fit, opts.Background)
		if err := imaging.Save(pixmapImage, pixmapPath); err != nil {
			return fmt.Errorf("save pixmap %s: %w", pixmapPath, err)
		}
		manifest.Outputs["pixmap"] = append(manifest.Outputs["pixmap"], pixmapPath)
	}

	if opts.Only["ico"] {
		icoPath := filepath.Join(opts.OutputDir, opts.ICOName)
		if err := writeICO(icoPath, iconsRoot, opts.OutputName, filterICOSizes(opts.Sizes)); err != nil {
			return err
		}
		manifest.Outputs["ico"] = append(manifest.Outputs["ico"], icoPath)
	}

	if opts.Only["icns"] {
		icnsPath := filepath.Join(opts.OutputDir, opts.ICNSName)
		if err := writeICNS(icnsPath, srcImage); err != nil {
			return err
		}
		manifest.Outputs["icns"] = append(manifest.Outputs["icns"], icnsPath)
	}

	if opts.Manifest != "" {
		manifestPath := filepath.Join(opts.OutputDir, opts.Manifest)
		if err := writeManifest(manifestPath, manifest); err != nil {
			return err
		}
		manifest.Outputs["manifest"] = append(manifest.Outputs["manifest"], manifestPath)
	}

	if opts.Archive != "" {
		archivePath := filepath.Join(opts.OutputDir, opts.Archive)
		if err := writeArchive(archivePath, opts.OutputDir, manifest.Outputs); err != nil {
			return err
		}
	}

	return nil
}

func cleanPaths(opts Options, iconsRoot, pixmapsRoot string) []string {
	paths := make([]string, 0, 4)
	if opts.Only["linux"] || opts.Only["ico"] {
		paths = append(paths, iconsRoot)
	}
	if opts.Only["pixmap"] {
		paths = append(paths, pixmapsRoot)
	}
	if opts.Only["ico"] {
		paths = append(paths, filepath.Join(opts.OutputDir, opts.ICOName))
	}
	if opts.Only["icns"] {
		paths = append(paths, filepath.Join(opts.OutputDir, opts.ICNSName))
	}
	return paths
}

func resizeSquare(src image.Image, size int, fit string, background color.NRGBA) image.Image {
	if fit == "contain" {
		fitted := imaging.Fit(src, size, size, imaging.Lanczos)
		canvas := imaging.New(size, size, background)
		return imaging.PasteCenter(canvas, fitted)
	}
	if fit == "cover" {
		return imaging.Fill(src, size, size, imaging.Center, imaging.Lanczos)
	}
	return imaging.Resize(src, size, size, imaging.Lanczos)
}

func writeManifest(path string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	return nil
}

func writeArchive(path, baseDir string, outputs map[string][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create archive %s: %w", path, err)
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	defer zw.Close()

	seen := map[string]bool{}
	for _, paths := range outputs {
		for _, outputPath := range paths {
			if seen[outputPath] {
				continue
			}
			seen[outputPath] = true

			relPath, err := filepath.Rel(baseDir, outputPath)
			if err != nil {
				return fmt.Errorf("resolve archive path for %s: %w", outputPath, err)
			}

			if err := addFileToZip(zw, outputPath, filepath.ToSlash(relPath)); err != nil {
				return err
			}
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("close archive %s: %w", path, err)
	}

	return nil
}

func addFileToZip(zw *zip.Writer, sourcePath, archivePath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("stat archive source %s: %w", sourcePath, err)
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("build zip header for %s: %w", sourcePath, err)
	}
	header.Name = archivePath
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create zip entry for %s: %w", sourcePath, err)
	}

	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open archive source %s: %w", sourcePath, err)
	}
	defer file.Close()

	if _, err := io.Copy(writer, file); err != nil {
		return fmt.Errorf("copy archive source %s: %w", sourcePath, err)
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
