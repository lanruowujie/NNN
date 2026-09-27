//go:build libnfc && cgo

package libnfc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/BennyThink/NFCX/internal/nfc"
)

func TestCGOInitializeAndExit(t *testing.T) {
	before := nativeResources()
	if result := nativeSmoke(); result.code != nativeOK {
		t.Fatalf("nativeSmoke() = %+v", result)
	}
	if after := nativeResources(); after != before {
		t.Fatalf("resources after smoke = %+v; want %+v", after, before)
	}
}

func TestCGODiscoveryDefaults(t *testing.T) {
	if result := nativeConfigureDiscoveryDefaults(); result.code != nativeOK {
		t.Fatalf("nativeConfigureDiscoveryDefaults() = %+v", result)
	}
}

func TestCGOOpenFailureReleasesResources(t *testing.T) {
	reader := NewReader()
	before := nativeResources()
	connString := strings.Repeat("x", 1024)
	err := reader.Open(context.Background(), connString)
	if !errors.Is(err, nfc.ErrInvalidArgument) {
		t.Fatalf("Open() error = %v; want ErrInvalidArgument", err)
	}
	if after := nativeResources(); after != before {
		t.Fatalf("resources after failed open = %+v; want %+v", after, before)
	}

	err = reader.Open(context.Background(), "not_a_real_libnfc_driver:test")
	if !errors.Is(err, nfc.ErrDeviceDisconnected) {
		t.Fatalf("Open() unknown driver error = %v; want ErrDeviceDisconnected", err)
	}
	if after := nativeResources(); after != before {
		t.Fatalf("resources after native open failure = %+v; want %+v", after, before)
	}
}
