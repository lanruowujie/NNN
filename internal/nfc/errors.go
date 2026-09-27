package nfc

import (
	"errors"
	"fmt"
)

// ErrorCode is stable across NFC backends and native libnfc versions.
type ErrorCode uint8

const (
	CodeUnknown ErrorCode = iota
	CodeNoCard
	CodeTimeout
	CodeAuthenticationFailed
	CodeDeviceDisconnected
	CodeIO
	CodeCanceled
	CodeInvalidArgument
	CodeNotOpen
	CodeUnsupported
	CodeBusy
	CodeInternal
	CodePermission
	CodeCardChanged
	CodeNotAuthenticated
	CodeVerificationFailed
)

type codeError struct {
	code ErrorCode
	text string
}

func (e codeError) Error() string { return e.text }

var (
	ErrNoCard               error = codeError{CodeNoCard, "no card present"}
	ErrTimeout              error = codeError{CodeTimeout, "operation timed out"}
	ErrAuthenticationFailed error = codeError{CodeAuthenticationFailed, "authentication failed"}
	ErrDeviceDisconnected   error = codeError{CodeDeviceDisconnected, "device disconnected"}
	ErrIO                   error = codeError{CodeIO, "I/O error"}
	ErrCanceled             error = codeError{CodeCanceled, "operation canceled"}
	ErrInvalidArgument      error = codeError{CodeInvalidArgument, "invalid argument"}
	ErrNotOpen              error = codeError{CodeNotOpen, "reader is not open"}
	ErrUnsupported          error = codeError{CodeUnsupported, "operation unsupported"}
	ErrBusy                 error = codeError{CodeBusy, "reader is busy"}
	ErrInternal             error = codeError{CodeInternal, "internal error"}
	ErrPermission           error = codeError{CodePermission, "permission denied"}
	ErrCardChanged          error = codeError{CodeCardChanged, "card changed during operation"}
	ErrNotAuthenticated     error = codeError{CodeNotAuthenticated, "sector is not authenticated"}
	ErrVerificationFailed   error = codeError{CodeVerificationFailed, "write verification failed"}
)

// OpError adds operation and native detail while preserving a stable error
// category for errors.Is and ErrorCodeOf.
type OpError struct {
	Op     string
	Code   ErrorCode
	Detail string
	Cause  error
}

func (e *OpError) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := errorForCode(e.Code).Error()
	if e.Detail != "" {
		message = e.Detail
	}
	if e.Op == "" {
		return message
	}
	return fmt.Sprintf("nfc %s: %s", e.Op, message)
}

// Unwrap exposes a context or other lower-level cause when one exists.
func (e *OpError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is makes an OpError match both its stable NFC category and its cause.
func (e *OpError) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	if code, ok := target.(codeError); ok && code.code == e.Code {
		return true
	}
	return errors.Is(e.Cause, target)
}

// NewError creates a categorized error suitable for Reader implementations and
// hardware-free mocks.
func NewError(op string, code ErrorCode, detail string, cause error) error {
	return &OpError{Op: op, Code: code, Detail: detail, Cause: cause}
}

// ErrorCodeOf returns the stable category carried by err.
func ErrorCodeOf(err error) (ErrorCode, bool) {
	if err == nil {
		return CodeUnknown, false
	}
	var opErr *OpError
	if errors.As(err, &opErr) {
		return opErr.Code, true
	}
	var category codeError
	if errors.As(err, &category) {
		return category.code, true
	}
	return CodeUnknown, false
}

func errorForCode(code ErrorCode) error {
	switch code {
	case CodeNoCard:
		return ErrNoCard
	case CodeTimeout:
		return ErrTimeout
	case CodeAuthenticationFailed:
		return ErrAuthenticationFailed
	case CodeDeviceDisconnected:
		return ErrDeviceDisconnected
	case CodeIO:
		return ErrIO
	case CodeCanceled:
		return ErrCanceled
	case CodeInvalidArgument:
		return ErrInvalidArgument
	case CodeNotOpen:
		return ErrNotOpen
	case CodeUnsupported:
		return ErrUnsupported
	case CodeBusy:
		return ErrBusy
	case CodeInternal:
		return ErrInternal
	case CodePermission:
		return ErrPermission
	case CodeCardChanged:
		return ErrCardChanged
	case CodeNotAuthenticated:
		return ErrNotAuthenticated
	case CodeVerificationFailed:
		return ErrVerificationFailed
	default:
		return errors.New("unknown NFC error")
	}
}
