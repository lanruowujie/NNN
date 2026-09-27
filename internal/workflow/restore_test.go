package workflow_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func restoreHarness(t *testing.T, failBlock int) (*workflow.RestoreService, workflow.RestoreRequest, *[]int) {
	t.Helper()
	card := classic1KCard(1)
	key := nfc.Key{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	result := completeDumpResult(t)
	blocks := make(map[byte][nfc.BlockSize]byte, len(result.Dump.Blocks))
	for index := range result.Dump.Blocks {
		blocks[byte(index)] = result.Dump.Blocks[index].Data
	}
	writes := make([]int, 0)
	reader := openMockReader(t, mock.ReaderFuncs{
		CardInfo:     func(context.Context) (nfc.CardInfo, error) { return card, nil },
		Authenticate: func(context.Context, byte, nfc.KeyType, nfc.Key) error { return nil },
		ReadBlock: func(_ context.Context, block byte) ([nfc.BlockSize]byte, error) {
			data := blocks[block]
			if block%4 == 3 {
				for index := 0; index < 6; index++ {
					data[index] = 0
				}
			}
			return data, nil
		},
		WriteBlock: func(_ context.Context, block byte, data [nfc.BlockSize]byte) error {
			writes = append(writes, int(block))
			if int(block) == failBlock {
				return nfc.NewError("write block", nfc.CodeIO, "injected failure", nil)
			}
			blocks[block] = data
			return nil
		},
		WriteTrailer: func(_ context.Context, block byte, data [nfc.BlockSize]byte) error {
			writes = append(writes, int(block))
			if int(block) == failBlock {
				return nfc.NewError("write trailer", nfc.CodeIO, "injected failure", nil)
			}
			blocks[block] = data
			return nil
		},
	})
	keys := make([]workflow.SectorKeys, mifare.Classic1KSectors)
	for sector := range keys {
		keys[sector] = workflow.SectorKeys{Sector: sector, KeyA: &key}
	}
	request := workflow.RestoreRequest{Card: card, Dump: result.Dump, SourceUIDLength: 4, TargetKeys: keys}
	return workflow.NewRestoreService(directExecutor{reader: reader}), request, &writes
}

func TestRestoreWritesAllDataBeforeAnyTrailerAndSkipsBlockZero(t *testing.T) {
	service, request, writes := restoreHarness(t, -1)
	result, err := service.Restore(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Phase != workflow.RestoreComplete || result.Blocks[0].Status != workflow.RestoreSkippedProtected {
		t.Fatalf("restore phase/block0 = %s/%s", result.Phase, result.Blocks[0].Status)
	}
	if len(*writes) != 63 || (*writes)[0] != 1 || (*writes)[46] != 62 || (*writes)[47] != 3 || (*writes)[62] != 63 {
		t.Fatalf("write plan = %v", *writes)
	}
	for _, block := range *writes {
		if block == 0 {
			t.Fatal("restore attempted block 0")
		}
	}
	if len(result.WrittenBlocks) != 63 || len(result.VerifiedBlocks) != 63 || len(result.UnwrittenBlocks) != 0 {
		t.Fatalf("result counts written=%d verified=%d unwritten=%d", len(result.WrittenBlocks), len(result.VerifiedBlocks), len(result.UnwrittenBlocks))
	}
}

func TestRestorePreflightRejectsBadAccessBitsWithoutWrites(t *testing.T) {
	service, request, writes := restoreHarness(t, -1)
	request.Dump.Blocks[7].Data[6] ^= 1
	result, err := service.Restore(context.Background(), request, nil)
	if err == nil || len(*writes) != 0 || result.Phase != workflow.RestoreFailed {
		t.Fatalf("Restore() writes=%v phase=%s error=%v", *writes, result.Phase, err)
	}
}

func TestRestorePreflightRejectsBadFourByteUIDBCCWithoutWrites(t *testing.T) {
	service, request, writes := restoreHarness(t, -1)
	request.Dump.Blocks[0].Data[4] ^= 1
	_, err := service.Restore(context.Background(), request, nil)
	if err == nil || len(*writes) != 0 {
		t.Fatalf("Restore() writes=%v error=%v", *writes, err)
	}
}

func TestRestorePreflightRejectsPartialDumpWithoutWrites(t *testing.T) {
	service, request, writes := restoreHarness(t, -1)
	request.Dump.Blocks[12].KnownMask = 0
	_, err := service.Restore(context.Background(), request, nil)
	if err == nil || len(*writes) != 0 {
		t.Fatalf("Restore() writes=%v error=%v", *writes, err)
	}
}

func TestRestoreStopsAndReportsWrittenAndUnwrittenBlocks(t *testing.T) {
	service, request, writes := restoreHarness(t, 5)
	result, err := service.Restore(context.Background(), request, nil)
	if !errors.Is(err, nfc.ErrIO) {
		t.Fatalf("Restore() error = %v; want ErrIO", err)
	}
	if !reflect.DeepEqual(*writes, []int{1, 2, 4, 5}) {
		t.Fatalf("writes = %v", *writes)
	}
	if !reflect.DeepEqual(result.WrittenBlocks, []int{1, 2, 4}) || !reflect.DeepEqual(result.VerifiedBlocks, []int{1, 2, 4}) {
		t.Fatalf("written=%v verified=%v", result.WrittenBlocks, result.VerifiedBlocks)
	}
	if result.FailedBlock == nil || *result.FailedBlock != 5 || result.Blocks[5].Status != workflow.RestoreWriteOutcomeUnknown || result.Blocks[5].WriteOutcomeKnown {
		t.Fatalf("failed block/status = %v/%s", result.FailedBlock, result.Blocks[5].Status)
	}
	if len(result.UnwrittenBlocks) != 59 {
		t.Fatalf("unwritten count = %d; want 59", len(result.UnwrittenBlocks))
	}
}

func TestPreflightReturnsDeterministicProtectedPlan(t *testing.T) {
	service, request, _ := restoreHarness(t, -1)
	plan, err := service.Preflight(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 63 || plan.Operations[0].Block != 1 || plan.Operations[47].Block != 3 || !plan.Operations[47].Trailer {
		t.Fatalf("plan operations = %d first=%+v first trailer=%+v", len(plan.Operations), plan.Operations[0], plan.Operations[47])
	}
}

func TestRestoreCanLimitWritesToDirtyBlocks(t *testing.T) {
	service, request, writes := restoreHarness(t, -1)
	request.Blocks = []int{1, 3, 4}
	result, err := service.Restore(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := *writes, []int{1, 4, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("writes = %v, want %v", got, want)
	}
	if result.Blocks[2].Status != workflow.RestoreSkippedUnchanged || result.Blocks[0].Status != workflow.RestoreSkippedProtected {
		t.Fatalf("unselected/protected status = %s/%s", result.Blocks[2].Status, result.Blocks[0].Status)
	}
}
