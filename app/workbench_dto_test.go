package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/workbench"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func TestWorkbenchSnapshotShowsTrailerKeys(t *testing.T) {
	image, _ := mifare.NewDump(mifare.Classic1K)
	bits := mifare.AccessBits{}
	bits.Groups[3] = mifare.AccessCondition{C2: true, C3: true}
	encodedAccess := mifare.EncodeAccessBits(bits)
	for index := range image.Blocks {
		image.Blocks[index].KnownMask = mifare.AllBytesKnown
		image.Blocks[index].Status = mifare.BlockRead
		if index%4 == 3 {
			image.Blocks[index].Data = [16]byte{1, 2, 3, 4, 5, 6, encodedAccess[0], encodedAccess[1], encodedAccess[2], 0x69, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6}
		}
	}
	model := workbench.New()
	if err := model.Load(workflow.DumpResult{Dump: image}, ""); err != nil {
		t.Fatal(err)
	}
	modified := image.Blocks[3].Data
	modified[0], modified[10] = 0x09, 0xe1
	if err := model.EditHex(3, strings.ToUpper(fmt.Sprintf("%x", modified))); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(workbenchDTO(model.Snapshot()))
	if err != nil {
		t.Fatal(err)
	}
	encoded := strings.ToUpper(string(payload))
	if !strings.Contains(encoded, "090203040506") || !strings.Contains(encoded, "E1A2A3A4A5A6") {
		t.Fatalf("workbench DTO did not show complete trailer keys: %s", encoded)
	}
	if !strings.Contains(string(payload), `"after":"09"`) || !strings.Contains(string(payload), `"after":"E1"`) {
		t.Fatal("workbench diff did not show edited trailer key bytes")
	}
	dashboardPayload, err := json.Marshal(blockDTOs(model.Snapshot()))
	if err != nil {
		t.Fatal(err)
	}
	dashboardText := strings.ToUpper(string(dashboardPayload))
	if !strings.Contains(dashboardText, "090203040506") || !strings.Contains(dashboardText, "E1A2A3A4A5A6") {
		t.Fatalf("dashboard block DTO did not show complete trailer keys: %s", dashboardText)
	}
}

func TestWorkbenchSavedStateDistinguishesLiveReadFromPersistedFile(t *testing.T) {
	image, _ := mifare.NewDump(mifare.Classic1K)
	model := workbench.New()
	if err := model.Load(workflow.DumpResult{Dump: image}, ""); err != nil {
		t.Fatal(err)
	}
	if dto := workbenchDTO(model.Snapshot()); dto.Saved {
		t.Fatal("live read was incorrectly reported as saved")
	}
	if err := model.MarkSaved("card.bin"); err != nil {
		t.Fatal(err)
	}
	if dto := workbenchDTO(model.Snapshot()); !dto.Saved || dto.Name != "card.bin" {
		t.Fatalf("persisted workbench state = %+v", dto)
	}
}

func TestClearWorkbenchReturnsAndEmitsEmptySnapshot(t *testing.T) {
	model := workbench.New()
	image, err := mifare.NewDump(mifare.Classic1K)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Load(workflow.DumpResult{Dump: image}, "card.bin"); err != nil {
		t.Fatal(err)
	}
	emitter := &orderedEmitter{}
	service := &Service{emitter: emitter, bench: model}

	result := service.ClearWorkbench()
	if result.Loaded || service.bench.Snapshot().Loaded {
		t.Fatalf("clear result = %+v", result)
	}
	if first := emitter.first(); first != WorkbenchEventName {
		t.Fatalf("first event = %q, want %q", first, WorkbenchEventName)
	}
}
