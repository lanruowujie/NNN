package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
)

type recoveryScannerStub struct {
	run func(context.Context, KeyScanRequest, func(KeyScanProgress)) (KeyScanResult, error)
}

func (s recoveryScannerStub) Scan(ctx context.Context, request KeyScanRequest, emit func(KeyScanProgress)) (KeyScanResult, error) {
	return s.run(ctx, request, emit)
}

type recoveryMFCUKStub struct {
	run func(context.Context, MFCUKPipelineRequest, func(attack.AttackEvent)) (MFCUKPipelineResult, error)
}

func (s recoveryMFCUKStub) Run(ctx context.Context, request MFCUKPipelineRequest, emit func(attack.AttackEvent)) (MFCUKPipelineResult, error) {
	return s.run(ctx, request, emit)
}

type recoveryMFoCStub struct {
	run func(context.Context, MFoCRequest, func(attack.AttackEvent)) (MFoCResult, error)
}

type recoveryHardnestedStub struct {
	available error
	run       func(context.Context, HardnestedRequest, func(attack.AttackEvent)) (HardnestedResult, error)
}

func (s recoveryHardnestedStub) Available(context.Context) error { return s.available }
func (s recoveryHardnestedStub) Run(ctx context.Context, request HardnestedRequest, emit func(attack.AttackEvent)) (HardnestedResult, error) {
	return s.run(ctx, request, emit)
}

func (s recoveryMFoCStub) Run(ctx context.Context, request MFoCRequest, emit func(attack.AttackEvent)) (MFoCResult, error) {
	return s.run(ctx, request, emit)
}

type recoveryReaderStub struct {
	run func(context.Context, DumpRequest) (DumpResult, error)
}

func (s recoveryReaderStub) ReadCard(ctx context.Context, request DumpRequest) (DumpResult, error) {
	return s.run(ctx, request)
}

func TestKeyRecoverySkipsAttacksWhenCommonKeysCoverCard(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 0x08}
	store := keys.NewStore()
	attackCalled := false
	service := &KeyRecoveryService{
		keys: store,
		scanner: recoveryScannerStub{run: func(_ context.Context, _ KeyScanRequest, _ func(KeyScanProgress)) (KeyScanResult, error) {
			for sector := 0; sector < mifare.Classic1KSectors; sector++ {
				addRecoveryVerified(t, store, card, sector, nfc.KeyTypeA)
			}
			return KeyScanResult{Card: card}, nil
		}},
		mfcuk: recoveryMFCUKStub{run: func(context.Context, MFCUKPipelineRequest, func(attack.AttackEvent)) (MFCUKPipelineResult, error) {
			attackCalled = true
			return MFCUKPipelineResult{}, nil
		}},
		mfoc: recoveryMFoCStub{run: func(context.Context, MFoCRequest, func(attack.AttackEvent)) (MFoCResult, error) {
			attackCalled = true
			return MFoCResult{}, nil
		}},
		reader: recoveryReaderStub{run: func(context.Context, DumpRequest) (DumpResult, error) {
			return recoveryReadResult(t, card, mifare.Classic1KSectors), nil
		}},
	}
	result, err := service.Run(context.Background(), KeyRecoveryRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}, nil)
	if err != nil || result.Outcome != KeyRecoveryComplete || result.ReadableSectors != mifare.Classic1KSectors {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if attackCalled {
		t.Fatal("external attack ran despite complete common-key coverage")
	}
	want := []KeyRecoveryStepStatus{KeyRecoveryCompleted, KeyRecoverySkipped, KeyRecoverySkipped, KeyRecoverySkipped, KeyRecoveryCompleted}
	for index, status := range want {
		if result.Steps[index].Status != status {
			t.Fatalf("step %d=%s want %s", index+1, result.Steps[index].Status, status)
		}
	}
}

func TestKeyRecoveryUsesNestedForPartialSeedAndReturnsPartialResult(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 0x08}
	store := keys.NewStore()
	mfcukCalled, mfocCalled := false, false
	service := &KeyRecoveryService{
		keys: store,
		scanner: recoveryScannerStub{run: func(_ context.Context, _ KeyScanRequest, _ func(KeyScanProgress)) (KeyScanResult, error) {
			addRecoveryVerified(t, store, card, 0, nfc.KeyTypeB)
			return KeyScanResult{Card: card}, nil
		}},
		mfcuk: recoveryMFCUKStub{run: func(context.Context, MFCUKPipelineRequest, func(attack.AttackEvent)) (MFCUKPipelineResult, error) {
			mfcukCalled = true
			return MFCUKPipelineResult{}, nil
		}},
		mfoc: recoveryMFoCStub{run: func(context.Context, MFoCRequest, func(attack.AttackEvent)) (MFoCResult, error) {
			mfocCalled = true
			return MFoCResult{}, errors.New("nested nonce is not exploitable")
		}},
		reader: recoveryReaderStub{run: func(context.Context, DumpRequest) (DumpResult, error) {
			return recoveryReadResult(t, card, 1), nil
		}},
	}
	result, err := service.Run(context.Background(), KeyRecoveryRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}, nil)
	if err != nil || result.Outcome != KeyRecoveryPartial || result.ReadableSectors != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if mfcukCalled || !mfocCalled {
		t.Fatalf("mfcukCalled=%v mfocCalled=%v", mfcukCalled, mfocCalled)
	}
	if result.Steps[2].Status != KeyRecoveryFailed || result.Steps[3].Status != KeyRecoveryUnavailable {
		t.Fatalf("steps=%+v", result.Steps)
	}
	if result.FailedStep != 3 || result.FailedStage != KeyRecoveryNested || result.Reason != "nested nonce is not exploitable" {
		t.Fatalf("failure detail=%d/%s %q", result.FailedStep, result.FailedStage, result.Reason)
	}
}

func TestKeyRecoveryUsesDarksideThenNestedWhenScanFindsNoSeed(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 0x08}
	store := keys.NewStore()
	mfcukCalled, directMFoCCalled := false, false
	service := &KeyRecoveryService{
		keys: store,
		scanner: recoveryScannerStub{run: func(_ context.Context, _ KeyScanRequest, _ func(KeyScanProgress)) (KeyScanResult, error) {
			return KeyScanResult{Card: card}, nil
		}},
		mfcuk: recoveryMFCUKStub{run: func(_ context.Context, _ MFCUKPipelineRequest, emit func(attack.AttackEvent)) (MFCUKPipelineResult, error) {
			mfcukCalled = true
			emit(attack.AttackEvent{Engine: "mfcuk", State: attack.StateRunning, Message: "darkside progress"})
			for sector := 0; sector < mifare.Classic1KSectors; sector++ {
				addRecoveryVerified(t, store, card, sector, nfc.KeyTypeA)
			}
			return MFCUKPipelineResult{Verified: 1}, nil
		}},
		mfoc: recoveryMFoCStub{run: func(context.Context, MFoCRequest, func(attack.AttackEvent)) (MFoCResult, error) {
			directMFoCCalled = true
			return MFoCResult{}, nil
		}},
		reader: recoveryReaderStub{run: func(context.Context, DumpRequest) (DumpResult, error) {
			return recoveryReadResult(t, card, mifare.Classic1KSectors), nil
		}},
	}
	result, err := service.Run(context.Background(), KeyRecoveryRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}, nil)
	if err != nil || result.Outcome != KeyRecoveryComplete || !mfcukCalled || directMFoCCalled {
		t.Fatalf("result=%+v err=%v mfcuk=%v directMFoC=%v", result, err, mfcukCalled, directMFoCCalled)
	}
	if result.Steps[1].Status != KeyRecoveryCompleted || result.Steps[2].Status != KeyRecoveryCompleted {
		t.Fatalf("steps=%+v", result.Steps)
	}
}

func TestKeyRecoveryFailsAtDarksideWithoutVerifiedCandidate(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 0x08}
	store := keys.NewStore()
	service := &KeyRecoveryService{
		keys: store,
		scanner: recoveryScannerStub{run: func(_ context.Context, _ KeyScanRequest, _ func(KeyScanProgress)) (KeyScanResult, error) {
			return KeyScanResult{Card: card}, nil
		}},
		mfcuk: recoveryMFCUKStub{run: func(context.Context, MFCUKPipelineRequest, func(attack.AttackEvent)) (MFCUKPipelineResult, error) {
			return MFCUKPipelineResult{}, errors.New("darkside timeout")
		}},
		reader: recoveryReaderStub{run: func(context.Context, DumpRequest) (DumpResult, error) {
			t.Fatal("readiness must not run without a verified key")
			return DumpResult{}, nil
		}},
	}
	result, err := service.Run(context.Background(), KeyRecoveryRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}, nil)
	if err == nil || result.Outcome != KeyRecoveryFailure || result.FailedStep != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestKeyRecoveryMarksDarksideCompleteBeforeNestedStarts(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 0x08}
	store := keys.NewStore()
	var events []KeyRecoveryEvent
	service := &KeyRecoveryService{
		keys: store,
		scanner: recoveryScannerStub{run: func(_ context.Context, _ KeyScanRequest, _ func(KeyScanProgress)) (KeyScanResult, error) {
			return KeyScanResult{Card: card}, nil
		}},
		mfcuk: recoveryMFCUKStub{run: func(_ context.Context, _ MFCUKPipelineRequest, emit func(attack.AttackEvent)) (MFCUKPipelineResult, error) {
			addRecoveryVerified(t, store, card, 0, nfc.KeyTypeA)
			emit(attack.AttackEvent{Engine: "mfcuk", State: attack.StateMFoCStage, Message: "starting nested"})
			return MFCUKPipelineResult{Verified: 1}, errors.New("nested failed")
		}},
		reader: recoveryReaderStub{run: func(context.Context, DumpRequest) (DumpResult, error) {
			return recoveryReadResult(t, card, 1), nil
		}},
	}
	result, err := service.Run(context.Background(), KeyRecoveryRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}, func(event KeyRecoveryEvent) {
		events = append(events, event)
	})
	if err != nil || result.Outcome != KeyRecoveryPartial {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	darksideCompleted, nestedRunning := -1, -1
	for index, event := range events {
		if event.Step == 2 && event.Status == KeyRecoveryCompleted {
			darksideCompleted = index
		}
		if event.Step == 3 && event.Status == KeyRecoveryRunning {
			nestedRunning = index
			break
		}
	}
	if darksideCompleted < 0 || nestedRunning < 0 || darksideCompleted >= nestedRunning {
		t.Fatalf("darksideCompleted=%d nestedRunning=%d events=%+v", darksideCompleted, nestedRunning, events)
	}
}

func TestKeyRecoveryPropagatesCancellationAfterDarksideSeed(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 0x08}
	store := keys.NewStore()
	service := &KeyRecoveryService{
		keys: store,
		scanner: recoveryScannerStub{run: func(_ context.Context, _ KeyScanRequest, _ func(KeyScanProgress)) (KeyScanResult, error) {
			return KeyScanResult{Card: card}, nil
		}},
		mfcuk: recoveryMFCUKStub{run: func(_ context.Context, _ MFCUKPipelineRequest, emit func(attack.AttackEvent)) (MFCUKPipelineResult, error) {
			addRecoveryVerified(t, store, card, 0, nfc.KeyTypeA)
			emit(attack.AttackEvent{Engine: "mfoc", State: attack.StateMFoCStage, Message: "nested running"})
			return MFCUKPipelineResult{Verified: 1}, attack.ErrCancelled
		}},
		reader: recoveryReaderStub{run: func(context.Context, DumpRequest) (DumpResult, error) {
			t.Fatal("readiness must not run after cancellation")
			return DumpResult{}, nil
		}},
	}
	result, err := service.Run(context.Background(), KeyRecoveryRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}, nil)
	if !errors.Is(err, attack.ErrCancelled) || result.FailedStep != 3 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestKeyRecoveryUsesHardnestedAfterNestedFailure(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 0x08}
	store := keys.NewStore()
	hardnestedCalled := false
	service := &KeyRecoveryService{
		keys: store,
		scanner: recoveryScannerStub{run: func(_ context.Context, _ KeyScanRequest, _ func(KeyScanProgress)) (KeyScanResult, error) {
			addRecoveryVerified(t, store, card, 0, nfc.KeyTypeA)
			return KeyScanResult{Card: card}, nil
		}},
		mfoc: recoveryMFoCStub{run: func(context.Context, MFoCRequest, func(attack.AttackEvent)) (MFoCResult, error) {
			return MFoCResult{}, errors.New("nested nonce is hardened")
		}},
		hardnested: recoveryHardnestedStub{run: func(context.Context, HardnestedRequest, func(attack.AttackEvent)) (HardnestedResult, error) {
			hardnestedCalled = true
			for sector := 1; sector < mifare.Classic1KSectors; sector++ {
				addRecoveryVerified(t, store, card, sector, nfc.KeyTypeA)
			}
			return HardnestedResult{}, nil
		}},
		reader: recoveryReaderStub{run: func(context.Context, DumpRequest) (DumpResult, error) {
			return recoveryReadResult(t, card, mifare.Classic1KSectors), nil
		}},
	}
	result, err := service.Run(context.Background(), KeyRecoveryRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}, nil)
	if err != nil || result.Outcome != KeyRecoveryComplete || !hardnestedCalled || !result.HardnestedAvailable {
		t.Fatalf("result=%+v err=%v hardnestedCalled=%v", result, err, hardnestedCalled)
	}
	if result.Steps[2].Status != KeyRecoveryFailed || result.Steps[3].Status != KeyRecoveryCompleted || result.FailedStep != 0 {
		t.Fatalf("steps=%+v failure=%d/%s %q", result.Steps, result.FailedStep, result.FailedStage, result.Reason)
	}
}

func addRecoveryVerified(t *testing.T, store *keys.Store, card nfc.CardInfo, sector int, keyType nfc.KeyType) {
	t.Helper()
	value := nfc.Key{byte(sector + 1), 2, 3, 4, 5, byte(keyType + 1)}
	record, _, err := store.Add(value, keys.SourceUserInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(keys.CardID(card), sector, keyType, record.ID); err != nil {
		t.Fatal(err)
	}
}

func recoveryReadResult(t *testing.T, card nfc.CardInfo, readableSectors int) DumpResult {
	t.Helper()
	image, err := mifare.NewDump(mifare.Classic1K)
	if err != nil {
		t.Fatal(err)
	}
	for sector := 0; sector < readableSectors; sector++ {
		first, _ := image.Layout.FirstBlock(sector)
		count, _ := image.Layout.BlocksInSector(sector)
		for block := first; block < first+count; block++ {
			image.Blocks[block].KnownMask = mifare.AllBytesKnown
		}
	}
	return DumpResult{Card: card, Dump: image}
}
