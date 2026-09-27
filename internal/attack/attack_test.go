package attack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

var fixtureRuntimeRoot string

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "nfcx runtime 空格-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, source, _, _ := runtime.Caller(0)
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	for _, fixture := range []struct{ name, pkg string }{{"nfcx-fake-engine", "./cmd/nfcx-fake-engine"}, {"mfoc", "./cmd/nfcx-fake-mfoc"}, {"mfoc-hardnested", "./cmd/nfcx-fake-mfoc"}, {"mfcuk", "./cmd/nfcx-fake-mfcuk"}} {
		name := fixture.name
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		path := filepath.Join(root, name)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		command := exec.CommandContext(ctx, "go", "build", "-o", path, fixture.pkg)
		command.Dir = repositoryRoot
		output, buildErr := command.CombinedOutput()
		cancel()
		if buildErr != nil {
			fmt.Fprintf(os.Stderr, "build %s: %v\n%s", fixture.name, buildErr, output)
			_ = os.RemoveAll(root)
			os.Exit(1)
		}
	}
	fixtureRuntimeRoot = root
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}

type fakeReleasedDevices struct {
	device       nfc.DeviceInfo
	recoveryErr  error
	operationErr error
	mu           sync.Mutex
	order        []string
}

func (f *fakeReleasedDevices) WithReleasedDevice(ctx context.Context, hooks device.ReleasedDeviceHooks, operation func(context.Context, device.ReleasedDevice) error) device.ReleasedDeviceResult {
	if hooks.Releasing != nil {
		hooks.Releasing()
	}
	f.record("release")
	err := f.operationErr
	if err == nil {
		err = operation(ctx, device.ReleasedDevice{Device: f.device})
	}
	if hooks.Reopening != nil {
		hooks.Reopening()
	}
	f.record("reopen")
	return device.ReleasedDeviceResult{OperationError: err, RecoveryError: f.recoveryErr}
}

func (f *fakeReleasedDevices) record(value string) {
	f.mu.Lock()
	f.order = append(f.order, value)
	f.mu.Unlock()
}

type eventRecorder struct {
	mu     sync.Mutex
	events []AttackEvent
	notify chan struct{}
}

func newEventRecorder() *eventRecorder { return &eventRecorder{notify: make(chan struct{}, 256)} }

func (r *eventRecorder) emit(event AttackEvent) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

func (r *eventRecorder) snapshot() []AttackEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]AttackEvent(nil), r.events...)
}

func fixtureCoordinator(mode string, devices *fakeReleasedDevices) (*Coordinator, *FixtureEngine) {
	engine := NewFixtureEngine(runtimebundle.NewLocator(fixtureRuntimeRoot))
	engine.mode = mode
	engine.steps = 0
	engine.interval = 0
	return NewCoordinator(devices, engine), engine
}

func TestCoordinatorRunsFixtureWithSpacesUnicodeAndLiveStreams(t *testing.T) {
	devices := &fakeReleasedDevices{device: nfc.DeviceInfo{Name: "fixture", ConnString: "pn532_uart:/tmp/reader with spaces/测试"}}
	coordinator, _ := fixtureCoordinator("success", devices)
	recorder := newEventRecorder()
	temporaryRoot := filepath.Join(t.TempDir(), "task dirs 空格")
	if err := os.MkdirAll(temporaryRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Run(context.Background(), AttackRequest{TaskID: "fixture-1", Timeout: 5 * time.Second, TemporaryRoot: temporaryRoot}, recorder.emit)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.State != StateSucceeded || !result.Validated || result.ExitCode != 0 || result.Version != "nfcx-fake-engine 1.0" {
		t.Fatalf("result = %+v", result)
	}
	if result.TemporaryDir != "" {
		t.Fatalf("temporary directory was retained: %q", result.TemporaryDir)
	}
	if !strings.Contains(string(result.Stdout), "argument with spaces") || !strings.Contains(string(result.Stderr), "stderr is live") {
		t.Fatalf("captured streams stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	events := recorder.snapshot()
	wantStates := []State{StateQueued, StateReleasingDevice, StateStarting, StateRunning, StateValidatingResult, StateReopeningDevice, StateSucceeded}
	assertStateSubsequence(t, events, wantStates)
	var stdoutSeen, stderrSeen bool
	for index, event := range events {
		if event.Sequence != uint64(index+1) {
			t.Fatalf("event sequence %d = %d", index, event.Sequence)
		}
		stdoutSeen = stdoutSeen || event.Stream == StreamStdout
		stderrSeen = stderrSeen || event.Stream == StreamStderr
	}
	if !stdoutSeen || !stderrSeen {
		t.Fatalf("live stream events missing: %+v", events)
	}
}

func TestCoordinatorCancellationTerminatesProcessAndStillReopens(t *testing.T) {
	devices := &fakeReleasedDevices{device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/cancel"}}
	coordinator, _ := fixtureCoordinator("hang", devices)
	recorder := newEventRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		result AttackResult
		err    error
	}, 1)
	go func() {
		result, err := coordinator.Run(ctx, AttackRequest{TaskID: "cancel", Timeout: 10 * time.Second}, recorder.emit)
		done <- struct {
			result AttackResult
			err    error
		}{result, err}
	}()
	waitForState(t, recorder, StateRunning)
	cancel()
	select {
	case finished := <-done:
		if !errors.Is(finished.err, ErrCancelled) || finished.result.State != StateCancelled {
			t.Fatalf("cancel result=%+v err=%v", finished.result, finished.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("cancelled process did not terminate")
	}
	assertStateSubsequence(t, recorder.snapshot(), []State{StateCancelling, StateReopeningDevice, StateCancelled})
}

func TestCoordinatorTimeoutIsDistinctFromCancellation(t *testing.T) {
	devices := &fakeReleasedDevices{device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/timeout"}}
	coordinator, _ := fixtureCoordinator("hang", devices)
	result, err := coordinator.Run(context.Background(), AttackRequest{TaskID: "timeout", Timeout: 100 * time.Millisecond}, nil)
	if !errors.Is(err, ErrTimeout) || errors.Is(err, ErrCancelled) {
		t.Fatalf("timeout error = %v", err)
	}
	if result.State != StateFailed {
		t.Fatalf("timeout state = %s", result.State)
	}
}

func TestCoordinatorPreservesNonzeroExitDiagnosticsAndTemporaryPolicy(t *testing.T) {
	devices := &fakeReleasedDevices{device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/fail"}}
	coordinator, _ := fixtureCoordinator("fail", devices)
	result, err := coordinator.Run(context.Background(), AttackRequest{TaskID: "fail", Timeout: 5 * time.Second, TemporaryPolicy: KeepOnFailure}, nil)
	if !errors.Is(err, ErrExit) || result.ExitCode != 17 || result.Version != "nfcx-fake-engine 1.0" {
		t.Fatalf("failure result=%+v err=%v", result, err)
	}
	if !strings.Contains(string(result.Stderr), "requested failure") {
		t.Fatalf("stderr not preserved: %q", result.Stderr)
	}
	if result.TemporaryDir == "" {
		t.Fatal("failure diagnostics directory was not retained")
	}
	if _, statErr := os.Stat(result.TemporaryDir); statErr != nil {
		t.Fatalf("retained directory: %v", statErr)
	}
	_ = os.RemoveAll(result.TemporaryDir)
}

func TestFixtureConcurrentLargeOutputDoesNotDeadlock(t *testing.T) {
	devices := &fakeReleasedDevices{device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/spam"}}
	coordinator, _ := fixtureCoordinator("spam", devices)
	result, err := coordinator.Run(context.Background(), AttackRequest{TaskID: "spam", Timeout: 10 * time.Second}, nil)
	if err != nil {
		t.Fatalf("spam run: %v", err)
	}
	if len(result.Stdout) < 100_000 || len(result.Stderr) < 100_000 {
		t.Fatalf("large streams were not captured: stdout=%d stderr=%d", len(result.Stdout), len(result.Stderr))
	}
}

func TestInvalidResultFailsValidation(t *testing.T) {
	devices := &fakeReleasedDevices{device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/invalid"}}
	coordinator, _ := fixtureCoordinator("invalid", devices)
	result, err := coordinator.Run(context.Background(), AttackRequest{TaskID: "invalid", Timeout: 5 * time.Second}, nil)
	if !errors.Is(err, ErrValidation) || result.State != StateFailed || result.Validated {
		t.Fatalf("validation result=%+v err=%v", result, err)
	}
}

func TestMissingResultFailsValidation(t *testing.T) {
	devices := &fakeReleasedDevices{device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/missing"}}
	coordinator, _ := fixtureCoordinator("missing", devices)
	result, err := coordinator.Run(context.Background(), AttackRequest{TaskID: "missing", Timeout: 5 * time.Second}, nil)
	if !errors.Is(err, ErrValidation) || result.State != StateFailed || result.Validated {
		t.Fatalf("missing result=%+v err=%v", result, err)
	}
}

func TestRecoveryErrorDoesNotOverwriteSuccessfulEngineResult(t *testing.T) {
	recoveryErr := errors.New("reader reopen failed")
	devices := &fakeReleasedDevices{device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/recovery"}, recoveryErr: recoveryErr}
	coordinator, _ := fixtureCoordinator("success", devices)
	result, err := coordinator.Run(context.Background(), AttackRequest{TaskID: "recovery", Timeout: 5 * time.Second}, nil)
	if err != nil || result.State != StateSucceeded || !errors.Is(result.RecoveryError, recoveryErr) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestCoordinatorIntegrationTransfersRealManagerOwnership(t *testing.T) {
	reader := mock.NewReader()
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader { return reader }, nil, device.Config{
		PollInterval: time.Millisecond, PollTimeout: 20 * time.Millisecond, OpenTimeout: 20 * time.Millisecond,
		RemovalMisses: 2, ReconnectBackoffs: []time.Duration{time.Millisecond},
	})
	connString := "pn532_uart:/dev/integration with spaces"
	if err := manager.Connect(context.Background(), connString); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	engine := NewFixtureEngine(runtimebundle.NewLocator(fixtureRuntimeRoot))
	engine.steps = 0
	engine.interval = 0
	result, err := NewCoordinator(manager, engine).Run(context.Background(), AttackRequest{TaskID: "manager-integration", Timeout: 5 * time.Second}, nil)
	if err != nil || result.State != StateSucceeded || !result.Validated {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	open, reopenedConn, openCount, closeCount := reader.State()
	if !open || reopenedConn != connString || openCount != 2 || closeCount != 1 {
		t.Fatalf("reader state open=%t conn=%q opens=%d closes=%d", open, reopenedConn, openCount, closeCount)
	}
	if manager.Snapshot().Status != device.StatusPolling {
		t.Fatalf("manager status = %s", manager.Snapshot().Status)
	}
}

func TestCoordinatorReselectsExpectedCardBeforeProcessLaunch(t *testing.T) {
	reader := mock.NewReader()
	reader.SetCardResult(nfc.CardInfo{UID: []byte{9, 9, 9, 9}, ATQA: [2]byte{4, 0}, SAK: 0x08}, nil)
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader { return reader }, nil, device.Config{
		PollInterval: time.Hour, PollTimeout: time.Second, OpenTimeout: time.Second,
		RemovalMisses: 2, ReconnectBackoffs: []time.Duration{time.Millisecond},
	})
	if err := manager.Connect(context.Background(), "pn532_uart:/dev/card-swap"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	engine := NewFixtureEngine(runtimebundle.NewLocator(fixtureRuntimeRoot))
	engine.steps, engine.interval = 0, 0
	expected := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{4, 0}, SAK: 0x08}
	result, err := NewCoordinator(manager, engine).Run(context.Background(), AttackRequest{
		TaskID: "card-swap", Timeout: 5 * time.Second, ExpectedCard: &expected,
	}, nil)
	if !errors.Is(err, nfc.ErrCardChanged) || result.Validated || result.ExitCode != -1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	open, _, openCount, closeCount := reader.State()
	if !open || openCount != 2 || closeCount != 1 {
		t.Fatalf("reader was not reset after rejected preflight: open=%t opens=%d closes=%d", open, openCount, closeCount)
	}
}

func waitForState(t *testing.T, recorder *eventRecorder, wanted State) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		for _, event := range recorder.snapshot() {
			if event.State == wanted {
				return
			}
		}
		select {
		case <-recorder.notify:
		case <-timer.C:
			t.Fatalf("timed out waiting for state %s: %+v", wanted, recorder.snapshot())
		}
	}
}

func assertStateSubsequence(t *testing.T, events []AttackEvent, wanted []State) {
	t.Helper()
	index := 0
	for _, event := range events {
		if index < len(wanted) && event.State == wanted[index] {
			index++
		}
	}
	if index != len(wanted) {
		t.Fatalf("states do not contain %v in order: %+v", wanted, events)
	}
}
