//go:build darwin || linux

package libnfc

import (
	"errors"
	"strconv"
	"strings"

	"github.com/BennyThink/NFCX/internal/nfc"
	"golang.org/x/sys/unix"
)

// connStringPermissionError distinguishes a known local UART permission
// failure after libnfc reports only that nfc_open returned no device.
func connStringPermissionError(connString string) error {
	const prefix = "pn532_uart:"
	if !strings.HasPrefix(connString, prefix) {
		return nil
	}
	path := strings.TrimPrefix(connString, prefix)
	if separator := strings.LastIndexByte(path, ':'); separator >= 0 {
		if _, err := strconv.ParseUint(path[separator+1:], 10, 32); err == nil {
			path = path[:separator]
		}
	}
	if path == "" {
		return nil
	}
	if err := unix.Access(path, unix.R_OK|unix.W_OK); errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
		return nfc.NewError("open", nfc.CodePermission, "permission denied while opening the PN532 UART device", err)
	}
	return nil
}
