package app

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type recordedEvent struct {
	name    string
	payload TaskEventDTO
}

type recordingEmitter struct {
	mu     sync.Mutex
	events []recordedEvent
	notify chan struct{}
}

func newRecordingEmitter() *recordingEmitter {
	return &recordingEmitter{notify: make(chan struct{}, 32)}
}

func (e *recordingEmitter) Emit(name string, payload any) {
	e.mu.Lock()
	e.events = append(e.events, recordedEvent{name: name, payload: payload.(TaskEventDTO)})
	e.mu.Unlock()
	e.notify <- struct{}{}
}

func (e *recordingEmitter) snapshot() []recordedEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]recordedEvent(nil), e.events...)
}

func waitForEvent(t *testing.T, emitter *recordingEmitter, eventType string) TaskEventDTO {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		for _, event := range emitter.snapshot() {
			if event.payload.Type == eventType {
				return event.payload
			}
		}
		select {
		case <-emitter.notify:
		case <-deadline:
			t.Fatalf("timed out waiting for %q event; got %+v", eventType, emitter.snapshot())
		}
	}
}

func TestMockTaskEmitsStartProgressAndCompletion(t *testing.T) {
	emitter := newRecordingEmitter()
	service := newService(emitter, mockTaskConfig{steps: 3, stepInterval: time.Millisecond})
	task := service.StartMockTask()

	completed := waitForEvent(t, emitter, TaskEventCompleted)
	if completed.TaskID != task.ID || completed.Progress != 100 {
		t.Fatalf("unexpected completion: %+v", completed)
	}

	events := emitter.snapshot()
	if events[0].name != MockTaskEventName || events[0].payload.Type != TaskEventStarted {
		t.Fatalf("first event must be started: %+v", events[0])
	}
	progressSeen := false
	for _, event := range events {
		if event.payload.Type == TaskEventProgress {
			progressSeen = true
		}
	}
	if !progressSeen {
		t.Fatal("expected at least one progress event")
	}
}

func TestCancelledMockTaskNeverCompletes(t *testing.T) {
	emitter := newRecordingEmitter()
	service := newService(emitter, mockTaskConfig{steps: 20, stepInterval: 5 * time.Millisecond})
	task := service.StartMockTask()
	waitForEvent(t, emitter, TaskEventStarted)

	if err := service.CancelMockTask(task.ID); err != nil {
		t.Fatalf("cancel task: %v", err)
	}
	waitForEvent(t, emitter, TaskEventCancelled)
	time.Sleep(25 * time.Millisecond)

	for _, event := range emitter.snapshot() {
		if event.payload.Type == TaskEventCompleted {
			t.Fatalf("cancelled task emitted completion: %+v", event.payload)
		}
	}
}

func TestCancelMockTaskValidatesID(t *testing.T) {
	service := NewService(nil)
	if err := service.CancelMockTask(""); !errors.Is(err, ErrTaskIDRequired) {
		t.Fatalf("expected ErrTaskIDRequired, got %v", err)
	}
	if err := service.CancelMockTask("missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}
