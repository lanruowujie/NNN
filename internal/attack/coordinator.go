package attack

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/nfc"
)

const defaultTimeout = 30 * time.Minute

// Coordinator performs the device-to-process ownership transaction for one
// engine. It is synchronous; the application layer owns GUI goroutines and
// task IDs.
type Coordinator struct {
	devices ReleasedDeviceController
	engine  AttackEngine
}

func NewCoordinator(devices ReleasedDeviceController, engine AttackEngine) *Coordinator {
	return &Coordinator{devices: devices, engine: engine}
}

func (c *Coordinator) Available(ctx context.Context) error {
	if c == nil || c.engine == nil {
		return &Error{Code: CodeUnavailable, ExitCode: -1, Detail: "engine is not configured"}
	}
	if err := c.engine.Available(ctx); err != nil {
		return &Error{Code: CodeUnavailable, Engine: c.engine.Name(), ExitCode: -1, Detail: err.Error(), Cause: err}
	}
	return nil
}

func (c *Coordinator) Run(ctx context.Context, request AttackRequest, emit func(AttackEvent)) (AttackResult, error) {
	result := AttackResult{ExitCode: -1}
	if ctx == nil || c == nil || c.devices == nil || c.engine == nil {
		err := &Error{Code: CodeUnavailable, ExitCode: -1, Detail: "external engine coordinator is not configured"}
		result.State, result.OperationError = StateFailed, err
		return result, err
	}
	engineName := c.engine.Name()
	result.Engine = engineName
	sequenced := newSequencedEmitter(emit)
	sequenced(AttackEvent{Engine: result.Engine, State: StateQueued, Message: "external task queued"})
	if err := c.Available(ctx); err != nil {
		result.State, result.OperationError = StateFailed, err
		sequenced(AttackEvent{Engine: result.Engine, State: StateFailed, Message: err.Error()})
		return result, err
	}

	timeout := request.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	temporaryDir, err := os.MkdirTemp(request.TemporaryRoot, temporaryPrefix(request.TaskID))
	if err != nil {
		err = &Error{Code: CodeTemporary, Engine: result.Engine, ExitCode: -1, Detail: "create task temporary directory", Cause: err}
		result.State, result.OperationError = StateFailed, err
		sequenced(AttackEvent{Engine: result.Engine, State: StateFailed, Message: err.Error()})
		return result, err
	}
	result.TemporaryDir = temporaryDir
	request.WorkingDir = temporaryDir

	cancelWatchDone := make(chan struct{})
	var cancelWatch sync.WaitGroup
	var cancellingOnce sync.Once
	emitCancelling := func() {
		cancellingOnce.Do(func() {
			sequenced(AttackEvent{Engine: engineName, State: StateCancelling, Message: "terminating external process"})
		})
	}
	cancelWatch.Add(1)
	go func() {
		defer cancelWatch.Done()
		select {
		case <-runCtx.Done():
			emitCancelling()
		case <-cancelWatchDone:
		}
	}()

	hooks := device.ReleasedDeviceHooks{
		Prepare: func(prepareCtx context.Context, reader nfc.Reader) error {
			if request.ExpectedCard == nil {
				return nil
			}
			sequenced(AttackEvent{Engine: result.Engine, State: StatePreflight, Message: "reselecting the expected card before release"})
			current, err := reader.CardInfo(prepareCtx)
			if err != nil {
				return err
			}
			if !nfc.SameCard(*request.ExpectedCard, current) {
				return nfc.NewError("external task preflight", nfc.CodeCardChanged, "card identity changed before external process launch", nil)
			}
			return nil
		},
		Releasing: func() {
			sequenced(AttackEvent{Engine: result.Engine, State: StateReleasingDevice, Message: "stopping polling and closing the reader"})
		},
		Reopening: func() {
			sequenced(AttackEvent{Engine: result.Engine, State: StateReopeningDevice, Message: "reopening the reader"})
		},
	}
	deviceResult := c.devices.WithReleasedDevice(runCtx, hooks, func(operationCtx context.Context, released device.ReleasedDevice) error {
		request.Device = released.Device
		engineResult, runErr := c.engine.Run(operationCtx, request, sequenced)
		result = engineResult
		result.Engine = c.engine.Name()
		result.TemporaryDir = temporaryDir
		return runErr
	})
	close(cancelWatchDone)
	if runCtx.Err() != nil {
		emitCancelling()
	}
	cancelWatch.Wait()
	result.OperationError = deviceResult.OperationError
	result.RecoveryError = deviceResult.RecoveryError
	if result.FinishedAt.IsZero() {
		result.FinishedAt = time.Now().UTC()
	}

	operationErr := normalizeOperationError(ctx, runCtx, result, deviceResult.OperationError)
	result.OperationError = operationErr
	state := StateSucceeded
	if operationErr != nil {
		state = StateFailed
		if errors.Is(operationErr, ErrCancelled) {
			state = StateCancelled
		}
	}
	result.State = state

	keepTemporary := request.TemporaryPolicy == KeepOnFailure && state == StateFailed
	if !keepTemporary {
		if cleanupErr := os.RemoveAll(temporaryDir); cleanupErr != nil && operationErr == nil {
			operationErr = &Error{Code: CodeTemporary, Engine: result.Engine, ExitCode: result.ExitCode, Version: result.Version, Detail: "clean task temporary directory", Cause: cleanupErr}
			result.OperationError = operationErr
			result.State = StateFailed
		}
		result.TemporaryDir = ""
	}

	message := "external task completed"
	if operationErr != nil {
		message = operationErr.Error()
	}
	sequenced(AttackEvent{Engine: result.Engine, State: result.State, Message: message})
	return result, operationErr
}

func normalizeOperationError(parent, run context.Context, result AttackResult, err error) error {
	if errors.Is(parent.Err(), context.Canceled) {
		return &Error{Code: CodeCancelled, Engine: result.Engine, ExitCode: result.ExitCode, Version: result.Version, Detail: "cancelled by user", Cause: parent.Err()}
	}
	if errors.Is(run.Err(), context.DeadlineExceeded) {
		return &Error{Code: CodeTimeout, Engine: result.Engine, ExitCode: result.ExitCode, Version: result.Version, Detail: "task deadline exceeded", Cause: run.Err()}
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, nfc.ErrCanceled) || errors.Is(err, ErrCancelled) {
		return &Error{Code: CodeCancelled, Engine: result.Engine, ExitCode: result.ExitCode, Version: result.Version, Detail: "external operation cancelled", Cause: err}
	}
	return err
}

func temporaryPrefix(taskID string) string {
	var builder strings.Builder
	builder.WriteString("nfcx-attack-")
	for _, value := range taskID {
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '-' || value == '_' {
			builder.WriteRune(value)
		}
		if builder.Len() >= 48 {
			break
		}
	}
	if builder.Len() == len("nfcx-attack-") {
		builder.WriteString("task")
	}
	builder.WriteByte('-')
	return builder.String()
}

func newSequencedEmitter(emit func(AttackEvent)) func(AttackEvent) {
	var mutex sync.Mutex
	var sequence uint64
	return func(event AttackEvent) {
		if emit == nil {
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		sequence++
		event.Sequence = sequence
		if event.Time.IsZero() {
			event.Time = time.Now().UTC()
		}
		emit(event)
	}
}
