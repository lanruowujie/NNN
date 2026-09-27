package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChecksumsAndPathScan(t *testing.T) {
	root := t.TempDir()
	needle := filepath.Join(root, "private", "developer", "workspace")
	artifact := filepath.Join(root, "artifact.zip")
	if err := os.WriteFile(artifact, []byte("release"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "SHA256SUMS")
	if err := run([]string{"checksums", "-output", output, artifact}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "  artifact.zip\n") {
		t.Fatalf("checksums = %q", data)
	}
	if err := run([]string{"scan-paths", "-root", root, "-needle", needle}, &bytes.Buffer{}); err != nil {
		t.Fatalf("clean scan: %v", err)
	}
	if err := os.WriteFile(artifact, []byte(filepath.Join(needle, "file.c")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"scan-paths", "-root", root, "-needle", needle}, &bytes.Buffer{}); err == nil {
		t.Fatal("forbidden path was not detected")
	}
}

func TestSetVersionUpdatesWailsInfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wails.json")
	if err := os.WriteFile(path, []byte(`{"name":"NFCX","info":{"productVersion":"old"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"set-version", "-file", path, "-version", "1.2.3-rc.1"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"productVersion": "1.2.3"`) {
		t.Fatalf("updated config = %s", data)
	}
}
