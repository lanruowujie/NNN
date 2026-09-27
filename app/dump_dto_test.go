package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func TestDumpResultDTOReportsKeyStatusWithoutKeyMaterial(t *testing.T) {
	image, _ := mifare.NewDump(mifare.Classic1K)
	key := nfc.Key{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	result := workflow.DumpResult{Dump: image, Card: nfc.CardInfo{UID: []byte{1, 2, 3, 4}}, Keys: []workflow.VerifiedSectorKeys{{Sector: 0, KeyA: &key}}}
	payload, err := json.Marshal(dumpResultDTO(result))
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if !strings.Contains(text, `"keyAStatus":"verified"`) || !strings.Contains(text, `"knownMask":"0000"`) {
		t.Fatalf("dump DTO = %s", text)
	}
	if strings.Contains(strings.ToUpper(text), "FFFFFFFFFFFF") || strings.Contains(text, "255,255,255") {
		t.Fatal("dump DTO exposed complete key material")
	}
}

func TestRestoreResultDTOIncludesAuditableBlockLists(t *testing.T) {
	failed := 5
	dto := restoreResultDTO(workflow.RestoreResult{
		Phase:         workflow.RestoreFailed,
		Blocks:        []workflow.RestoreBlockResult{{Block: 1, Status: workflow.RestoreVerified, Written: true, Verified: true}},
		WrittenBlocks: []int{1}, VerifiedBlocks: []int{1}, UnwrittenBlocks: []int{5, 6}, FailedBlock: &failed,
	})
	payload, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"writtenBlocks":[1]`, `"verifiedBlocks":[1]`, `"unwrittenBlocks":[5,6]`, `"failedBlock":5`} {
		if !strings.Contains(string(payload), field) {
			t.Errorf("restore DTO missing %s: %s", field, payload)
		}
	}
}
