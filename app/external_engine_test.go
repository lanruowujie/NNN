package app

import (
	"context"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/attack"
)

func TestExternalCancellationCannotEmitCompletion(t *testing.T) {
	emitter := newAnyRecordingEmitter()
	service := &Service{emitter: emitter, operations: make(map[string]*operationTask)}
	started := make(chan struct{})
	task, err := service.startOperation("external_fixture", "fixture", func(ctx context.Context, _ string) error {
		close(started)
		<-ctx.Done()
		return &attack.Error{Code: attack.CodeCancelled, Engine: "fixture", ExitCode: -1, Cause: ctx.Err()}
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("external task did not start")
	}
	if err := service.CancelTask(task.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var cancelled, completed bool
		emitter.mu.Lock()
		for _, event := range emitter.events {
			payload, ok := event.payload.(TaskEventDetailDTO)
			if !ok || payload.TaskID != task.ID {
				continue
			}
			cancelled = cancelled || payload.Type == TaskEventCancelled
			completed = completed || payload.Type == TaskEventCompleted
		}
		emitter.mu.Unlock()
		if completed {
			t.Fatal("cancelled external task emitted completion")
		}
		if cancelled {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("cancelled external task did not emit cancellation")
}

func TestExternalAttackEventDTOSeparatesRawStreamFromStatus(t *testing.T) {
	dto := externalAttackEventDTO("task-1", attack.AttackEvent{
		Engine: "fixture", State: attack.StateRunning, Stream: attack.StreamStdout,
		Data: "partial output", Message: "specific progress", Sequence: 3, Time: time.Unix(1, 0).UTC(),
	})
	if dto.Log != "partial output" || dto.Stream != "stdout" || dto.Phase != "running" || dto.Sequence != 3 || dto.Message != "specific progress" {
		t.Fatalf("DTO = %+v", dto)
	}
}
