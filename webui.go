package main

import (
	"archive/zip"
	_ "embed"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed webui.html
var webUIPage string

func serveWebUI(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleWebUIIndex)
	mux.HandleFunc("/api/convert", handleWebUIConvert)

	fmt.Printf("Web UI listening on http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func handleWebUIIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, webUIPage)
}

func handleWebUIConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	archiveName, archivePath, err := buildWebUIArchive(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer os.RemoveAll(filepath.Dir(archivePath))

	file, err := os.Open(archivePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("open archive: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", archiveName))
	w.Header().Set("X-Download-Name", archiveName)
	if _, err := io.Copy(w, file); err != nil {
		http.Error(w, fmt.Sprintf("stream archive: %v", err), http.StatusInternalServerError)
	}
}

func buildWebUIArchive(r *http.Request) (string, string, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return "", "", fmt.Errorf("parse form: %w", err)
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return "", "", fmt.Errorf("read upload: %w", err)
	}
	defer file.Close()

	targets, err := parseWebTargets(r.MultipartForm.Value["target"])
	if err != nil {
		return "", "", err
	}

	name := sanitizeBaseName(r.FormValue("name"))
	if name == "" {
		name = trimExt(header.Filename)
	}
	if name == "" {
		name = "app"
	}

	sizes := r.FormValue("sizes")
	if strings.TrimSpace(sizes) == "" {
		sizes = "16,24,32,48,64,96,128,256"
	}

	fit := r.FormValue("fit")
	if fit == "" {
		fit = "contain"
	}

	background, err := parseBackground(r.FormValue("background"))
	if err != nil {
		return "", "", err
	}

	tempDir, err := os.MkdirTemp("", "png-convert-webui-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}

	inputName := sanitizeUploadName(header)
	inputPath := filepath.Join(tempDir, inputName)
	if err := writeUpload(inputPath, file); err != nil {
		os.RemoveAll(tempDir)
		return "", "", err
	}

	parsedSizes, err := parseSizes(sizes)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", "", err
	}

	opts := Options{
		InputPath:  inputPath,
		OutputName: name + ".png",
		ICOName:    name + ".ico",
		ICNSName:   name + ".icns",
		Name:       name,
		OutputDir:  tempDir,
		Clean:      true,
		Sizes:      parsedSizes,
		Only:       targets,
		Fit:        fit,
		Archive:    name + "-icons.zip",
		Background: background,
	}
	if err := Convert(opts); err != nil {
		os.RemoveAll(tempDir)
		return "", "", err
	}

	return opts.Archive, filepath.Join(tempDir, opts.Archive), nil
}

func parseWebTargets(values []string) (map[string]bool, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("select at least one output")
	}
	joined := strings.Join(values, ",")
	targets, err := parseOnly(joined)
	if err != nil {
		return nil, err
	}
	delete(targets, "linux")
	delete(targets, "pixmap")
	if len(targets) == 0 {
		return nil, fmt.Errorf("select at least one output")
	}
	return targets, nil
}

func sanitizeBaseName(value string) string {
	value = strings.TrimSpace(value)
	value = filepath.Base(value)
	value = strings.ReplaceAll(value, string(filepath.Separator), "")
	value = strings.ReplaceAll(value, "/", "")
	return trimExt(value)
}

func trimExt(value string) string {
	ext := filepath.Ext(value)
	if ext == "" {
		return value
	}
	return strings.TrimSuffix(value, ext)
}

func sanitizeUploadName(header *multipart.FileHeader) string {
	name := filepath.Base(header.Filename)
	if name == "." || name == "" {
		return "upload.png"
	}
	return name
}

func writeUpload(path string, src multipart.File) error {
	dst, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create upload file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("save upload: %w", err)
	}
	return nil
}

func listArchiveEntries(path string) ([]string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	return names, nil
}
