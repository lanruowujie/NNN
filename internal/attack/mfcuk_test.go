package attack

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/nfc"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

func TestMFCUKEngineAvailabilityAndCandidateArtifact(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	engine := NewMFCUKEngine(runtimebundle.NewLocator(fixtureRuntimeRoot), MFCUKInvocation{Card: card})
	if err := engine.Available(context.Background()); err != nil {
		t.Fatalf("available: %v", err)
	}
	engine.extraEnv = map[string]string{"NFCX_FAKE_MFCUK_MODE": "multiple"}
	result, err := engine.Run(context.Background(), AttackRequest{
		Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/reader with spaces"}, WorkingDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	candidates, err := MFCUKCandidatesFromArtifacts(result.Artifacts)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("candidates=%+v err=%v", candidates, err)
	}
	if !result.Validated || result.Version != MFCUKVersion || candidates[0].Sector != 0 || candidates[0].Type != nfc.KeyTypeA || candidates[1].Sector != 3 || candidates[1].Type != nfc.KeyTypeB {
		t.Fatalf("result=%+v candidates=%+v", result, candidates)
	}
}

func TestMFCUKEngineFailureAndTimeout(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	for _, test := range []struct {
		name string
		mode string
		want error
	}{{"nonzero", "nonzero", ErrExit}, {"missing", "missing", ErrValidation}} {
		t.Run(test.name, func(t *testing.T) {
			engine := NewMFCUKEngine(runtimebundle.NewLocator(fixtureRuntimeRoot), MFCUKInvocation{Card: card})
			engine.extraEnv = map[string]string{"NFCX_FAKE_MFCUK_MODE": test.mode}
			_, err := engine.Run(context.Background(), AttackRequest{Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, WorkingDir: t.TempDir()}, nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v; want %v", err, test.want)
			}
		})
	}
	engine := NewMFCUKEngine(runtimebundle.NewLocator(fixtureRuntimeRoot), MFCUKInvocation{Card: card})
	engine.extraEnv = map[string]string{"NFCX_FAKE_MFCUK_MODE": "hang"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := engine.Run(ctx, AttackRequest{Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, WorkingDir: t.TempDir()}, nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("timeout error=%v", err)
	}
}

func TestMFCUKCandidateParserRejectsMissingAndDeduplicates(t *testing.T) {
	stream := []byte("prefix\nNFCX_RESULT key=B sector=3 value=A0A1A2A3A4A5\nNFCX_RESULT key=B sector=3 value=a0a1a2a3a4a5\n")
	candidates, err := parseMFCUKCandidates(stream[:31], stream[31:])
	if err != nil || len(candidates) != 1 || candidates[0].Sector != 3 || candidates[0].Type != nfc.KeyTypeB {
		t.Fatalf("candidates=%+v err=%v", candidates, err)
	}
	if _, err := parseMFCUKCandidates([]byte("INFO: block 3 recovered KEY: ffffffffffff")); err == nil || !strings.Contains(err.Error(), "no machine-readable") {
		t.Fatalf("missing marker error=%v", err)
	}
}

func TestMFCUKProgressParserAcceptsArbitraryChunks(t *testing.T) {
	var events []AttackEvent
	stream := newMFCUKStreamEmitter(func(event AttackEvent) { events = append(events, event) })
	for _, chunk := range []string{"NFCX_PROG", "RESS key=A sector=0 ", "attempts=25\n"} {
		stream.emit(AttackEvent{Engine: "mfcuk", State: StateRunning, Stream: StreamStdout, Data: chunk})
	}
	stream.flush()
	combined := ""
	progress := false
	for _, event := range events {
		combined += event.Data
		progress = progress || strings.Contains(event.Message, "25 次认证")
	}
	if combined != "NFCX_PROGRESS key=A sector=0 attempts=25\n" || !progress {
		t.Fatalf("events = %+v", events)
	}
}

func TestMFCUKEmitsHeartbeatWhileRecoveryIsSilent(t *testing.T) {
	engine := NewMFCUKEngine(runtimebundle.NewLocator(fixtureRuntimeRoot), MFCUKInvocation{Card: nfc.CardInfo{SAK: 0x08}})
	engine.extraEnv = map[string]string{"NFCX_FAKE_MFCUK_MODE": "hang"}
	engine.runner.HeartbeatInterval = 10 * time.Millisecond
	recorder := &eventRecorder{}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err := engine.Run(ctx, AttackRequest{Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, WorkingDir: t.TempDir()}, recorder.emit)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("timeout error=%v", err)
	}
	for _, event := range recorder.snapshot() {
		if strings.Contains(event.Message, "进程仍在运行") && strings.Contains(event.Message, "超时剩余") {
			return
		}
	}
	t.Fatal("missing external process heartbeat")
}

func TestParseMFCUKVersionAcceptsMachineAndPlatformHeaders(t *testing.T) {
	for _, output := range []string{
		"NFCX_VERSION 0.3.8\n",
		"mfcuk - 0.3.8\n",
		"/Applications/NFCX.app/Contents/Resources/runtime/darwin-arm64/mfcuk - v0.3.8\n",
		`C:\\NFCX\\runtime\\mfcuk.exe version 0.3.8` + "\r\n",
	} {
		if got := parseMFCUKVersion([]byte(output)); got != MFCUKVersion {
			t.Fatalf("parseMFCUKVersion(%q)=%q", output, got)
		}
	}
}

func TestMFCUKRejectsUnsupportedCardAndDevice(t *testing.T) {
	engine := NewMFCUKEngine(runtimebundle.NewLocator(fixtureRuntimeRoot), MFCUKInvocation{Card: nfc.CardInfo{SAK: 0x18}})
	_, err := engine.Run(context.Background(), AttackRequest{Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, WorkingDir: t.TempDir()}, nil)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("4K error=%v", err)
	}
	engine = NewMFCUKEngine(runtimebundle.NewLocator(fixtureRuntimeRoot), MFCUKInvocation{Card: nfc.CardInfo{SAK: 0x08}})
	_, err = engine.Run(context.Background(), AttackRequest{Device: nfc.DeviceInfo{ConnString: "acr122_pcsc:test"}, WorkingDir: t.TempDir()}, nil)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("device error=%v", err)
	}
}
