//go:build darwin || linux

package libnfc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/BennyThink/NFCX/internal/nfc"
)

func TestConnStringPermissionErrorForInaccessibleUART(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tty-nfcx")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	err := connStringPermissionError("pn532_uart:" + path + ":115200")
	if !errors.Is(err, nfc.ErrPermission) {
		t.Fatalf("permission check error = %v; want ErrPermission", err)
	}
}

func TestConnStringPermissionErrorIgnoresMissingPathAndOtherDrivers(t *testing.T) {
	if err := connStringPermissionError("pn532_uart:/definitely/not/a/device"); err != nil {
		t.Fatalf("missing path should remain a native open error: %v", err)
	}
	if err := connStringPermissionError("pn53x_usb:001:002"); err != nil {
		t.Fatalf("non-UART driver should remain a native open error: %v", err)
	}
}
