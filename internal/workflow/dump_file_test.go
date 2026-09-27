package workflow_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func completeDumpResult(t *testing.T) workflow.DumpResult {
	t.Helper()
	image, _ := mifare.NewDump(mifare.Classic1K)
	for index := range image.Blocks {
		image.Blocks[index].Data[0] = byte(index)
		image.Blocks[index].KnownMask = mifare.AllBytesKnown
		image.Blocks[index].Status = mifare.BlockRead
		if index%4 == 3 {
			image.Blocks[index].Data = transportTrailer(0xff)
			image.Blocks[index].Status = mifare.BlockSynthetic
			image.Blocks[index].KeyASource = "authenticated_key_a"
			image.Blocks[index].KeyBSource = "card_read"
		}
	}
	image.Blocks[0].Data[0], image.Blocks[0].Data[1], image.Blocks[0].Data[2], image.Blocks[0].Data[3] = 1, 2, 3, 4
	image.Blocks[0].Data[4] = 1 ^ 2 ^ 3 ^ 4
	key := nfc.Key{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	return workflow.DumpResult{
		Dump: image, Card: classic1KCard(1), Device: nfc.DeviceInfo{Name: "mock", ConnString: "mock:test"},
		Keys:      []workflow.VerifiedSectorKeys{{Sector: 0, KeyA: &key}},
		StartedAt: time.Date(2026, 9, 12, 1, 2, 3, 4, time.UTC), FinishedAt: time.Date(2026, 9, 12, 1, 2, 4, 5, time.UTC),
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	want := completeDumpResult(t)
	got, err := workflow.DumpFromMetadata(workflow.MetadataFromDump(want))
	if err != nil {
		t.Fatal(err)
	}
	wantRaw, _ := want.Dump.Raw()
	gotRaw, _ := got.Dump.Raw()
	if string(gotRaw) != string(wantRaw) || got.Card.UID[0] != want.Card.UID[0] || got.Device.ConnString != want.Device.ConnString {
		t.Fatalf("metadata round trip changed dump identity")
	}
	if len(got.Keys) != 1 || got.Keys[0].KeyA == nil || *got.Keys[0].KeyA != *want.Keys[0].KeyA {
		t.Fatalf("metadata round trip keys = %+v", got.Keys)
	}
}

func TestSaveAndLoadRawWithSidecar(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "card.mfd")
	want := completeDumpResult(t)
	sidecar, err := workflow.SaveRawDump(path, want)
	if err != nil {
		t.Fatal(err)
	}
	if sidecar != path+".nfcx.json" {
		t.Fatalf("sidecar = %q", sidecar)
	}
	got, err := workflow.LoadRawDump(path)
	if err != nil || !got.Complete() || len(got.Card.UID) != 4 {
		t.Fatalf("LoadRawDump() complete=%v uid=%x error=%v", got.Complete(), got.Card.UID, err)
	}
	info, err := os.Stat(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("metadata permissions = %v", info.Mode().Perm())
	}
}

func TestPartialProjectRoundTripButRawSaveRejected(t *testing.T) {
	result := completeDumpResult(t)
	result.Dump.Blocks[9].KnownMask = 0
	result.Dump.Blocks[9].Status = mifare.BlockReadFailed
	result.Dump.Blocks[9].Error = "removed"
	if _, err := workflow.SaveRawDump(filepath.Join(t.TempDir(), "partial.bin"), result); !errors.Is(err, mifare.ErrIncompleteDump) {
		t.Fatalf("SaveRawDump(part() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "partial.nfcx.json")
	if err := workflow.SaveDumpProject(path, result); err != nil {
		t.Fatal(err)
	}
	loaded, err := workflow.LoadDumpProject(path)
	if err != nil || loaded.Complete() || loaded.Dump.Blocks[9].Status != mifare.BlockReadFailed {
		t.Fatalf("LoadDumpProject() complete=%v status=%s error=%v", loaded.Complete(), loaded.Dump.Blocks[9].Status, err)
	}
}

func TestLoadRawWithoutMetadataDoesNotGuessUIDLength(t *testing.T) {
	path := filepath.Join(t.TempDir(), "import.bin")
	raw, _ := completeDumpResult(t).Dump.Raw()
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := workflow.LoadRawDump(path)
	if err != nil || len(loaded.Card.UID) != 0 {
		t.Fatalf("LoadRawDump() UID=%x error=%v", loaded.Card.UID, err)
	}
	if status, err := loaded.Dump.ValidateBCC(len(loaded.Card.UID)); status != mifare.BCCNotApplicable || err != nil {
		t.Fatalf("raw-only BCC = %s, %v", status, err)
	}
}

func TestLoadRawRejectsInvalidTrailerStructure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.bin")
	raw, _ := completeDumpResult(t).Dump.Raw()
	raw[3*nfc.BlockSize+6] ^= 1
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.LoadRawDump(path); err == nil {
		t.Fatal("LoadRawDump accepted malformed access bits")
	}
}
