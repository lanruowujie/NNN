package workflow

import (
	"context"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
)

// ReaderExecutor is the device-ownership boundary implemented by
// device.Manager. Its narrow shape keeps ClassicIO hardware-free in tests.
type ReaderExecutor interface {
	WithReader(context.Context, func(context.Context, nfc.Reader) error) error
}

// ClassicBlockRequest identifies one authorized operation on the card that was
// visible when the user started the task.
type ClassicBlockRequest struct {
	Card    nfc.CardInfo
	Block   int
	KeyType nfc.KeyType
	Key     nfc.Key
}

// ClassicIO runs known-key single-block operations while polling is suspended.
type ClassicIO struct {
	devices ReaderExecutor
}

func NewClassicIO(devices ReaderExecutor) *ClassicIO {
	return &ClassicIO{devices: devices}
}

// ReadBlock selects and verifies the expected card, authenticates the target
// sector, and reads exactly one block. Manufacturer block 0 is readable.
func (s *ClassicIO) ReadBlock(ctx context.Context, request ClassicBlockRequest) ([nfc.BlockSize]byte, error) {
	request.Card = request.Card.Clone()
	block, err := validateClassicRequest("read block", request)
	if err != nil {
		return [nfc.BlockSize]byte{}, err
	}
	if s == nil || s.devices == nil {
		return [nfc.BlockSize]byte{}, nfc.NewError("read block", nfc.CodeNotOpen, "device manager is unavailable", nil)
	}

	var output [nfc.BlockSize]byte
	err = s.devices.WithReader(ctx, func(operationCtx context.Context, reader nfc.Reader) error {
		if err := selectExpectedCard(operationCtx, reader, request.Card); err != nil {
			return err
		}
		if err := reader.Authenticate(operationCtx, block, request.KeyType, request.Key); err != nil {
			return err
		}
		var err error
		output, err = reader.ReadBlock(operationCtx, block)
		return err
	})
	return output, err
}

// WriteBlock writes one ordinary data block and relies on Reader.WriteBlock to
// perform immediate readback verification under the same exclusive lock.
func (s *ClassicIO) WriteBlock(ctx context.Context, request ClassicBlockRequest, data [nfc.BlockSize]byte) error {
	request.Card = request.Card.Clone()
	block, err := validateClassicRequest("write block", request)
	if err != nil {
		return err
	}
	if block == 0 {
		return nfc.NewError("write block", nfc.CodeInvalidArgument, "ordinary writes to manufacturer block 0 are forbidden", nil)
	}
	layout, _ := layoutForCard(request.Card)
	isTrailer, _ := layout.IsTrailer(int(block))
	if isTrailer {
		return nfc.NewError("write block", nfc.CodeInvalidArgument, "sector trailer writes require an explicit protected workflow", nil)
	}
	if s == nil || s.devices == nil {
		return nfc.NewError("write block", nfc.CodeNotOpen, "device manager is unavailable", nil)
	}

	return s.devices.WithReader(ctx, func(operationCtx context.Context, reader nfc.Reader) error {
		if err := selectExpectedCard(operationCtx, reader, request.Card); err != nil {
			return err
		}
		if err := reader.Authenticate(operationCtx, block, request.KeyType, request.Key); err != nil {
			return err
		}
		return reader.WriteBlock(operationCtx, block, data)
	})
}

func validateClassicRequest(op string, request ClassicBlockRequest) (byte, error) {
	if len(request.Card.UID) == 0 {
		return 0, nfc.NewError(op, nfc.CodeInvalidArgument, "expected card identity is required", nil)
	}
	if !request.KeyType.Valid() {
		return 0, nfc.NewError(op, nfc.CodeInvalidArgument, "unknown key type", nil)
	}
	layout, err := layoutForCard(request.Card)
	if err != nil {
		return 0, nfc.NewError(op, nfc.CodeUnsupported, "card is not MIFARE Classic 1K/4K", err)
	}
	if _, err := layout.SectorForBlock(request.Block); err != nil {
		return 0, nfc.NewError(op, nfc.CodeInvalidArgument, err.Error(), err)
	}
	return byte(request.Block), nil
}

func layoutForCard(card nfc.CardInfo) (mifare.Layout, error) {
	switch nfc.InferCardType(card) {
	case nfc.CardTypeMIFAREClassic1K:
		return mifare.Classic1K, nil
	case nfc.CardTypeMIFAREClassic4K:
		return mifare.Classic4K, nil
	default:
		return 0, nfc.ErrUnsupported
	}
}

func selectExpectedCard(ctx context.Context, reader nfc.Reader, expected nfc.CardInfo) error {
	actual, err := reader.CardInfo(ctx)
	if err != nil {
		return err
	}
	if !nfc.SameCard(expected, actual) {
		return nfc.NewError("select expected card", nfc.CodeCardChanged, "selected card does not match the card that started the task", nil)
	}
	return nil
}
