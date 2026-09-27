//go:build !libnfc || !cgo

package libnfc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/libnfc"
)

func TestDefaultBuildReportsUnavailableNativeSupport(t *testing.T) {
	backend := libnfc.NewBackend()
	if _, err := backend.ListDevices(context.Background()); !errors.Is(err, nfc.ErrUnsupported) {
		t.Fatalf("ListDevices() error = %v; want ErrUnsupported", err)
	}
	reader := backend.NewReader()
	if err := reader.Open(context.Background(), "pn532_uart:test"); !errors.Is(err, nfc.ErrUnsupported) {
		t.Fatalf("Open() error = %v; want ErrUnsupported", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
