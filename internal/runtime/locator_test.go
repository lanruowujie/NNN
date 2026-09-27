package runtime_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

func TestLocatorResolvesControlledExecutableAndHash(t *testing.T) {
	root := t.TempDir()
	name := "engine"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(root, name)
	contents := []byte("fixture")
	if err := os.WriteFile(path, contents, 0o755); err != nil {
		t.Fatal(err)
	}
	wanted := fmt.Sprintf("%x", sha256.Sum256(contents))
	resolved, err := runtimebundle.NewLocator(root).Resolve(runtimebundle.Executable{
		ID: "fixture", FileName: "engine", WindowsFileName: "engine.exe", SHA256: wanted,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != want {
		t.Fatalf("resolved %q; want %q", resolved, want)
	}
}

func TestLocatorRejectsUnsafeNamesAndHashMismatch(t *testing.T) {
	root := t.TempDir()
	locator := runtimebundle.NewLocator(root)
	if _, err := locator.Resolve(runtimebundle.Executable{ID: "bad", FileName: "../bad"}); !errors.Is(err, runtimebundle.ErrUnsafeExecutable) {
		t.Fatalf("unsafe name error = %v", err)
	}
	name := "engine"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := locator.Resolve(runtimebundle.Executable{ID: "fixture", FileName: "engine", WindowsFileName: "engine.exe", SHA256: fmt.Sprintf("%064x", 1)})
	if !errors.Is(err, runtimebundle.ErrHashMismatch) {
		t.Fatalf("hash error = %v", err)
	}
}

func TestLocatorRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks normally requires additional Windows privileges")
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "engine")
	if err := os.WriteFile(outside, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "engine")); err != nil {
		t.Fatal(err)
	}
	_, err := runtimebundle.NewLocator(root).Resolve(runtimebundle.Executable{ID: "fixture", FileName: "engine"})
	if !errors.Is(err, runtimebundle.ErrUnsafeExecutable) {
		t.Fatalf("symlink escape error = %v", err)
	}
}

func TestLocatorRuntimeRootUsesFirstExistingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	root := t.TempDir()
	got, err := runtimebundle.NewLocator(missing, root).RuntimeRoot()
	if err != nil {
		t.Fatalf("RuntimeRoot: %v", err)
	}
	if got != root {
		t.Fatalf("RuntimeRoot = %q, want %q", got, root)
	}
}
