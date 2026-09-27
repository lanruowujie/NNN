package attack

import (
	"context"
	"strings"
	"testing"

	"github.com/BennyThink/NFCX/internal/nfc"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

func TestMFSetUIDEngineRejectsNonPN532ReaderBeforeProcess(t *testing.T) {
	engine := NewMFSetUIDEngine(runtimebundle.NewLocator(t.TempDir()), MFSetUIDInvocation{
		Card: nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{4, 0}, SAK: 0x08},
	})
	_, err := engine.Run(context.Background(), AttackRequest{
		Device: nfc.DeviceInfo{ConnString: "acr122_pcsc:test"}, WorkingDir: t.TempDir(),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "PN532 UART") {
		t.Fatalf("error = %v", err)
	}
}

func TestMFSetUIDFullBlockArgumentPreservesManufacturerBytes(t *testing.T) {
	block := [nfc.BlockSize]byte{1, 2, 3, 4, 4, 0x08, 0x04, 0x00, 0x62, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69}
	arguments := mfsetuidArguments(block)
	if len(arguments) != 1 || arguments[0] != "01020304040804006263646566676869" {
		t.Fatalf("arguments = %#v", arguments)
	}
}
