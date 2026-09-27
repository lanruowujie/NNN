package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
)

type mfocTestDevices struct{ reader nfc.Reader }

func (d mfocTestDevices) WithReader(ctx context.Context, operation func(context.Context, nfc.Reader) error) error {
	return operation(ctx, d.reader)
}

func (d mfocTestDevices) WithReleasedDevice(ctx context.Context, hooks device.ReleasedDeviceHooks, operation func(context.Context, device.ReleasedDevice) error) device.ReleasedDeviceResult {
	if hooks.Releasing != nil {
		hooks.Releasing()
	}
	if hooks.Prepare != nil {
		if err := hooks.Prepare(ctx, d.reader); err != nil {
			return device.ReleasedDeviceResult{OperationError: err}
		}
	}
	err := operation(ctx, device.ReleasedDevice{Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Capabilities: device.Capabilities{Nested: true, Hardnested: true}})
	if hooks.Reopening != nil {
		hooks.Reopening()
	}
	return device.ReleasedDeviceResult{OperationError: err}
}

type staticMFoCEngine struct{ raw []byte }

func (e staticMFoCEngine) Name() string                    { return "mfoc" }
func (e staticMFoCEngine) Available(context.Context) error { return nil }
func (e staticMFoCEngine) Run(_ context.Context, _ attack.AttackRequest, _ func(attack.AttackEvent)) (attack.AttackResult, error) {
	return attack.AttackResult{Engine: "mfoc", Version: attack.MFoCVersion, ExitCode: 0, Validated: true, Artifacts: []attack.AttackArtifact{{MediaType: "application/vnd.nfcx.mifare-dump", Data: e.raw}}}, nil
}

func TestMFoCWorkflowExtractsAndRevalidatesEvery1KKey(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{4, 0}, SAK: 0x08}
	raw := mfocTestDump(t, card)
	reader := mock.NewReaderWithFuncs(mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) { return card, nil },
		Authenticate: func(_ context.Context, block byte, keyType nfc.KeyType, _ nfc.Key) error {
			sector, _ := mifare.Classic1K.SectorForBlock(int(block))
			if sector == 3 && keyType == nfc.KeyTypeB {
				return nfc.ErrAuthenticationFailed
			}
			return nil
		},
	})
	if err := reader.Open(context.Background(), "pn532_uart:/dev/test"); err != nil {
		t.Fatal(err)
	}
	store := keys.NewStore()
	seed := nfc.Key{1, 1, 1, 1, 1, 1}
	record, _, _ := store.Add(seed, keys.SourceUserInput)
	if err := store.Verify(keys.CardID(card), 0, nfc.KeyTypeA, record.ID); err != nil {
		t.Fatal(err)
	}
	devices := mfocTestDevices{reader: reader}
	service := NewMFoCService(devices, devices, store, func(attack.MFoCInvocation) attack.AttackEngine { return staticMFoCEngine{raw: raw} })
	result, err := service.Run(context.Background(), MFoCRequest{
		TaskID: "mfoc-test", Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true,
	}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Claims) != 32 || result.Added != 30 || result.Existing != 1 || result.Rejected != 1 || result.Conflicts != 0 {
		t.Fatalf("result = %+v", result)
	}
	if got := len(store.Verified(keys.CardID(card))); got != 31 {
		t.Fatalf("verified slots = %d", got)
	}
}

func TestMFoCWorkflowPreconditions(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	reader := mock.NewReader()
	devices := mfocTestDevices{reader: reader}
	store := keys.NewStore()
	service := NewMFoCService(devices, devices, store, func(attack.MFoCInvocation) attack.AttackEngine { return staticMFoCEngine{} })
	base := MFoCRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}
	if _, err := service.Run(context.Background(), base, nil); !errors.Is(err, ErrMFoCSeedRequired) {
		t.Fatalf("missing seed error = %v", err)
	}
	base.Authorized = false
	if _, err := service.Run(context.Background(), base, nil); !errors.Is(err, ErrMFoCAuthorization) {
		t.Fatalf("authorization error = %v", err)
	}
	base.Authorized = true
	base.Device.ConnString = "acr122_pcsc:test"
	if _, err := service.Run(context.Background(), base, nil); !errors.Is(err, ErrMFoCNested) {
		t.Fatalf("capability error = %v", err)
	}
}

func TestMFoCVerifiedSeedsRetainKeyAAndKeyBSlots(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	store := keys.NewStore()
	keyA, keyB := nfc.Key{1, 2, 3, 4, 5, 6}, nfc.Key{6, 5, 4, 3, 2, 1}
	for _, candidate := range []struct {
		sector  int
		keyType nfc.KeyType
		value   nfc.Key
	}{{1, nfc.KeyTypeA, keyA}, {7, nfc.KeyTypeB, keyB}} {
		record, _, _ := store.Add(candidate.value, keys.SourceUserInput)
		if err := store.Verify(keys.CardID(card), candidate.sector, candidate.keyType, record.ID); err != nil {
			t.Fatal(err)
		}
	}
	service := &MFoCService{keys: store}
	seeds := service.verifiedSeeds(card, mifare.Classic1K)
	if len(seeds) != 2 || seeds[0].Sector != 1 || seeds[0].Type != nfc.KeyTypeA || seeds[1].Sector != 7 || seeds[1].Type != nfc.KeyTypeB {
		t.Fatalf("seeds = %+v", seeds)
	}
}

func mfocTestDump(t *testing.T, card nfc.CardInfo) []byte {
	t.Helper()
	image, _ := mifare.NewDump(mifare.Classic1K)
	for block := range image.Blocks {
		image.Blocks[block].KnownMask = mifare.AllBytesKnown
		image.Blocks[block].Status = mifare.BlockRead
	}
	copy(image.Blocks[0].Data[:4], card.UID)
	image.Blocks[0].Data[4] = card.UID[0] ^ card.UID[1] ^ card.UID[2] ^ card.UID[3]
	for sector := 0; sector < mifare.Classic1KSectors; sector++ {
		trailer, _ := mifare.Classic1K.TrailerBlock(sector)
		for index := 0; index < 6; index++ {
			image.Blocks[trailer].Data[index] = byte(sector + 1)
			image.Blocks[trailer].Data[10+index] = byte(0xa0 + sector)
		}
		copy(image.Blocks[trailer].Data[6:10], []byte{0xff, 0x07, 0x80, 0x69})
	}
	raw, err := image.Raw()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
