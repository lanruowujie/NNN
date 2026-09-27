//go:build libnfc && cgo && libnfc_hardware

package workflow_test

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/libnfc"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func TestHardwareMIFAREClassicDumpRestore(t *testing.T) {
	if os.Getenv("NFCX_WORKFLOW_HARDWARE") != "1" {
		t.Skip("set NFCX_WORKFLOW_HARDWARE=1 to enable the explicit dump/restore hardware test")
	}
	connString := os.Getenv("LIBNFC_DEVICE")
	if connString == "" {
		t.Fatal("LIBNFC_DEVICE is required")
	}
	keyA := hardwareKey(t, "NFCX_TEST_KEY_A", "FFFFFFFFFFFF")
	var keyB *nfc.Key
	if value := os.Getenv("NFCX_TEST_KEY_B"); value != "" {
		parsed := hardwareKey(t, "NFCX_TEST_KEY_B", "")
		keyB = &parsed
	}

	reader := libnfc.NewReader()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := reader.Open(ctx, connString); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	card, err := reader.CardInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var layout mifare.Layout
	switch nfc.InferCardType(card) {
	case nfc.CardTypeMIFAREClassic1K:
		layout = mifare.Classic1K
	case nfc.CardTypeMIFAREClassic4K:
		layout = mifare.Classic4K
	default:
		t.Fatalf("unsupported hardware card type: %s", nfc.InferCardType(card))
	}
	sectors, _ := layout.SectorCount()
	keys := make([]workflow.SectorKeys, sectors)
	for sector := range keys {
		keys[sector] = workflow.SectorKeys{Sector: sector, KeyA: &keyA, KeyB: keyB}
	}
	executor := directExecutor{reader: reader}
	dump, err := workflow.NewDumpService(executor).ReadCard(ctx, workflow.DumpRequest{
		Card: card, Device: nfc.DeviceInfo{ConnString: connString}, Keys: keys,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dump.Complete() {
		t.Fatal("hardware read is partial; supply every required sector key before restore")
	}
	path := os.Getenv("NFCX_TEST_DUMP_PATH")
	if path == "" {
		path = filepath.Join(t.TempDir(), "hardware-backup.bin")
	}
	if _, err := workflow.SaveRawDump(path, dump); err != nil {
		t.Fatal(err)
	}
	loaded, err := workflow.LoadRawDump(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("NFCX_HARDWARE_RESTORE_CONFIRM") != "I_OWN_THIS_CARD" {
		t.Skip("dump and reload passed; set NFCX_HARDWARE_RESTORE_CONFIRM=I_OWN_THIS_CARD to write the test card")
	}
	if os.Getenv("NFCX_TEST_DUMP_PATH") == "" {
		t.Fatal("NFCX_TEST_DUMP_PATH is required for restore so the recovery dump survives the test")
	}

	result, err := workflow.NewRestoreService(executor).Restore(ctx, workflow.RestoreRequest{
		Card: card, Dump: loaded.Dump, SourceUIDLength: len(loaded.Card.UID), TargetKeys: keys,
	}, nil)
	if err != nil {
		t.Fatalf("restore stopped in phase %s after verified blocks %v: %v", result.Phase, result.VerifiedBlocks, err)
	}
	blocks, _ := layout.BlockCount()
	if len(result.VerifiedBlocks) != blocks-1 || result.Blocks[0].Status != workflow.RestoreSkippedProtected {
		t.Fatalf("verified %d blocks of %d; block 0 status %s", len(result.VerifiedBlocks), blocks-1, result.Blocks[0].Status)
	}
}

func hardwareKey(t *testing.T, environment, fallback string) nfc.Key {
	t.Helper()
	value := os.Getenv(environment)
	if value == "" {
		value = fallback
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("%s must contain 12 hexadecimal characters: %v", environment, err)
	}
	key, err := nfc.KeyFromBytes(decoded)
	if err != nil {
		t.Fatalf("%s: %v", environment, err)
	}
	return key
}
