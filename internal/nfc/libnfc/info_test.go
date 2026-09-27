package libnfc_test

import (
	"testing"

	"github.com/BennyThink/NFCX/internal/nfc/libnfc"
)

func TestInfoAlwaysReportsDeclaredDriverSet(t *testing.T) {
	info := libnfc.Info()
	if len(info.Drivers) != 1 || info.Drivers[0] != "pn532_uart" {
		t.Fatalf("drivers = %v", info.Drivers)
	}
}
