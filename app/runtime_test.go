package app

import (
	"os"
	"testing"
)

func TestConfigureLibNFCDiscoverySetsMissingDefaults(t *testing.T) {
	for _, name := range []string{"LIBNFC_AUTO_SCAN", "LIBNFC_INTRUSIVE_SCAN"} {
		t.Setenv(name, "restore-after-test")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	configureLibNFCDiscovery()
	for _, name := range []string{"LIBNFC_AUTO_SCAN", "LIBNFC_INTRUSIVE_SCAN"} {
		if got := os.Getenv(name); got != "true" {
			t.Fatalf("%s = %q; want true", name, got)
		}
	}
}

func TestConfigureLibNFCDiscoveryPreservesExplicitValues(t *testing.T) {
	t.Setenv("LIBNFC_AUTO_SCAN", "false")
	t.Setenv("LIBNFC_INTRUSIVE_SCAN", "false")
	configureLibNFCDiscovery()
	if got := os.Getenv("LIBNFC_AUTO_SCAN"); got != "false" {
		t.Fatalf("LIBNFC_AUTO_SCAN = %q; want explicit false", got)
	}
	if got := os.Getenv("LIBNFC_INTRUSIVE_SCAN"); got != "false" {
		t.Fatalf("LIBNFC_INTRUSIVE_SCAN = %q; want explicit false", got)
	}
}
