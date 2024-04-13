package main

import (
	"archive/zip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed webui.html
var webUIPage string

type webUIArtifact struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type webUIResult struct {
	ID          string          `json:"id"`
	ArchiveName string          `json:"archive_name"`
	DownloadURL string          `json:"download_url"`
	Artifacts   []webUIArtifact `json:"artifacts"`
}

type webUIJob struct {
	ArchiveName string
	ArchivePath string
	TempDir     string
	Artifacts   []webUIArtifact
}

var webUIJobs = struct {
	sync.Mutex
	items map[string]webUIJob
}{
	items: map[string]webUIJob{},
}

func serveWebUI(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleWebUIIndex)
	mux.HandleFunc("/api/convert", handleWebUIConvert)
	mux.HandleFunc("/api/download/", handleWebUIDownload)

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

	result, err := buildWebUIResult(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, fmt.Sprintf("encode response: %v", err), http.StatusInternalServerError)
	}
}

func handleWebUIDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := strings.TrimPrefix(r.URL.Path, "/api/download/")
	if jobID == "" || strings.Contains(jobID, "/") {
		http.NotFound(w, r)
		return
	}

	webUIJobs.Lock()
	job, ok := webUIJobs.items[jobID]
	webUIJobs.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	file, err := os.Open(job.ArchivePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("open archive: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", job.ArchiveName))
	if _, err := io.Copy(w, file); err != nil {
		http.Error(w, fmt.Sprintf("stream archive: %v", err), http.StatusInternalServerError)
	}
}

func buildWebUIResult(r *http.Request) (webUIResult, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return webUIResult{}, fmt.Errorf("parse form: %w", err)
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return webUIResult{}, fmt.Errorf("read upload: %w", err)
	}
	defer file.Close()

	targets, err := parseWebTargets(r.MultipartForm.Value["target"])
	if err != nil {
		return webUIResult{}, err
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
		return webUIResult{}, err
	}

	tempDir, err := os.MkdirTemp("", "png-convert-webui-*")
	if err != nil {
		return webUIResult{}, fmt.Errorf("create temp dir: %w", err)
	}

	inputName := sanitizeUploadName(header)
	inputPath := filepath.Join(tempDir, inputName)
	if err := writeUpload(inputPath, file); err != nil {
		os.RemoveAll(tempDir)
		return webUIResult{}, err
	}

	parsedSizes, err := parseSizes(sizes)
	if err != nil {
		os.RemoveAll(tempDir)
		return webUIResult{}, err
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
		return webUIResult{}, err
	}

	artifacts, err := webUIArtifacts(tempDir, name, targets)
	if err != nil {
		os.RemoveAll(tempDir)
		return webUIResult{}, err
	}

	jobID := webUIJobID()
	webUIJobs.Lock()
	webUIJobs.items[jobID] = webUIJob{
		ArchiveName: opts.Archive,
		ArchivePath: filepath.Join(tempDir, opts.Archive),
		TempDir:     tempDir,
		Artifacts:   artifacts,
	}
	webUIJobs.Unlock()

	return webUIResult{
		ID:          jobID,
		ArchiveName: opts.Archive,
		DownloadURL: "/api/download/" + jobID,
		Artifacts:   artifacts,
	}, nil
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

func webUIArtifacts(outputDir, name string, targets map[string]bool) ([]webUIArtifact, error) {
	artifacts := make([]webUIArtifact, 0, 2)
	for _, item := range []struct {
		target string
		path   string
	}{
		{target: "ico", path: filepath.Join(outputDir, name+".ico")},
		{target: "icns", path: filepath.Join(outputDir, name+".icns")},
	} {
		if !targets[item.target] {
			continue
		}
		info, err := os.Stat(item.path)
		if err != nil {
			return nil, fmt.Errorf("stat output %s: %w", item.path, err)
		}
		artifacts = append(artifacts, webUIArtifact{
			Name: filepath.Base(item.path),
			Size: info.Size(),
		})
	}
	return artifacts, nil
}

func webUIJobID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
