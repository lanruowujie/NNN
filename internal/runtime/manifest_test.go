package runtime_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

func TestBuildWriteAndVerifyManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "engine"), []byte("engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "LICENSES"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "LICENSES", "COPYING"), []byte("license"), 0o644); err != nil {
		t.Fatal(err)
	}
	components := []runtimebundle.ManifestComponent{{ID: "engine", Version: "1.0", License: "GPL-2.0-or-later", Source: "https://example.invalid/engine"}}
	manifest, err := runtimebundle.BuildManifest(root, "manifest.json", "test-amd64", "1.2.3", "abc", components)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "manifest.json")
	if err := runtimebundle.WriteManifest(path, manifest); err != nil {
		t.Fatal(err)
	}
	verified, err := runtimebundle.VerifyManifest(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Verified != 2 || verified.Manifest.Platform != "test-amd64" {
		t.Fatalf("verification = %#v", verified)
	}
	if err := os.WriteFile(filepath.Join(root, "unlisted"), []byte("extra"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimebundle.VerifyManifest(root, path); !errors.Is(err, runtimebundle.ErrInvalidManifest) {
		t.Fatalf("unlisted file error = %v", err)
	}
	if err := os.Remove(filepath.Join(root, "unlisted")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "engine"), []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimebundle.VerifyManifest(root, path); !errors.Is(err, runtimebundle.ErrHashMismatch) {
		t.Fatalf("tamper error = %v", err)
	}
}

func TestManifestRejectsEscapingPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "manifest.json")
	manifest := runtimebundle.Manifest{SchemaVersion: 1, Platform: "test-amd64", NFCXVersion: "1", Commit: "abc", Files: []runtimebundle.ManifestFile{{Path: "../outside", SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}}}
	if err := runtimebundle.WriteManifest(path, manifest); !errors.Is(err, runtimebundle.ErrInvalidManifest) {
		t.Fatalf("unsafe manifest error = %v", err)
	}
}
