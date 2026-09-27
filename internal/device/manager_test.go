package device_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
)

func TestNestedCapabilityIsExplicitlyLimitedToPN532UART(t *testing.T) {
	capabilities := device.CapabilitiesFor(nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"})
	if !capabilities.Nested || !capabilities.Darkside || !capabilities.Hardnested {
		t.Fatal("pn532_uart should be enabled for the validated Nested, Darkside, and Hardnested profile")
	}
	for _, conn := range []string{"acr122_pcsc:test", "pn53x_usb:001:002", "unknown:test", ""} {
		capabilities := device.CapabilitiesFor(nfc.DeviceInfo{ConnString: conn})
		if capabilities.Nested || capabilities.Darkside || capabilities.Hardnested {
			t.Fatalf("unexpected attack capability for %q", conn)
		}
	}
}

type cardResult struct {
	card nfc.CardInfo
	err  error
}

type channelReader struct {
	results chan cardResult
	opened  chan string
	started chan struct{}
	exited  chan struct{}
	start   sync.Once
	exit    sync.Once
}

func newChannelReader() *channelReader {
	return &channelReader{
		results: make(chan cardResult, 32),
		opened:  make(chan string, 1),
		started: make(chan struct{}),
		exited:  make(chan struct{}),
	}
}

func (r *channelReader) Open(_ context.Context, connString string) error {
	r.opened <- connString
	return nil
}

func (r *channelReader) Close() error { return nil }

func (r *channelReader) CardInfo(ctx context.Context) (nfc.CardInfo, error) {
	r.start.Do(func() { close(r.started) })
	select {
	case result := <-r.results:
		return result.card.Clone(), result.err
	case <-ctx.Done():
		r.exit.Do(func() { close(r.exited) })
		code := nfc.CodeCanceled
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = nfc.CodeTimeout
		}
		return nfc.CardInfo{}, nfc.NewError("card info", code, "", ctx.Err())
	}
}

func (r *channelReader) Authenticate(context.Context, byte, nfc.KeyType, nfc.Key) error {
	return nfc.ErrUnsupported
}
func (r *channelReader) ReadBlock(context.Context, byte) ([nfc.BlockSize]byte, error) {
	return [nfc.BlockSize]byte{}, nfc.ErrUnsupported
}
func (r *channelReader) WriteBlock(context.Context, byte, [nfc.BlockSize]byte) error {
	return nfc.ErrUnsupported
}

type recordingSink struct {
	mu     sync.Mutex
	states []device.Snapshot
	cards  []device.CardEvent
	notify chan struct{}
}

func newRecordingSink() *recordingSink {
	return &recordingSink{notify: make(chan struct{}, 128)}
}

func (s *recordingSink) DeviceStateChanged(snapshot device.Snapshot) {
	s.mu.Lock()
	s.states = append(s.states, snapshot)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *recordingSink) CardChanged(event device.CardEvent) {
	s.mu.Lock()
	s.cards = append(s.cards, event)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *recordingSink) cardEvents() []device.CardEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]device.CardEvent(nil), s.cards...)
}

func (s *recordingSink) statuses() []device.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]device.Status, len(s.states))
	for index, state := range s.states {
		result[index] = state.Status
	}
	return result
}

func waitUntil(t *testing.T, sink *recordingSink, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for !condition() {
		select {
		case <-sink.notify:
		case <-deadline.C:
			t.Fatal("timed out waiting for device event")
		}
	}
}

func testConfig() device.Config {
	return device.Config{
		PollInterval:      time.Millisecond,
		PollTimeout:       100 * time.Millisecond,
		OpenTimeout:       50 * time.Millisecond,
		RemovalMisses:     2,
		ReconnectBackoffs: []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
	}
}

func TestCardEventsAreDeduplicatedAndOrdered(t *testing.T) {
	reader := newChannelReader()
	sink := newRecordingSink()
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader { return reader }, sink, testConfig())
	if err := manager.Connect(context.Background(), "pn532_uart:/dev/test"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown() })

	cardA := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 0x08}
	cardB := nfc.CardInfo{UID: []byte{5, 6, 7, 8}, ATQA: [2]byte{0, 4}, SAK: 0x08}
	reader.results <- cardResult{card: cardA}
	reader.results <- cardResult{card: cardA}
	reader.results <- cardResult{card: cardA}
	reader.results <- cardResult{card: cardB}
	reader.results <- cardResult{err: nfc.ErrNoCard}
	reader.results <- cardResult{err: nfc.ErrNoCard}

	waitUntil(t, sink, func() bool { return len(sink.cardEvents()) == 4 })
	events := sink.cardEvents()
	wantTypes := []device.CardEventType{device.CardAppeared, device.CardRemoved, device.CardAppeared, device.CardRemoved}
	for index, want := range wantTypes {
		if events[index].Type != want {
			t.Fatalf("event %d type = %q; want %q; all=%+v", index, events[index].Type, want, events)
		}
	}
	if got := events[0].Card.UID[0]; got != 1 {
		t.Fatalf("first card UID changed: %d", got)
	}
	if got := events[2].Card.UID[0]; got != 5 {
		t.Fatalf("replacement card UID = %d; want 5", got)
	}
}

func TestDisconnectCancelsPollingAndPreservesExactConnString(t *testing.T) {
	reader := newChannelReader()
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader { return reader }, nil, testConfig())
	connString := "  pn532_uart:/dev/tty exact  "
	if err := manager.Connect(context.Background(), connString); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if got := <-reader.opened; got != connString {
		t.Fatalf("Open connstring = %q; want exact %q", got, connString)
	}
	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("polling CardInfo did not start")
	}
	if err := manager.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	select {
	case <-reader.exited:
	case <-time.After(time.Second):
		t.Fatal("polling CardInfo did not observe cancellation")
	}
	if got := manager.Snapshot().Status; got != device.StatusDisconnected {
		t.Fatalf("status = %q; want disconnected", got)
	}
}

type failingOpenReader struct {
	open func(string) error
	card func(context.Context) error
}

func (r *failingOpenReader) Open(_ context.Context, conn string) error { return r.open(conn) }
func (*failingOpenReader) Close() error                                { return nil }
func (r *failingOpenReader) CardInfo(ctx context.Context) (nfc.CardInfo, error) {
	return nfc.CardInfo{}, r.card(ctx)
}
func (*failingOpenReader) Authenticate(context.Context, byte, nfc.KeyType, nfc.Key) error {
	return nfc.ErrUnsupported
}
func (*failingOpenReader) ReadBlock(context.Context, byte) ([nfc.BlockSize]byte, error) {
	return [nfc.BlockSize]byte{}, nfc.ErrUnsupported
}
func (*failingOpenReader) WriteBlock(context.Context, byte, [nfc.BlockSize]byte) error {
	return nfc.ErrUnsupported
}

func TestDisconnectedDeviceUsesFiniteRecovery(t *testing.T) {
	sink := newRecordingSink()
	var factoryCalls atomic.Int32
	var openedMu sync.Mutex
	var opened []string
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader {
		call := factoryCalls.Add(1)
		return &failingOpenReader{
			open: func(conn string) error {
				openedMu.Lock()
				opened = append(opened, conn)
				openedMu.Unlock()
				if call == 1 {
					return nil
				}
				return nfc.ErrDeviceDisconnected
			},
			card: func(context.Context) error { return nfc.ErrDeviceDisconnected },
		}
	}, sink, testConfig())
	connString := "pn532_uart:/dev/reconnect"
	if err := manager.Connect(context.Background(), connString); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown() })

	waitUntil(t, sink, func() bool {
		state := manager.Snapshot()
		return state.Status == device.StatusError && state.Detail == "读卡器自动恢复失败"
	})
	if got, want := factoryCalls.Load(), int32(1+len(testConfig().ReconnectBackoffs)); got != want {
		t.Fatalf("reader factory calls = %d; want finite %d", got, want)
	}
	statuses := sink.statuses()
	if !containsStatus(statuses, device.StatusError) || !containsStatus(statuses, device.StatusRecovering) {
		t.Fatalf("recovery states missing from %v", statuses)
	}
	openedMu.Lock()
	defer openedMu.Unlock()
	for _, got := range opened {
		if got != connString {
			t.Fatalf("recovery rewrote connstring to %q", got)
		}
	}
}

func TestPermissionFailureDoesNotReconnect(t *testing.T) {
	sink := newRecordingSink()
	var factoryCalls atomic.Int32
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader {
		factoryCalls.Add(1)
		return &failingOpenReader{
			open: func(string) error { return nil },
			card: func(context.Context) error { return nfc.ErrPermission },
		}
	}, sink, testConfig())
	if err := manager.Connect(context.Background(), "pn532_uart:/dev/denied"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown() })
	waitUntil(t, sink, func() bool { return manager.Snapshot().Status == device.StatusError })
	if got := factoryCalls.Load(); got != 1 {
		t.Fatalf("permission error retried %d times", got-1)
	}
}

func TestDisconnectedDeviceRecoversAndResumesPolling(t *testing.T) {
	sink := newRecordingSink()
	var factoryCalls atomic.Int32
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader {
		call := factoryCalls.Add(1)
		if call == 1 {
			return &failingOpenReader{
				open: func(string) error { return nil },
				card: func(context.Context) error { return nfc.ErrDeviceDisconnected },
			}
		}
		return &failingOpenReader{
			open: func(string) error { return nil },
			card: func(ctx context.Context) error {
				<-ctx.Done()
				return nfc.NewError("card info", nfc.CodeTimeout, "", ctx.Err())
			},
		}
	}, sink, testConfig())
	if err := manager.Connect(context.Background(), "pn532_uart:/dev/recover"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown() })
	waitUntil(t, sink, func() bool {
		return factoryCalls.Load() == 2 && manager.Snapshot().Status == device.StatusPolling
	})
	statuses := sink.statuses()
	if !containsStatus(statuses, device.StatusRecovering) || !containsStatus(statuses, device.StatusReady) {
		t.Fatalf("successful recovery states missing from %v", statuses)
	}
}

func TestWithReaderSerializesConcurrentOperations(t *testing.T) {
	reader := newChannelReader()
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader { return reader }, nil, testConfig())
	if err := manager.Connect(context.Background(), "pn532_uart:/dev/test"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown() })

	var active atomic.Int32
	var maximum atomic.Int32
	var group sync.WaitGroup
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			err := manager.WithReader(context.Background(), func(context.Context, nfc.Reader) error {
				current := active.Add(1)
				for current > maximum.Load() && !maximum.CompareAndSwap(maximum.Load(), current) {
				}
				time.Sleep(time.Millisecond)
				active.Add(-1)
				return nil
			})
			if err != nil {
				t.Errorf("exclusive operation: %v", err)
			}
		}()
	}
	group.Wait()
	if got := maximum.Load(); got != 1 {
		t.Fatalf("maximum concurrent operations = %d; want 1", got)
	}
}

func TestListDevicesPreservesAndDeduplicatesConnStrings(t *testing.T) {
	devices := []nfc.DeviceInfo{
		{Name: "A", ConnString: " pn532_uart:/dev/a "},
		{Name: "duplicate", ConnString: " pn532_uart:/dev/a "},
		{Name: "empty"},
	}
	manager := device.NewManager(mock.NewEnumerator(devices, nil), nil, nil, device.Config{})
	got, err := manager.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(got) != 1 || got[0] != devices[0] {
		t.Fatalf("ListDevices() = %+v; want exact first device", got)
	}
}

func TestWithReaderRequiresConnection(t *testing.T) {
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader { return newChannelReader() }, nil, testConfig())
	err := manager.WithReader(context.Background(), func(context.Context, nfc.Reader) error { return nil })
	if !errors.Is(err, nfc.ErrNotOpen) {
		t.Fatalf("WithReader error = %v; want ErrNotOpen", err)
	}
}

func TestDisconnectCancelsExclusiveOperation(t *testing.T) {
	reader := newChannelReader()
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader { return reader }, nil, testConfig())
	if err := manager.Connect(context.Background(), "pn532_uart:/dev/test"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- manager.WithReader(context.Background(), func(ctx context.Context, _ nfc.Reader) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("exclusive operation did not start")
	}
	if err := manager.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("operation error = %v; want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("disconnect did not cancel exclusive operation")
	}
}

type releasedReader struct {
	openErr  error
	closeErr error
	opened   chan string
	polling  chan struct{}
	pollOnce sync.Once
	closed   atomic.Int32
}

func newReleasedReader() *releasedReader {
	return &releasedReader{opened: make(chan string, 1), polling: make(chan struct{})}
}

func (r *releasedReader) Open(_ context.Context, connString string) error {
	r.opened <- connString
	return r.openErr
}

func (r *releasedReader) Close() error {
	r.closed.Add(1)
	return r.closeErr
}

func (r *releasedReader) CardInfo(ctx context.Context) (nfc.CardInfo, error) {
	r.pollOnce.Do(func() { close(r.polling) })
	select {
	case <-time.After(time.Millisecond):
		return nfc.CardInfo{}, nfc.ErrNoCard
	case <-ctx.Done():
		return nfc.CardInfo{}, nfc.NewError("card info", nfc.CodeCanceled, "", ctx.Err())
	}
}

func (*releasedReader) Authenticate(context.Context, byte, nfc.KeyType, nfc.Key) error {
	return nfc.ErrUnsupported
}
func (*releasedReader) ReadBlock(context.Context, byte) ([nfc.BlockSize]byte, error) {
	return [nfc.BlockSize]byte{}, nfc.ErrUnsupported
}
func (*releasedReader) WriteBlock(context.Context, byte, [nfc.BlockSize]byte) error {
	return nfc.ErrUnsupported
}

func TestWithReleasedDeviceClosesRunsAndReopens(t *testing.T) {
	first, reopened := newReleasedReader(), newReleasedReader()
	readers := []nfc.Reader{first, reopened}
	var factoryIndex atomic.Int32
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader {
		index := int(factoryIndex.Add(1)) - 1
		return readers[index]
	}, nil, testConfig())
	connString := "pn532_uart:/dev/path with spaces/测试"
	if err := manager.Connect(context.Background(), connString); err != nil {
		t.Fatalf("connect: %v", err)
	}
	select {
	case <-first.polling:
	case <-time.After(time.Second):
		t.Fatal("initial polling did not start")
	}
	operationErr := errors.New("engine failed")
	var phases []string
	result := manager.WithReleasedDevice(context.Background(), device.ReleasedDeviceHooks{
		Releasing: func() { phases = append(phases, "release") },
		Reopening: func() { phases = append(phases, "reopen") },
	}, func(_ context.Context, released device.ReleasedDevice) error {
		phases = append(phases, "run")
		if got := first.closed.Load(); got != 1 {
			t.Fatalf("reader close count during callback = %d", got)
		}
		if released.Device.ConnString != connString {
			t.Fatalf("released connstring = %q", released.Device.ConnString)
		}
		if got := manager.Snapshot().Status; got != device.StatusBusy {
			t.Fatalf("status during callback = %q", got)
		}
		if err := manager.WithReader(context.Background(), func(context.Context, nfc.Reader) error { return nil }); !errors.Is(err, nfc.ErrBusy) {
			t.Fatalf("concurrent in-process operation error = %v; want busy", err)
		}
		if _, err := manager.ListDevices(context.Background()); !errors.Is(err, nfc.ErrBusy) {
			t.Fatalf("concurrent enumeration error = %v; want busy", err)
		}
		return operationErr
	})
	if !errors.Is(result.OperationError, operationErr) || result.RecoveryError != nil {
		t.Fatalf("released result = %+v", result)
	}
	if got := fmt.Sprint(phases); got != "[release run reopen]" {
		t.Fatalf("phase order = %s", got)
	}
	if got := <-reopened.opened; got != connString {
		t.Fatalf("reopen connstring = %q", got)
	}
	if got := manager.Snapshot().Status; got != device.StatusPolling {
		t.Fatalf("status after recovery = %q", got)
	}
	manager.Shutdown()
}

func TestWithReleasedDeviceCancellationStillReopens(t *testing.T) {
	first, reopened := newReleasedReader(), newReleasedReader()
	readers := []nfc.Reader{first, reopened}
	var factoryIndex atomic.Int32
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader {
		return readers[int(factoryIndex.Add(1))-1]
	}, nil, testConfig())
	if err := manager.Connect(context.Background(), "pn532_uart:/dev/cancel"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan device.ReleasedDeviceResult, 1)
	go func() {
		done <- manager.WithReleasedDevice(ctx, device.ReleasedDeviceHooks{}, func(operationCtx context.Context, _ device.ReleasedDevice) error {
			close(started)
			<-operationCtx.Done()
			return operationCtx.Err()
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("released operation did not start")
	}
	cancel()
	select {
	case result := <-done:
		if !errors.Is(result.OperationError, context.Canceled) || result.RecoveryError != nil {
			t.Fatalf("cancel result = %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled released operation did not finish")
	}
	if factoryIndex.Load() != 2 || manager.Snapshot().Status != device.StatusPolling {
		t.Fatalf("reader was not reopened after cancellation: calls=%d status=%s", factoryIndex.Load(), manager.Snapshot().Status)
	}
	manager.Shutdown()
}

func TestWithReleasedDeviceReportsRecoverySeparately(t *testing.T) {
	first := newReleasedReader()
	var factoryCalls atomic.Int32
	manager := device.NewManager(mock.NewEnumerator(nil, nil), func() nfc.Reader {
		if factoryCalls.Add(1) == 1 {
			return first
		}
		failed := newReleasedReader()
		failed.openErr = nfc.ErrPermission
		return failed
	}, nil, device.Config{
		PollInterval: time.Millisecond, PollTimeout: 50 * time.Millisecond, OpenTimeout: 20 * time.Millisecond,
		RemovalMisses: 2, ReconnectBackoffs: []time.Duration{time.Millisecond},
	})
	if err := manager.Connect(context.Background(), "pn532_uart:/dev/recovery-fail"); err != nil {
		t.Fatal(err)
	}
	result := manager.WithReleasedDevice(context.Background(), device.ReleasedDeviceHooks{}, func(context.Context, device.ReleasedDevice) error { return nil })
	if result.OperationError != nil || !errors.Is(result.RecoveryError, nfc.ErrPermission) {
		t.Fatalf("result = %+v", result)
	}
	if manager.Snapshot().Status != device.StatusError {
		t.Fatalf("status = %s; want error", manager.Snapshot().Status)
	}
}

func containsStatus(statuses []device.Status, wanted device.Status) bool {
	for _, status := range statuses {
		if status == wanted {
			return true
		}
	}
	return false
}
