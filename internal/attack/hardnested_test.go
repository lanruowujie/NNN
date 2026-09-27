package attack

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/nfc"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

func TestHardnestedEngineAvailabilityAndSuccessfulValidatedDump(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{4, 0}, SAK: 0x08}
	key := nfc.Key{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	engine := NewHardnestedEngine(runtimebundle.NewLocator(fixtureRuntimeRoot), MFoCInvocation{
		Card: card, KnownKeys: []MFoCKnownKey{{Sector: 3, Type: nfc.KeyTypeB, Value: key}},
	})
	if err := engine.Available(context.Background()); err != nil {
		t.Fatalf("available: %v", err)
	}
	result, err := engine.Run(context.Background(), AttackRequest{
		Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/reader with spaces"}, WorkingDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Validated || result.ExitCode != 0 || result.Version != HardnestedVersion || len(result.Artifacts) != 1 || len(result.Artifacts[0].Data) != 1024 {
		t.Fatalf("result = %+v", result)
	}
}

func TestHardnestedArgumentsForceOnlyVerifiedSeedPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result with spaces.mfd")
	first := nfc.Key{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	second := nfc.Key{1, 2, 3, 4, 5, 6}
	arguments := hardnestedArguments([]MFoCKnownKey{
		{Sector: 1, Type: nfc.KeyTypeA, Value: first},
		{Sector: 9, Type: nfc.KeyTypeB, Value: first},
		{Sector: 2, Type: nfc.KeyTypeB, Value: second},
	}, path)
	want := []string{"-C", "-F", "-k", "010203040506", "-k", "AABBCCDDEEFF", "-O", path}
	if strings.Join(arguments, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("arguments = %#v; want %#v", arguments, want)
	}
}

func TestHardnestedStreamFlushKeepsEngineIdentity(t *testing.T) {
	var events []AttackEvent
	stream := newMFoCStreamEmitter(func(event AttackEvent) { events = append(events, event) }, 16, "mfoc-hardnested")
	stream.emit(AttackEvent{Engine: "mfoc-hardnested", State: StateRunning, Stream: StreamStdout, Data: "Sector: 3"})
	stream.flush()
	if len(events) != 2 || events[0].Engine != "mfoc-hardnested" || events[1].Engine != "mfoc-hardnested" {
		t.Fatalf("events = %+v", events)
	}
}

func TestHardnestedEngineTimeout(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	engine := NewHardnestedEngine(runtimebundle.NewLocator(fixtureRuntimeRoot), MFoCInvocation{
		Card: card, KnownKeys: []MFoCKnownKey{{Sector: 0, Type: nfc.KeyTypeA, Value: nfc.Key{1}}},
	})
	engine.extraEnv = map[string]string{"NFCX_FAKE_HARDNESTED_MODE": "hang"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := engine.Run(ctx, AttackRequest{Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, WorkingDir: t.TempDir()}, nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v", err)
	}
}

func TestHardnestedEngineFailureAndTimeout(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	key := nfc.Key{1, 2, 3, 4, 5, 6}
	engine := NewHardnestedEngine(runtimebundle.NewLocator(fixtureRuntimeRoot), MFoCInvocation{Card: card, KnownKeys: []MFoCKnownKey{{Sector: 0, Type: nfc.KeyTypeA, Value: key}}})
	engine.extraEnv = map[string]string{"NFCX_FAKE_HARDNESTED_MODE": "missing"}
	if _, err := engine.Run(context.Background(), AttackRequest{Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, WorkingDir: t.TempDir()}, nil); !errors.Is(err, ErrValidation) {
		t.Fatalf("missing output error = %v", err)
	}
	engine.extraEnv = map[string]string{"NFCX_FAKE_HARDNESTED_MODE": "hang"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := engine.Run(ctx, AttackRequest{Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, WorkingDir: t.TempDir()}, nil); !errors.Is(err, ErrTimeout) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestHardnestedEngineRejectsUnsupportedReader(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	engine := NewHardnestedEngine(runtimebundle.NewLocator(fixtureRuntimeRoot), MFoCInvocation{Card: card, KnownKeys: []MFoCKnownKey{{Sector: 0, Type: nfc.KeyTypeA, Value: nfc.Key{1}}}})
	_, err := engine.Run(context.Background(), AttackRequest{Device: nfc.DeviceInfo{ConnString: "acr122_pcsc:test"}, WorkingDir: t.TempDir()}, nil)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v", err)
	}
}
