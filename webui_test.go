package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildWebUIArchiveFromPNGUpload(t *testing.T) {
	t.Parallel()

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	fileWriter, err := writer.CreateFormFile("file", "icon.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}

	pngData, err := samplePNGBytes(256, 256)
	if err != nil {
		t.Fatalf("samplePNGBytes: %v", err)
	}
	if _, err := fileWriter.Write(pngData); err != nil {
		t.Fatalf("write png body: %v", err)
	}

	for key, value := range map[string]string{
		"name":       "desk-app",
		"fit":        "contain",
		"sizes":      "16,32,128,256",
		"background": "transparent",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField %s: %v", key, err)
		}
	}
	if err := writer.WriteField("target", "ico"); err != nil {
		t.Fatalf("WriteField target ico: %v", err)
	}
	if err := writer.WriteField("target", "icns"); err != nil {
		t.Fatalf("WriteField target icns: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/convert", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	archiveName, archivePath, err := buildWebUIArchive(req)
	if err != nil {
		t.Fatalf("buildWebUIArchive: %v", err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(archivePath)) }()

	if archiveName != "desk-app-icons.zip" {
		t.Fatalf("unexpected archive name: %s", archiveName)
	}

	entries, err := listArchiveEntries(archivePath)
	if err != nil {
		t.Fatalf("listArchiveEntries: %v", err)
	}

	foundICO := false
	foundICNS := false
	for _, entry := range entries {
		if entry == "desk-app.ico" {
			foundICO = true
		}
		if entry == "desk-app.icns" {
			foundICNS = true
		}
	}
	if !foundICO || !foundICNS {
		t.Fatalf("archive missing expected outputs: ico=%v icns=%v entries=%v", foundICO, foundICNS, entries)
	}
}

func TestHandleWebUIIndex(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handleWebUIIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Build Icons") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
