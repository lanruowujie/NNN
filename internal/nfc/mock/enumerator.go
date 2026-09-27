package mock

import (
	"context"
	"sync"

	"github.com/BennyThink/NFCX/internal/nfc"
)

// Enumerator is a configurable hardware-free device enumerator.
type Enumerator struct {
	mu      sync.Mutex
	devices []nfc.DeviceInfo
	err     error
}

func NewEnumerator(devices []nfc.DeviceInfo, err error) *Enumerator {
	e := &Enumerator{}
	e.SetResult(devices, err)
	return e
}

func (e *Enumerator) SetResult(devices []nfc.DeviceInfo, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.devices = append([]nfc.DeviceInfo(nil), devices...)
	e.err = err
}

func (e *Enumerator) ListDevices(ctx context.Context) ([]nfc.DeviceInfo, error) {
	if ctx == nil {
		return nil, nfc.NewError("list devices", nfc.CodeInvalidArgument, "nil context", nil)
	}
	if err := ctx.Err(); err != nil {
		code := nfc.CodeCanceled
		if err == context.DeadlineExceeded {
			code = nfc.CodeTimeout
		}
		return nil, nfc.NewError("list devices", code, "", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]nfc.DeviceInfo(nil), e.devices...), e.err
}

var _ nfc.DeviceEnumerator = (*Enumerator)(nil)
