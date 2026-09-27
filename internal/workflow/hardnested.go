package workflow

import (
	"context"
	"errors"
	"time"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/nfc"
)

var ErrHardnestedCapability = errors.New("selected reader is not enabled for Hardnested attacks")

type HardnestedEngineFactory func(attack.MFoCInvocation) attack.AttackEngine

type HardnestedRequest struct {
	TaskID          string
	Card            nfc.CardInfo
	Device          nfc.DeviceInfo
	Authorized      bool
	Timeout         time.Duration
	TemporaryPolicy attack.TemporaryPolicy
}

type HardnestedResult = MFoCResult

// HardnestedService reuses the MFOC dump extraction and per-key libnfc
// verification path, while selecting a separate engine and capability gate.
type HardnestedService struct {
	delegate *MFoCService
	factory  HardnestedEngineFactory
}

func NewHardnestedService(released attack.ReleasedDeviceController, readers ReaderExecutor, store *keys.Store, factory HardnestedEngineFactory) *HardnestedService {
	return &HardnestedService{
		delegate: &MFoCService{released: released, readers: readers, keys: store, engineFactory: MFoCEngineFactory(factory)},
		factory:  factory,
	}
}

func (s *HardnestedService) Available(ctx context.Context) error {
	if ctx == nil || s == nil || s.factory == nil {
		return attack.ErrUnavailable
	}
	return s.factory(attack.MFoCInvocation{}).Available(ctx)
}

func (s *HardnestedService) Run(ctx context.Context, request HardnestedRequest, emit func(attack.AttackEvent)) (HardnestedResult, error) {
	if s == nil || s.delegate == nil {
		return HardnestedResult{}, attack.ErrUnavailable
	}
	mfocRequest := MFoCRequest{
		TaskID: request.TaskID, Card: request.Card, Device: request.Device, Authorized: request.Authorized,
		Timeout: request.Timeout, TemporaryPolicy: request.TemporaryPolicy,
	}
	return s.delegate.run(ctx, mfocRequest, emit, device.CapabilitiesFor(request.Device).Hardnested, ErrHardnestedCapability, "mfoc-hardnested")
}
