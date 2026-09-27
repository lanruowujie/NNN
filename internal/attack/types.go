package attack

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/nfc"
)

type State string

const (
	StateQueued           State = "queued"
	StatePreflight        State = "preflight"
	StateReleasingDevice  State = "releasing_device"
	StateStarting         State = "starting"
	StateRunning          State = "running"
	StateCancelling       State = "cancelling"
	StateValidatingResult State = "validating_result"
	StateReopeningDevice  State = "reopening_device"
	StateVerifyingKeys    State = "verifying_keys"
	StateMergingResult    State = "merging_result"
	StateMFCUKStage       State = "mfcuk_stage"
	StateVerifyingSeed    State = "verifying_seed"
	StateMFoCStage        State = "mfoc_stage"
	StateReadingDump      State = "reading_dump"
	StateSucceeded        State = "succeeded"
	StateFailed           State = "failed"
	StateCancelled        State = "cancelled"
)

type Stream string

const (
	StreamStdout Stream = "stdout"
	StreamStderr Stream = "stderr"
)

type AttackEvent struct {
	Engine   string
	State    State
	Stream   Stream
	Data     string
	Message  string
	Sequence uint64
	Time     time.Time
}

type TemporaryPolicy string

const (
	CleanupAlways TemporaryPolicy = "cleanup_always"
	KeepOnFailure TemporaryPolicy = "keep_on_failure"
)

// AttackRequest contains algorithm-neutral execution facts. It intentionally has no
// executable path or raw argument list; those belong to a bundled adapter.
type AttackRequest struct {
	TaskID          string
	Device          nfc.DeviceInfo
	Timeout         time.Duration
	TemporaryRoot   string
	TemporaryPolicy TemporaryPolicy
	WorkingDir      string
	// ExpectedCard is reselected under the device lock immediately before the
	// in-process reader is closed. It prevents a card swap between GUI preflight
	// and external process launch.
	ExpectedCard *nfc.CardInfo
}

// AttackArtifact carries a small validated engine output across temporary
// directory cleanup. External engine dumps are bounded to Classic card sizes.
type AttackArtifact struct {
	Name      string
	MediaType string
	Data      []byte
}

type AttackResult struct {
	Engine         string
	Version        string
	State          State
	ExitCode       int
	StartedAt      time.Time
	FinishedAt     time.Time
	Stdout         []byte
	Stderr         []byte
	Validated      bool
	TemporaryDir   string
	RecoveryError  error
	OperationError error
	Artifacts      []AttackArtifact
}

func (r AttackResult) Duration() time.Duration {
	if r.StartedAt.IsZero() || r.FinishedAt.IsZero() {
		return 0
	}
	return r.FinishedAt.Sub(r.StartedAt)
}

type AttackEngine interface {
	Name() string
	Available(ctx context.Context) error
	Run(ctx context.Context, request AttackRequest, emit func(AttackEvent)) (AttackResult, error)
}

// ReleasedDeviceController is implemented by device.Manager. The callback is
// invoked only after polling has stopped and the in-process reader is closed.
type ReleasedDeviceController interface {
	WithReleasedDevice(context.Context, device.ReleasedDeviceHooks, func(context.Context, device.ReleasedDevice) error) device.ReleasedDeviceResult
}

type ErrorCode string

const (
	CodeUnavailable ErrorCode = "unavailable"
	CodeStart       ErrorCode = "start_failed"
	CodeExit        ErrorCode = "nonzero_exit"
	CodeCancelled   ErrorCode = "cancelled"
	CodeTimeout     ErrorCode = "timeout"
	CodeValidation  ErrorCode = "validation_failed"
	CodeTemporary   ErrorCode = "temporary_directory_failed"
)

var (
	ErrUnavailable = errors.New("external engine unavailable")
	ErrStart       = errors.New("external engine failed to start")
	ErrExit        = errors.New("external engine exited unsuccessfully")
	ErrCancelled   = errors.New("external engine cancelled")
	ErrTimeout     = errors.New("external engine timed out")
	ErrValidation  = errors.New("external engine result validation failed")
	ErrTemporary   = errors.New("external engine temporary directory failed")
)

type Error struct {
	Code     ErrorCode
	Engine   string
	ExitCode int
	Version  string
	Detail   string
	Cause    error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	detail := e.Detail
	if detail == "" {
		detail = string(e.Code)
	}
	metadata := ""
	if e.Version != "" {
		metadata += ", version " + e.Version
	}
	if e.ExitCode >= 0 {
		metadata += fmt.Sprintf(", exit code %d", e.ExitCode)
	}
	if e.Engine == "" {
		return detail + metadata
	}
	return fmt.Sprintf("external engine %s: %s%s", e.Engine, detail, metadata)
}

func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	return target == sentinelForCode(e.Code) || errors.Is(e.Cause, target)
}

func sentinelForCode(code ErrorCode) error {
	switch code {
	case CodeUnavailable:
		return ErrUnavailable
	case CodeStart:
		return ErrStart
	case CodeExit:
		return ErrExit
	case CodeCancelled:
		return ErrCancelled
	case CodeTimeout:
		return ErrTimeout
	case CodeValidation:
		return ErrValidation
	case CodeTemporary:
		return ErrTemporary
	default:
		return nil
	}
}
