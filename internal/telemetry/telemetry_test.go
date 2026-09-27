package telemetry

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

func TestStoreCreatesRandomIDOnlyWhenEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "NFCX", "telemetry.json")
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	disabled, err := store.SetEnabled(false)
	if err != nil {
		t.Fatal(err)
	}
	if !disabled.Configured || disabled.InstallationID != "" {
		t.Fatalf("disabled settings = %+v", disabled)
	}
	enabled, err := store.SetEnabled(true)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(enabled.InstallationID) {
		t.Fatalf("invalid UUID %q", enabled.InstallationID)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat settings: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("settings mode = %v, want a regular file", info.Mode())
	}
	// Windows does not expose POSIX owner/group/other permission bits through
	// os.FileMode; a file created with 0600 is commonly reported as 0666.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("settings permissions = %v, want 0600", info.Mode().Perm())
	}
}

func TestCoarseOSIsAllowlisted(t *testing.T) {
	if _, ok := map[string]bool{"windows": true, "macos": true, "linux": true, "other": true}[coarseOS()]; !ok {
		t.Fatal(coarseOS())
	}
}
