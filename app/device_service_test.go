package app

import (
	"sync"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
)

type anyRecordedEvent struct {
	name    string
	payload any
}

type anyRecordingEmitter struct {
	mu     sync.Mutex
	events []anyRecordedEvent
	notify chan struct{}
}

func newAnyRecordingEmitter() *anyRecordingEmitter {
	return &anyRecordingEmitter{notify: make(chan struct{}, 128)}
}

func (e *anyRecordingEmitter) Emit(name string, payload any) {
	e.mu.Lock()
	e.events = append(e.events, anyRecordedEvent{name: name, payload: payload})
	e.mu.Unlock()
	select {
	case e.notify <- struct{}{}:
	default:
	}
}

func (e *anyRecordingEmitter) count(name string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	count := 0
	for _, event := range e.events {
		if event.name == name {
			count++
		}
	}
	return count
}

func TestServiceRefreshConnectAndCardEvents(t *testing.T) {
	emitter := newAnyRecordingEmitter()
	connString := " pn532_uart:/dev/exact path "
	enumerator := mock.NewEnumerator([]nfc.DeviceInfo{{Name: "PN532", ConnString: connString}}, nil)
	reader := mock.NewReader()
	reader.SetCardResult(nfc.CardInfo{UID: []byte{4, 1, 2, 3}, ATQA: [2]byte{0, 4}, SAK: 0x08}, nil)
	manager := device.NewManager(enumerator, func() nfc.Reader { return reader }, deviceEventSink{emitter: emitter}, device.Config{
		PollInterval:      time.Millisecond,
		PollTimeout:       20 * time.Millisecond,
		OpenTimeout:       20 * time.Millisecond,
		RemovalMisses:     2,
		ReconnectBackoffs: []time.Duration{time.Millisecond},
	})
	service := &Service{
		emitter:       emitter,
		tasks:         make(map[string]*mockTask),
		config:        mockTaskConfig{steps: 1, stepInterval: time.Millisecond},
		deviceManager: manager,
		deviceTimeout: 100 * time.Millisecond,
	}
	t.Cleanup(service.Shutdown)

	devices, err := service.RefreshDevices()
	if err != nil {
		t.Fatalf("refresh devices: %v", err)
	}
	if len(devices) != 1 || devices[0].ConnString != connString || devices[0].Name != "PN532" {
		t.Fatalf("unexpected device DTOs: %+v", devices)
	}
	if err := service.ConnectDevice(connString); err != nil {
		t.Fatalf("connect device: %v", err)
	}

	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for emitter.count(CardEventName) == 0 {
		select {
		case <-emitter.notify:
		case <-deadline.C:
			t.Fatal("timed out waiting for card event")
		}
	}
	time.Sleep(10 * time.Millisecond)
	if got := emitter.count(CardEventName); got != 1 {
		t.Fatalf("same card emitted %d events; want one appeared event", got)
	}

	dashboard := service.Dashboard()
	if !dashboard.Card.Present || dashboard.Card.UIDLength != 4 || dashboard.Connection.Status != string(device.StatusPolling) {
		t.Fatalf("unexpected connected dashboard: %+v", dashboard)
	}
	open, gotConnString, _, _ := reader.State()
	if !open || gotConnString != connString {
		t.Fatalf("reader state open=%v conn=%q; exact connstring was not preserved", open, gotConnString)
	}
}
