package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
)

type staticHardnestedEngine struct{ raw []byte }

func (e staticHardnestedEngine) Name() string                    { return "mfoc-hardnested" }
func (e staticHardnestedEngine) Available(context.Context) error { return nil }
func (e staticHardnestedEngine) Run(_ context.Context, _ attack.AttackRequest, _ func(attack.AttackEvent)) (attack.AttackResult, error) {
	return attack.AttackResult{Engine: "mfoc-hardnested", Version: attack.HardnestedVersion, ExitCode: 0, Validated: true, Artifacts: []attack.AttackArtifact{{MediaType: "application/vnd.nfcx.mifare-dump", Data: e.raw}}}, nil
}

func TestHardnestedWorkflowRevalidatesRecoveredKeys(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{4, 0}, SAK: 0x08}
	raw := mfocTestDump(t, card)
	reader := mock.NewReaderWithFuncs(mock.ReaderFuncs{
		CardInfo:     func(context.Context) (nfc.CardInfo, error) { return card, nil },
		Authenticate: func(_ context.Context, _ byte, _ nfc.KeyType, _ nfc.Key) error { return nil },
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
	service := NewHardnestedService(devices, devices, store, func(attack.MFoCInvocation) attack.AttackEngine { return staticHardnestedEngine{raw: raw} })
	result, err := service.Run(context.Background(), HardnestedRequest{
		TaskID: "hardnested-test", Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true,
	}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Claims) != mifare.Classic1KSectors*2 || result.Added != mifare.Classic1KSectors*2-1 || result.Existing != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestHardnestedWorkflowRequiresCapabilityAndSeed(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	store := keys.NewStore()
	devices := mfocTestDevices{reader: mock.NewReader()}
	service := NewHardnestedService(devices, devices, store, func(attack.MFoCInvocation) attack.AttackEngine { return staticHardnestedEngine{} })
	request := HardnestedRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}
	if _, err := service.Run(context.Background(), request, nil); !errors.Is(err, ErrMFoCSeedRequired) {
		t.Fatalf("missing seed error = %v", err)
	}
	record, _, _ := store.Add(nfc.Key{1}, keys.SourceUserInput)
	_ = store.Verify(keys.CardID(card), 0, nfc.KeyTypeA, record.ID)
	request.Device.ConnString = "acr122_pcsc:test"
	if _, err := service.Run(context.Background(), request, nil); !errors.Is(err, ErrHardnestedCapability) {
		t.Fatalf("capability error = %v", err)
	}
}
