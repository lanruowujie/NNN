package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/workbench"
	"github.com/BennyThink/NFCX/internal/workflow"
)

type orderedEmitter struct {
	mu    sync.Mutex
	names []string
}

func (e *orderedEmitter) Emit(name string, _ any) {
	e.mu.Lock()
	e.names = append(e.names, name)
	e.mu.Unlock()
}

func (e *orderedEmitter) first() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.names) == 0 {
		return ""
	}
	return e.names[0]
}

func TestPreparedOperationClearsWorkbenchBeforeWorkerStarts(t *testing.T) {
	image, err := mifare.NewDump(mifare.Classic1K)
	if err != nil {
		t.Fatal(err)
	}
	model := workbench.New()
	if err := model.Load(workflow.DumpResult{Dump: image}, "old-card.bin"); err != nil {
		t.Fatal(err)
	}
	emitter := &orderedEmitter{}
	service := &Service{emitter: emitter, operations: make(map[string]*operationTask), bench: model}
	workerSawLoaded := make(chan bool, 1)

	if _, err := service.startPreparedOperation("read", "读取整卡", service.clearWorkbench, func(context.Context, string) error {
		workerSawLoaded <- service.bench.Snapshot().Loaded
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if service.bench.Snapshot().Loaded {
		t.Fatal("accepted operation returned before the old workbench was cleared")
	}
	select {
	case loaded := <-workerSawLoaded:
		if loaded {
			t.Fatal("hardware worker started while the old workbench was still loaded")
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	if first := emitter.first(); first != WorkbenchEventName {
		t.Fatalf("first event = %q, want %q", first, WorkbenchEventName)
	}
}

func TestRejectedPreparedOperationDoesNotClearWorkbench(t *testing.T) {
	service := &Service{emitter: noopEmitter{}, operations: map[string]*operationTask{"busy": {}}, bench: workbench.New()}
	var prepared atomic.Bool
	_, err := service.startPreparedOperation("scan", "扫描已知密钥", func() { prepared.Store(true) }, func(context.Context, string) error { return nil })
	if !errors.Is(err, ErrOperationRunning) {
		t.Fatalf("error = %v, want ErrOperationRunning", err)
	}
	if prepared.Load() {
		t.Fatal("rejected operation applied its preparation transition")
	}
}
