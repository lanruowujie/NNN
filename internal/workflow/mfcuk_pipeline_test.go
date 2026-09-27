package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
)

type pipelineDevices struct {
	reader nfc.Reader
	mu     sync.Mutex
	order  []string
}

type blockingPipelineEngine struct {
	name    string
	started chan struct{}
}

func (e blockingPipelineEngine) Name() string                    { return e.name }
func (e blockingPipelineEngine) Available(context.Context) error { return nil }
func (e blockingPipelineEngine) Run(ctx context.Context, _ attack.AttackRequest, _ func(attack.AttackEvent)) (attack.AttackResult, error) {
	close(e.started)
	<-ctx.Done()
	return attack.AttackResult{Engine: e.name, ExitCode: -1}, ctx.Err()
}

func (d *pipelineDevices) WithReader(ctx context.Context, operation func(context.Context, nfc.Reader) error) error {
	d.record("with_reader")
	return operation(ctx, d.reader)
}

func (d *pipelineDevices) WithReleasedDevice(ctx context.Context, hooks device.ReleasedDeviceHooks, operation func(context.Context, device.ReleasedDevice) error) device.ReleasedDeviceResult {
	if hooks.Releasing != nil {
		hooks.Releasing()
	}
	if hooks.Prepare != nil {
		if err := hooks.Prepare(ctx, d.reader); err != nil {
			return device.ReleasedDeviceResult{OperationError: err}
		}
	}
	d.record("release")
	err := operation(ctx, device.ReleasedDevice{Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Capabilities: device.Capabilities{Nested: true, Darkside: true}})
	if hooks.Reopening != nil {
		hooks.Reopening()
	}
	d.record("reopen")
	return device.ReleasedDeviceResult{OperationError: err}
}

func (d *pipelineDevices) record(value string) {
	d.mu.Lock()
	d.order = append(d.order, value)
	d.mu.Unlock()
}

type staticMFCUKEngine struct {
	candidates []attack.MFCUKCandidate
	err        error
}

func (e staticMFCUKEngine) Name() string                    { return "mfcuk" }
func (e staticMFCUKEngine) Available(context.Context) error { return nil }
func (e staticMFCUKEngine) Run(_ context.Context, _ attack.AttackRequest, _ func(attack.AttackEvent)) (attack.AttackResult, error) {
	payload, _ := json.Marshal(e.candidates)
	result := attack.AttackResult{Engine: "mfcuk", Version: attack.MFCUKVersion, ExitCode: 0, Validated: e.err == nil,
		Artifacts: []attack.AttackArtifact{{MediaType: "application/vnd.nfcx.mfcuk-candidates+json", Data: payload}}}
	return result, e.err
}

func TestMFCUKPipelineVerifiesEveryCandidateThenRunsMFoC(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{4, 0}, SAK: 0x08}
	good := nfc.Key{1, 2, 3, 4, 5, 6}
	bad := nfc.Key{6, 5, 4, 3, 2, 1}
	reader := mock.NewReaderWithFuncs(mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) { return card, nil },
		Authenticate: func(_ context.Context, _ byte, _ nfc.KeyType, key nfc.Key) error {
			if key == bad {
				return nfc.ErrAuthenticationFailed
			}
			return nil
		},
	})
	if err := reader.Open(context.Background(), "pn532_uart:/dev/test"); err != nil {
		t.Fatal(err)
	}
	devices := &pipelineDevices{reader: reader}
	store := keys.NewStore()
	mfoc := NewMFoCService(devices, devices, store, func(attack.MFoCInvocation) attack.AttackEngine {
		return staticMFoCEngine{raw: mfocTestDump(t, card)}
	})
	pipeline := NewMFCUKPipelineService(devices, devices, store, func(attack.MFCUKInvocation) attack.AttackEngine {
		return staticMFCUKEngine{candidates: []attack.MFCUKCandidate{{Sector: 0, Type: nfc.KeyTypeA, Value: bad}, {Sector: 3, Type: nfc.KeyTypeB, Value: good}}}
	}, mfoc)
	result, err := pipeline.Run(context.Background(), MFCUKPipelineRequest{
		TaskID: "pipeline", Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true,
	}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Verified != 1 || result.Rejected != 1 || len(result.MFoC.Claims) != 32 {
		t.Fatalf("result=%+v", result)
	}
	want := []string{"release", "reopen", "with_reader", "release", "reopen", "with_reader"}
	if len(devices.order) != len(want) {
		t.Fatalf("order=%v", devices.order)
	}
	for index := range want {
		if devices.order[index] != want[index] {
			t.Fatalf("order=%v; want=%v", devices.order, want)
		}
	}
}

func TestMFCUKPipelineDoesNotRunMFoCWithoutVerifiedCandidate(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	reader := mock.NewReaderWithFuncs(mock.ReaderFuncs{
		CardInfo:     func(context.Context) (nfc.CardInfo, error) { return card, nil },
		Authenticate: func(context.Context, byte, nfc.KeyType, nfc.Key) error { return nfc.ErrAuthenticationFailed },
	})
	_ = reader.Open(context.Background(), "pn532_uart:/dev/test")
	devices := &pipelineDevices{reader: reader}
	store := keys.NewStore()
	mfocCalls := 0
	mfoc := NewMFoCService(devices, devices, store, func(attack.MFoCInvocation) attack.AttackEngine {
		mfocCalls++
		return staticMFoCEngine{}
	})
	pipeline := NewMFCUKPipelineService(devices, devices, store, func(attack.MFCUKInvocation) attack.AttackEngine {
		return staticMFCUKEngine{candidates: []attack.MFCUKCandidate{{Sector: 0, Type: nfc.KeyTypeA, Value: nfc.Key{1}}}}
	}, mfoc)
	_, err := pipeline.Run(context.Background(), MFCUKPipelineRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}, nil)
	if !errors.Is(err, ErrMFCUKNoVerifiedCandidate) || mfocCalls != 0 || len(devices.order) != 3 {
		t.Fatalf("error=%v calls=%d order=%v", err, mfocCalls, devices.order)
	}
}

func TestMFCUKPipelinePreconditions(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	reader := mock.NewReader()
	devices := &pipelineDevices{reader: reader}
	store := keys.NewStore()
	mfoc := NewMFoCService(devices, devices, store, func(attack.MFoCInvocation) attack.AttackEngine { return staticMFoCEngine{} })
	pipeline := NewMFCUKPipelineService(devices, devices, store, func(attack.MFCUKInvocation) attack.AttackEngine { return staticMFCUKEngine{} }, mfoc)
	base := MFCUKPipelineRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}
	unauthorized := base
	unauthorized.Authorized = false
	if _, err := pipeline.Run(context.Background(), unauthorized, nil); !errors.Is(err, ErrMFCUKAuthorization) {
		t.Fatalf("authorization error=%v", err)
	}
	unsupported := base
	unsupported.Device.ConnString = "acr122_pcsc:test"
	if _, err := pipeline.Run(context.Background(), unsupported, nil); !errors.Is(err, ErrMFCUKDarkside) {
		t.Fatalf("capability error=%v", err)
	}
	fourK := base
	fourK.Card.SAK = 0x18
	if _, err := pipeline.Run(context.Background(), fourK, nil); !errors.Is(err, ErrMFCUKClassic1K) {
		t.Fatalf("card error=%v", err)
	}
}

func TestMFCUKPipelineCancellationDuringFirstStageReopensReader(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	reader := mock.NewReaderWithFuncs(mock.ReaderFuncs{CardInfo: func(context.Context) (nfc.CardInfo, error) { return card, nil }})
	_ = reader.Open(context.Background(), "pn532_uart:/dev/test")
	devices := &pipelineDevices{reader: reader}
	store := keys.NewStore()
	started := make(chan struct{})
	mfoc := NewMFoCService(devices, devices, store, func(attack.MFoCInvocation) attack.AttackEngine { return staticMFoCEngine{} })
	pipeline := NewMFCUKPipelineService(devices, devices, store, func(attack.MFCUKInvocation) attack.AttackEngine {
		return blockingPipelineEngine{name: "mfcuk", started: started}
	}, mfoc)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := pipeline.Run(ctx, MFCUKPipelineRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}, nil)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("mfcuk stage did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, attack.ErrCancelled) {
			t.Fatalf("cancel error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pipeline did not cancel")
	}
	if len(devices.order) != 2 || devices.order[0] != "release" || devices.order[1] != "reopen" {
		t.Fatalf("order=%v", devices.order)
	}
}

func TestMFCUKPipelineSecondStageFailureKeepsVerifiedSeed(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x08}
	seed := nfc.Key{1, 2, 3, 4, 5, 6}
	reader := mock.NewReaderWithFuncs(mock.ReaderFuncs{
		CardInfo:     func(context.Context) (nfc.CardInfo, error) { return card, nil },
		Authenticate: func(context.Context, byte, nfc.KeyType, nfc.Key) error { return nil },
	})
	_ = reader.Open(context.Background(), "pn532_uart:/dev/test")
	devices := &pipelineDevices{reader: reader}
	store := keys.NewStore()
	mfocFailure := &attack.Error{Code: attack.CodeExit, Engine: "mfoc", ExitCode: 9, Detail: "fixture failure"}
	mfoc := NewMFoCService(devices, devices, store, func(attack.MFoCInvocation) attack.AttackEngine {
		return staticErrorEngine{name: "mfoc", err: mfocFailure}
	})
	pipeline := NewMFCUKPipelineService(devices, devices, store, func(attack.MFCUKInvocation) attack.AttackEngine {
		return staticMFCUKEngine{candidates: []attack.MFCUKCandidate{{Sector: 0, Type: nfc.KeyTypeA, Value: seed}}}
	}, mfoc)
	result, err := pipeline.Run(context.Background(), MFCUKPipelineRequest{Card: card, Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"}, Authorized: true}, nil)
	if !errors.Is(err, attack.ErrExit) || result.Verified != 1 || len(store.Verified(keys.CardID(card))) != 1 {
		t.Fatalf("result=%+v error=%v verified=%v", result, err, store.Verified(keys.CardID(card)))
	}
}

type staticErrorEngine struct {
	name string
	err  error
}

func (e staticErrorEngine) Name() string                    { return e.name }
func (e staticErrorEngine) Available(context.Context) error { return nil }
func (e staticErrorEngine) Run(context.Context, attack.AttackRequest, func(attack.AttackEvent)) (attack.AttackResult, error) {
	return attack.AttackResult{Engine: e.name, ExitCode: 9}, e.err
}
