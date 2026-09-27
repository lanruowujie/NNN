package workbench

import (
	"testing"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func TestMergeReadResultFillsUnknownAndPreservesUserEdits(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{4, 0}, SAK: 0x08}
	partial, _ := mifare.NewDump(mifare.Classic1K)
	partial.Blocks[1].Data[0] = 0x11
	partial.Blocks[1].KnownMask = 1
	model := New()
	if err := model.Load(workflow.DumpResult{Dump: partial, Card: card}, "partial.nfcx.json"); err != nil {
		t.Fatal(err)
	}
	if err := model.EditHex(2, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"); err != nil {
		t.Fatal(err)
	}
	fresh, _ := mifare.NewDump(mifare.Classic1K)
	for block := range fresh.Blocks {
		for offset := range fresh.Blocks[block].Data {
			fresh.Blocks[block].Data[offset] = 0x22
		}
		fresh.Blocks[block].KnownMask = mifare.AllBytesKnown
		fresh.Blocks[block].Status = mifare.BlockRead
	}
	report, err := model.MergeReadResult(workflow.DumpResult{Dump: fresh, Card: card})
	if err != nil {
		t.Fatal(err)
	}
	if report.FilledBytes == 0 || len(report.Conflicts) == 0 {
		t.Fatalf("report = %+v", report)
	}
	snapshot := model.Snapshot()
	if !snapshot.Dirty || snapshot.Result.Dump.Blocks[1].Data[0] != 0x11 || snapshot.Result.Dump.Blocks[1].Data[1] != 0x22 || snapshot.Result.Dump.Blocks[2].Data[0] != 0xaa {
		t.Fatalf("merge did not preserve known data and edits: %+v", snapshot)
	}
	if err := model.Undo(); err != nil {
		t.Fatal(err)
	}
	afterUndo := model.Snapshot()
	if afterUndo.Result.Dump.Blocks[2].Data[0] != 0x22 {
		t.Fatalf("undo discarded freshly merged background data: %02X", afterUndo.Result.Dump.Blocks[2].Data[0])
	}
}

func TestMergeReadResultRejectsTruncatedBlockList(t *testing.T) {
	model := New()
	image, err := mifare.NewDump(mifare.Classic1K)
	if err != nil {
		t.Fatal(err)
	}
	image.Blocks = image.Blocks[:1]
	if _, err := model.MergeReadResult(workflow.DumpResult{Dump: image}); err == nil {
		t.Fatal("expected truncated block list to be rejected")
	}
}
