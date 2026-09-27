package device

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BennyThink/NFCX/internal/nfc"
)

// Status is the stable device lifecycle state exposed to application services.
type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusReady        Status = "ready"
	StatusPolling      Status = "polling"
	StatusBusy         Status = "busy"
	StatusRecovering   Status = "recovering"
	StatusError        Status = "error"
)

type CardEventType string

const (
	CardAppeared CardEventType = "appeared"
	CardRemoved  CardEventType = "removed"
)

// Capabilities are deliberately conservative. A reader being enumerable or
// able to perform ordinary Classic I/O does not imply that it supports the
// raw parity/timing behaviour required by Nested attacks.
type Capabilities struct {
	Nested     bool
	Darkside   bool
	Hardnested bool
}

// CapabilitiesFor returns the explicitly supported capabilities for a reader
// profile. Unknown profiles keep the zero-value (all capabilities disabled).
func CapabilitiesFor(info nfc.DeviceInfo) Capabilities {
	pn532UART := strings.HasPrefix(strings.ToLower(info.ConnString), "pn532_uart:")
	return Capabilities{Nested: pn532UART, Darkside: pn532UART, Hardnested: pn532UART}
}

// Snapshot is an immutable copy of the manager state.
type Snapshot struct {
	Status       Status
	Device       nfc.DeviceInfo
	Card         *nfc.CardInfo
	CardLastSeen time.Time
	ErrorCode    nfc.ErrorCode
	ErrorMessage string
	Detail       string
	Capabilities Capabilities
	UpdatedAt    time.Time
}

// CardEvent reports only meaningful presence changes. Repeated sightings of
// the same card update Snapshot.CardLastSeen without producing another event.
type CardEvent struct {
	Type CardEventType
	Card nfc.CardInfo
	Time time.Time
}

// EventSink keeps the device package independent of Wails.
type EventSink interface {
	DeviceStateChanged(Snapshot)
	CardChanged(CardEvent)
}

type noopSink struct{}

func (noopSink) DeviceStateChanged(Snapshot) {}
func (noopSink) CardChanged(CardEvent)       {}

type ReaderFactory func() nfc.Reader

// ReleasedDevice is a snapshot passed to an external operation after the
// in-process reader has been closed. ConnString is preserved byte-for-byte.
type ReleasedDevice struct {
	Device       nfc.DeviceInfo
	Card         *nfc.CardInfo
	Capabilities Capabilities
}

// ReleasedDeviceHooks let the workflow publish its finer-grained task states
// without coupling the device package to an external engine package.
type ReleasedDeviceHooks struct {
	Releasing func()
	Reopening func()
	// Prepare runs while the reader is still open, after polling has stopped
	// and while the global device lock is held. Returning an error prevents the
	// external process from starting.
	Prepare func(context.Context, nfc.Reader) error
}

// ReleasedDeviceResult keeps the engine outcome separate from a later reader
// recovery failure.
type ReleasedDeviceResult struct {
	OperationError error
	RecoveryError  error
}

// Config controls bounded polling and recovery. Zero values use safe defaults.
type Config struct {
	PollInterval      time.Duration
	PollTimeout       time.Duration
	OpenTimeout       time.Duration
	RemovalMisses     int
	ReconnectBackoffs []time.Duration
}

func DefaultConfig() Config {
	return Config{
		PollInterval:      250 * time.Millisecond,
		PollTimeout:       750 * time.Millisecond,
		OpenTimeout:       3 * time.Second,
		RemovalMisses:     2,
		ReconnectBackoffs: []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second},
	}
}

// Manager owns exactly one selected Reader and its polling goroutine.
type Manager struct {
	enumerator nfc.DeviceEnumerator
	newReader  ReaderFactory
	sink       EventSink
	config     Config

	lifecycleMu sync.Mutex
	stateMu     sync.Mutex
	operation   chan struct{}

	status          Status
	device          nfc.DeviceInfo
	reader          nfc.Reader
	card            *nfc.CardInfo
	cardLastSeen    time.Time
	errorCode       nfc.ErrorCode
	errorMessage    string
	detail          string
	updatedAt       time.Time
	pollCancel      context.CancelFunc
	pollDone        chan struct{}
	operationCancel context.CancelFunc
	externalActive  bool
}

func NewManager(enumerator nfc.DeviceEnumerator, newReader ReaderFactory, sink EventSink, config Config) *Manager {
	defaults := DefaultConfig()
	if config.PollInterval <= 0 {
		config.PollInterval = defaults.PollInterval
	}
	if config.PollTimeout <= 0 {
		config.PollTimeout = defaults.PollTimeout
	}
	if config.OpenTimeout <= 0 {
		config.OpenTimeout = defaults.OpenTimeout
	}
	if config.RemovalMisses < 1 {
		config.RemovalMisses = defaults.RemovalMisses
	}
	if len(config.ReconnectBackoffs) == 0 {
		config.ReconnectBackoffs = append([]time.Duration(nil), defaults.ReconnectBackoffs...)
	} else {
		config.ReconnectBackoffs = append([]time.Duration(nil), config.ReconnectBackoffs...)
	}
	if sink == nil {
		sink = noopSink{}
	}
	return &Manager{
		enumerator: enumerator,
		newReader:  newReader,
		sink:       sink,
		config:     config,
		operation:  make(chan struct{}, 1),
		status:     StatusDisconnected,
		updatedAt:  time.Now().UTC(),
	}
}

// ListDevices enumerates readers without altering the active connection. Exact
// connstrings are preserved and duplicate results are removed in source order.
func (m *Manager) ListDevices(ctx context.Context) ([]nfc.DeviceInfo, error) {
	if m.enumerator == nil {
		return nil, nfc.NewError("list devices", nfc.CodeUnsupported, "device enumeration is unavailable", nil)
	}
	m.stateMu.Lock()
	externalActive := m.externalActive
	m.stateMu.Unlock()
	if externalActive {
		return nil, nfc.NewError("list devices", nfc.CodeBusy, "reader is owned by an external engine", nil)
	}
	devices, err := m.enumerator.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(devices))
	result := make([]nfc.DeviceInfo, 0, len(devices))
	for _, candidate := range devices {
		if candidate.ConnString == "" {
			continue
		}
		if _, exists := seen[candidate.ConnString]; exists {
			continue
		}
		seen[candidate.ConnString] = struct{}{}
		result = append(result, candidate)
	}
	return result, nil
}

// Connect replaces any prior connection and starts a cancellable poll loop.
// connString is validated but passed to the Reader byte-for-byte unchanged.
func (m *Manager) Connect(ctx context.Context, connString string) error {
	if ctx == nil {
		return nfc.NewError("connect", nfc.CodeInvalidArgument, "nil context", nil)
	}
	if strings.TrimSpace(connString) == "" || strings.IndexByte(connString, 0) >= 0 {
		return nfc.NewError("connect", nfc.CodeInvalidArgument, "connstring must be non-empty and contain no NUL byte", nil)
	}
	if m.newReader == nil {
		return nfc.NewError("connect", nfc.CodeUnsupported, "reader backend is unavailable", nil)
	}

	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if err := m.stopSession(); err != nil {
		m.transition(StatusError, "关闭之前的读卡器失败", err)
		return err
	}
	m.setDevice(nfc.DeviceInfo{Name: connString, ConnString: connString})
	m.transition(StatusConnecting, "正在连接读卡器", nil)

	reader := m.newReader()
	if reader == nil {
		err := nfc.NewError("connect", nfc.CodeInternal, "reader factory returned nil", nil)
		m.transition(StatusError, "无法创建设备句柄", err)
		return err
	}
	if err := reader.Open(ctx, connString); err != nil {
		m.transition(StatusError, "连接读卡器失败", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = reader.Close()
		mapped := contextNFCError("connect", err)
		m.transition(StatusError, "连接已取消", mapped)
		return mapped
	}

	deviceInfo := describeReader(reader, connString)
	m.stateMu.Lock()
	m.reader = reader
	m.device = deviceInfo
	m.stateMu.Unlock()
	m.transition(StatusReady, "读卡器已就绪", nil)
	m.startPolling(reader, connString)
	return nil
}

// Disconnect stops all background work, waits for it to exit, and closes the
// Reader. It is safe to call repeatedly.
func (m *Manager) Disconnect() error {
	m.cancelActiveOperation()
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	err := m.stopSession()
	m.transition(StatusDisconnected, "未连接读卡器", nil)
	return err
}

// Shutdown is the application-lifecycle alias for Disconnect.
func (m *Manager) Shutdown() { _ = m.Disconnect() }

// WithReader runs one exclusive in-process operation. Polling cannot touch the
// device during the callback, and the GUI observes the busy state.
func (m *Manager) WithReader(ctx context.Context, operation func(context.Context, nfc.Reader) error) error {
	if ctx == nil || operation == nil {
		return nfc.NewError("device operation", nfc.CodeInvalidArgument, "context and operation are required", nil)
	}
	m.stateMu.Lock()
	externalActive := m.externalActive
	m.stateMu.Unlock()
	if externalActive {
		return nfc.NewError("device operation", nfc.CodeBusy, "reader is owned by an external engine", nil)
	}
	if err := m.acquire(ctx); err != nil {
		return err
	}
	defer m.release()

	operationCtx, cancel := context.WithCancel(ctx)
	m.stateMu.Lock()
	reader := m.reader
	status := m.status
	if reader != nil && (status == StatusReady || status == StatusPolling) {
		m.operationCancel = cancel
	}
	m.stateMu.Unlock()
	if reader == nil || (status != StatusReady && status != StatusPolling) {
		cancel()
		if status == StatusRecovering || status == StatusConnecting {
			return nfc.NewError("device operation", nfc.CodeBusy, "reader is changing state", nil)
		}
		return nfc.NewError("device operation", nfc.CodeNotOpen, "reader is not connected", nil)
	}
	m.transition(StatusBusy, "读卡器正在执行独占操作", nil)
	err := operation(operationCtx, reader)
	cancel()
	m.stateMu.Lock()
	m.operationCancel = nil
	stillCurrent := m.reader == reader && m.status == StatusBusy
	m.stateMu.Unlock()
	if stillCurrent {
		m.transition(StatusPolling, "正在检测卡片", nil)
	}
	return err
}

// WithReleasedDevice transfers exclusive ownership from the in-process reader
// to an external operation. Polling is stopped and the reader is closed before
// operation runs. Reopening uses a cleanup context, so cancellation or timeout
// of the operation cannot skip device recovery.
func (m *Manager) WithReleasedDevice(ctx context.Context, hooks ReleasedDeviceHooks, operation func(context.Context, ReleasedDevice) error) ReleasedDeviceResult {
	result := ReleasedDeviceResult{}
	if ctx == nil || operation == nil {
		result.OperationError = nfc.NewError("external device operation", nfc.CodeInvalidArgument, "context and operation are required", nil)
		return result
	}
	operationCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if err := m.acquire(operationCtx); err != nil {
		result.OperationError = err
		return result
	}
	defer m.release()

	m.stateMu.Lock()
	reader := m.reader
	status := m.status
	deviceInfo := m.device
	card := cloneCardPointer(m.card)
	pollCancel := m.pollCancel
	pollDone := m.pollDone
	if reader == nil || (status != StatusReady && status != StatusPolling) {
		m.stateMu.Unlock()
		if status == StatusBusy || status == StatusRecovering || status == StatusConnecting {
			result.OperationError = nfc.NewError("external device operation", nfc.CodeBusy, "reader is changing state", nil)
		} else {
			result.OperationError = nfc.NewError("external device operation", nfc.CodeNotOpen, "reader is not connected", nil)
		}
		return result
	}
	m.externalActive = true
	m.operationCancel = cancel
	m.pollCancel = nil
	m.pollDone = nil
	m.stateMu.Unlock()

	defer func() {
		m.stateMu.Lock()
		m.externalActive = false
		m.operationCancel = nil
		m.stateMu.Unlock()
	}()
	if hooks.Releasing != nil {
		hooks.Releasing()
	}
	m.transition(StatusBusy, "正在释放读卡器给外部任务", nil)
	if pollCancel != nil {
		pollCancel()
	}
	if pollDone != nil {
		<-pollDone
	}
	if hooks.Prepare != nil {
		if err := hooks.Prepare(operationCtx, reader); err != nil {
			result.OperationError = err
		}
	}
	m.stateMu.Lock()
	if m.reader == reader {
		m.reader = nil
	}
	m.stateMu.Unlock()
	if err := reader.Close(); err != nil && result.OperationError == nil {
		result.OperationError = err
	} else if result.OperationError != nil {
		// Preparation already rejected the task. Closing and reopening still
		// resets the reader to the same well-defined state as other failures.
	} else if err := operationCtx.Err(); err != nil {
		result.OperationError = contextNFCError("external device operation", err)
	} else {
		result.OperationError = operation(operationCtx, ReleasedDevice{Device: deviceInfo, Card: card, Capabilities: CapabilitiesFor(deviceInfo)})
	}

	if hooks.Reopening != nil {
		hooks.Reopening()
	}
	m.transition(StatusRecovering, "正在重新打开读卡器", nil)
	result.RecoveryError = m.reopenReleasedDevice(deviceInfo.ConnString)
	return result
}

func (m *Manager) Snapshot() Snapshot {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.snapshotLocked()
}

func (m *Manager) startPolling(reader nfc.Reader, connString string) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.stateMu.Lock()
	m.pollCancel = cancel
	m.pollDone = done
	m.stateMu.Unlock()
	m.transition(StatusPolling, "正在检测卡片", nil)
	go m.pollLoop(ctx, done, reader, connString)
}

func (m *Manager) pollLoop(ctx context.Context, done chan struct{}, reader nfc.Reader, connString string) {
	defer close(done)
	ticker := time.NewTicker(m.config.PollInterval)
	defer ticker.Stop()
	misses := 0

	for {
		err := m.pollCard(ctx, reader)
		if ctx.Err() != nil {
			return
		}
		switch {
		case err == nil:
			misses = 0
		case errors.Is(err, nfc.ErrNoCard):
			misses++
			if misses >= m.config.RemovalMisses {
				m.clearCard()
			}
		case errors.Is(err, nfc.ErrTimeout), errors.Is(err, nfc.ErrBusy):
			// A timeout is not proof that a card was removed. Keep polling.
		case shouldReconnect(err):
			misses = 0
			var recovered bool
			reader, recovered = m.recover(ctx, reader, connString, err)
			if !recovered {
				return
			}
		default:
			m.clearCard()
			m.transition(StatusError, "寻卡停止", err)
			_ = reader.Close()
			m.clearReader(reader)
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) pollCard(parent context.Context, reader nfc.Reader) error {
	ctx, cancel := context.WithTimeout(parent, m.config.PollTimeout)
	defer cancel()
	if err := m.acquire(ctx); err != nil {
		return err
	}
	card, err := reader.CardInfo(ctx)
	m.release()
	if err != nil {
		return err
	}
	m.observeCard(card)
	return nil
}

func (m *Manager) recover(ctx context.Context, oldReader nfc.Reader, connString string, cause error) (nfc.Reader, bool) {
	m.transition(StatusError, "读卡器连接中断", cause)
	m.clearCard()
	if err := m.acquire(ctx); err != nil {
		return nil, false
	}
	_ = oldReader.Close()
	m.release()
	m.clearReader(oldReader)

	lastErr := cause
	for attempt, delay := range m.config.ReconnectBackoffs {
		if ctx.Err() != nil {
			return nil, false
		}
		m.transition(StatusRecovering, fmt.Sprintf("正在尝试恢复读卡器（%d/%d）", attempt+1, len(m.config.ReconnectBackoffs)), lastErr)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, false
		case <-timer.C:
		}

		candidate := m.newReader()
		if candidate == nil {
			lastErr = nfc.NewError("recover", nfc.CodeInternal, "reader factory returned nil", nil)
			break
		}
		openCtx, cancel := context.WithTimeout(ctx, m.config.OpenTimeout)
		err := candidate.Open(openCtx, connString)
		cancel()
		if ctx.Err() != nil {
			_ = candidate.Close()
			return nil, false
		}
		if err == nil {
			m.stateMu.Lock()
			m.reader = candidate
			m.device = describeReader(candidate, connString)
			m.stateMu.Unlock()
			m.transition(StatusReady, "读卡器已恢复", nil)
			m.transition(StatusPolling, "正在检测卡片", nil)
			return candidate, true
		}
		_ = candidate.Close()
		lastErr = err
		if !shouldReconnect(err) {
			break
		}
	}
	m.transition(StatusError, "读卡器自动恢复失败", lastErr)
	return nil, false
}

func (m *Manager) stopSession() error {
	m.stateMu.Lock()
	cancel := m.pollCancel
	done := m.pollDone
	reader := m.reader
	operationCancel := m.operationCancel
	m.pollCancel = nil
	m.pollDone = nil
	m.reader = nil
	m.operationCancel = nil
	m.stateMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if operationCancel != nil {
		operationCancel()
	}
	if done != nil {
		<-done
	}
	var err error
	if reader != nil {
		m.operation <- struct{}{}
		err = reader.Close()
		m.release()
	}
	m.clearCard()
	return err
}

func (m *Manager) cancelActiveOperation() {
	m.stateMu.Lock()
	cancel := m.operationCancel
	m.stateMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *Manager) reopenReleasedDevice(connString string) error {
	if m.newReader == nil || connString == "" {
		err := nfc.NewError("reopen reader", nfc.CodeNotOpen, "selected reader is unavailable", nil)
		m.clearCard()
		m.transition(StatusError, "读卡器恢复失败", err)
		return err
	}
	var lastErr error
	for attempt := 0; attempt <= len(m.config.ReconnectBackoffs); attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(m.config.ReconnectBackoffs[attempt-1])
			<-timer.C
		}
		candidate := m.newReader()
		if candidate == nil {
			lastErr = nfc.NewError("reopen reader", nfc.CodeInternal, "reader factory returned nil", nil)
			break
		}
		openCtx, cancel := context.WithTimeout(context.Background(), m.config.OpenTimeout)
		err := candidate.Open(openCtx, connString)
		cancel()
		if err == nil {
			m.stateMu.Lock()
			m.reader = candidate
			m.device = describeReader(candidate, connString)
			m.stateMu.Unlock()
			m.transition(StatusReady, "读卡器已重新打开", nil)
			m.startPolling(candidate, connString)
			return nil
		}
		_ = candidate.Close()
		lastErr = err
		if !shouldReconnect(err) {
			break
		}
		m.transition(StatusRecovering, fmt.Sprintf("正在重试打开读卡器（%d/%d）", attempt+1, len(m.config.ReconnectBackoffs)+1), err)
	}
	if lastErr == nil {
		lastErr = nfc.NewError("reopen reader", nfc.CodeInternal, "reader recovery failed", nil)
	}
	m.clearCard()
	m.transition(StatusError, "读卡器恢复失败", lastErr)
	return lastErr
}

func (m *Manager) observeCard(card nfc.CardInfo) {
	now := time.Now().UTC()
	card = card.Clone()
	m.stateMu.Lock()
	previous := cloneCardPointer(m.card)
	same := previous != nil && cardsEqual(*previous, card)
	m.card = &card
	m.cardLastSeen = now
	m.updatedAt = now
	m.stateMu.Unlock()
	if same {
		return
	}
	if previous != nil {
		m.sink.CardChanged(CardEvent{Type: CardRemoved, Card: *previous, Time: now})
	}
	m.sink.CardChanged(CardEvent{Type: CardAppeared, Card: card.Clone(), Time: now})
}

func (m *Manager) clearCard() {
	now := time.Now().UTC()
	m.stateMu.Lock()
	previous := cloneCardPointer(m.card)
	m.card = nil
	m.cardLastSeen = time.Time{}
	m.updatedAt = now
	m.stateMu.Unlock()
	if previous != nil {
		m.sink.CardChanged(CardEvent{Type: CardRemoved, Card: *previous, Time: now})
	}
}

func (m *Manager) setDevice(info nfc.DeviceInfo) {
	m.stateMu.Lock()
	m.device = info
	m.stateMu.Unlock()
}

func (m *Manager) clearReader(reader nfc.Reader) {
	m.stateMu.Lock()
	if m.reader == reader {
		m.reader = nil
	}
	m.stateMu.Unlock()
}

func (m *Manager) transition(status Status, detail string, err error) {
	now := time.Now().UTC()
	m.stateMu.Lock()
	m.status = status
	m.detail = detail
	m.errorCode = nfc.CodeUnknown
	m.errorMessage = ""
	if err != nil {
		m.errorMessage = err.Error()
		if code, ok := nfc.ErrorCodeOf(err); ok {
			m.errorCode = code
		}
	}
	m.updatedAt = now
	snapshot := m.snapshotLocked()
	m.stateMu.Unlock()
	m.sink.DeviceStateChanged(snapshot)
}

func (m *Manager) snapshotLocked() Snapshot {
	return Snapshot{
		Status:       m.status,
		Device:       m.device,
		Card:         cloneCardPointer(m.card),
		CardLastSeen: m.cardLastSeen,
		ErrorCode:    m.errorCode,
		ErrorMessage: m.errorMessage,
		Detail:       m.detail,
		Capabilities: CapabilitiesFor(m.device),
		UpdatedAt:    m.updatedAt,
	}
}

func (m *Manager) acquire(ctx context.Context) error {
	select {
	case m.operation <- struct{}{}:
		return nil
	case <-ctx.Done():
		return contextNFCError("acquire device", ctx.Err())
	}
}

func (m *Manager) release() { <-m.operation }

func describeReader(reader nfc.Reader, connString string) nfc.DeviceInfo {
	if describer, ok := reader.(nfc.DeviceDescriber); ok {
		if described := describer.DeviceInfo(); described.ConnString != "" {
			return described
		}
	}
	return nfc.DeviceInfo{Name: connString, ConnString: connString}
}

func cloneCardPointer(card *nfc.CardInfo) *nfc.CardInfo {
	if card == nil {
		return nil
	}
	clone := card.Clone()
	return &clone
}

func cardsEqual(left, right nfc.CardInfo) bool {
	return bytes.Equal(left.UID, right.UID) && left.ATQA == right.ATQA && left.SAK == right.SAK
}

func shouldReconnect(err error) bool {
	return errors.Is(err, nfc.ErrDeviceDisconnected) || errors.Is(err, nfc.ErrIO) || errors.Is(err, nfc.ErrTimeout)
}

func contextNFCError(op string, err error) error {
	code := nfc.CodeCanceled
	if errors.Is(err, context.DeadlineExceeded) {
		code = nfc.CodeTimeout
	}
	return nfc.NewError(op, code, "", err)
}
