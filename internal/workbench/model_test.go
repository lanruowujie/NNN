package workbench

import (
	"testing"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func workbenchResult(t *testing.T) workflow.DumpResult {
	t.Helper()
	image, err := mifare.NewDump(mifare.Classic1K)
	if err != nil {
		t.Fatal(err)
	}
	for index := range image.Blocks {
		image.Blocks[index].KnownMask = mifare.AllBytesKnown
		image.Blocks[index].Status = mifare.BlockRead
		if index%4 == 3 {
			image.Blocks[index].Data[6] = 0xff
			image.Blocks[index].Data[7] = 0x07
			image.Blocks[index].Data[8] = 0x80
			image.Blocks[index].Data[9] = 0x69
		}
	}
	return workflow.DumpResult{Dump: image}
}

func TestHexASCIIRoundTripDirtyAndUndo(t *testing.T) {
	model := New()
	if err := model.Load(workbenchResult(t), "test.bin"); err != nil {
		t.Fatal(err)
	}
	if err := model.EditASCII(1, "0123456789ABCDEF"); err != nil {
		t.Fatal(err)
	}
	snapshot := model.Snapshot()
	if !snapshot.Dirty || len(snapshot.Diffs) != 1 || snapshot.Result.Dump.Blocks[1].Hex() != "30313233343536373839414243444546" {
		t.Fatalf("edited snapshot = %+v", snapshot)
	}
	if got := ASCII(snapshot.Result.Dump.Blocks[1].Data); got != "0123456789ABCDEF" {
		t.Fatalf("ASCII = %q", got)
	}
	if err := model.Undo(); err != nil {
		t.Fatal(err)
	}
	if model.Snapshot().Dirty {
		t.Fatal("undo did not clear dirty state")
	}
}

func TestASCIIEditPadsShortInputWithZeroBytes(t *testing.T) {
	model := New()
	if err := model.Load(workbenchResult(t), "test.bin"); err != nil {
		t.Fatal(err)
	}
	if err := model.EditASCII(1, "NFCX"); err != nil {
		t.Fatal(err)
	}
	if got := model.Snapshot().Result.Dump.Blocks[1].Hex(); got != "4e464358000000000000000000000000" {
		t.Fatalf("short ASCII edit = %s", got)
	}
	for _, value := range []string{"0123456789ABCDEFG", "中文"} {
		if err := model.EditASCII(1, value); err == nil {
			t.Fatalf("EditASCII(%q) unexpectedly succeeded", value)
		}
	}
}

func TestReloadAndMarkSavedClearDirty(t *testing.T) {
	model := New()
	result := workbenchResult(t)
	if err := model.Load(result, "one.bin"); err != nil {
		t.Fatal(err)
	}
	if err := model.EditHex(2, "0102030405060708090A0B0C0D0E0F10"); err != nil {
		t.Fatal(err)
	}
	if err := model.MarkSaved("two.bin"); err != nil {
		t.Fatal(err)
	}
	if snapshot := model.Snapshot(); snapshot.Dirty || snapshot.Path != "two.bin" {
		t.Fatalf("saved snapshot = %+v", snapshot)
	}
	if err := model.EditHex(2, "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"); err != nil {
		t.Fatal(err)
	}
	if err := model.Load(result, "one.bin"); err != nil {
		t.Fatal(err)
	}
	if model.Snapshot().Dirty {
		t.Fatal("reload did not clear dirty state")
	}
}

func TestKindsAndTrailerValidation(t *testing.T) {
	if KindForBlock(mifare.Classic1K, 0) != BlockManufacturer || KindForBlock(mifare.Classic1K, 3) != BlockTrailer || KindForBlock(mifare.Classic1K, 1) != BlockData {
		t.Fatal("block kinds are incorrect")
	}
	model := New()
	if err := model.Load(workbenchResult(t), ""); err != nil {
		t.Fatal(err)
	}
	if err := model.EditHex(3, "FFFFFFFFFFFF00000069FFFFFFFFFFFF"); err != nil {
		t.Fatal(err)
	}
	if validation := model.ValidateForSave(); validation.Valid || len(validation.Errors) == 0 {
		t.Fatalf("invalid trailer accepted: %+v", validation)
	}
}
