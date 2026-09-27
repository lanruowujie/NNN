package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
)

// SectorKeys contains caller-supplied known keys for one sector. A nil key is
// unknown; a key becomes verified only after this workflow authenticates it.
type SectorKeys struct {
	Sector int
	KeyA   *nfc.Key
	KeyB   *nfc.Key
}

type VerifiedSectorKeys struct {
	Sector int
	KeyA   *nfc.Key
	KeyB   *nfc.Key
}

type DumpRequest struct {
	Card   nfc.CardInfo
	Device nfc.DeviceInfo
	Keys   []SectorKeys
}

type DumpResult struct {
	Dump       mifare.Dump
	Card       nfc.CardInfo
	Device     nfc.DeviceInfo
	Keys       []VerifiedSectorKeys
	StartedAt  time.Time
	FinishedAt time.Time
}

func (r DumpResult) Complete() bool { return r.Dump.Complete() }

type DumpService struct {
	devices ReaderExecutor
	now     func() time.Time
}

func NewDumpService(devices ReaderExecutor) *DumpService {
	return &DumpService{devices: devices, now: func() time.Time { return time.Now().UTC() }}
}

// ReadCard produces a partial result even when a card/device failure stops the
// operation. Unvisited blocks remain unread and are never silently zero-filled.
func (s *DumpService) ReadCard(ctx context.Context, request DumpRequest) (DumpResult, error) {
	request.Card = request.Card.Clone()
	layout, err := layoutForCard(request.Card)
	if err != nil {
		return DumpResult{}, nfc.NewError("read card dump", nfc.CodeUnsupported, "card is not MIFARE Classic 1K/4K", err)
	}
	if s == nil || s.devices == nil {
		return DumpResult{}, nfc.NewError("read card dump", nfc.CodeNotOpen, "device manager is unavailable", nil)
	}
	keyMap, err := validateSectorKeys(layout, request.Keys)
	if err != nil {
		return DumpResult{}, err
	}
	image, _ := mifare.NewDump(layout)
	now := s.now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	result := DumpResult{
		Dump:      image,
		Card:      request.Card.Clone(),
		Device:    request.Device,
		StartedAt: now(),
	}
	sectors, _ := layout.SectorCount()
	result.Keys = make([]VerifiedSectorKeys, sectors)
	for sector := range result.Keys {
		result.Keys[sector].Sector = sector
	}

	err = s.devices.WithReader(ctx, func(operationCtx context.Context, reader nfc.Reader) error {
		for sector := 0; sector < sectors; sector++ {
			provided := keyMap[sector]
			verified, verifyErr := verifyProvidedKeys(operationCtx, reader, request.Card, sector, layout, provided)
			if verifyErr != nil {
				return verifyErr
			}
			result.Keys[sector] = verified
			keyType, key, ok := preferredVerifiedKey(verified)
			first, _ := layout.FirstBlock(sector)
			count, _ := layout.BlocksInSector(sector)
			if !ok {
				for block := first; block < first+count; block++ {
					result.Dump.Blocks[block].Status = mifare.BlockAuthFailed
					result.Dump.Blocks[block].ErrorCode = "authentication_failed"
					result.Dump.Blocks[block].Error = "no supplied key authenticated this sector"
				}
				continue
			}

			for block := first; block < first+count; block++ {
				if err := selectExpectedCard(operationCtx, reader, request.Card); err != nil {
					return err
				}
				if err := reader.Authenticate(operationCtx, byte(block), keyType, key); err != nil {
					setBlockError(&result.Dump.Blocks[block], mifare.BlockAuthFailed, err)
					if fatalCardOperation(err) {
						return err
					}
					continue
				}
				data, err := reader.ReadBlock(operationCtx, byte(block))
				if err != nil {
					setBlockError(&result.Dump.Blocks[block], mifare.BlockReadFailed, err)
					if fatalCardOperation(err) {
						return err
					}
					continue
				}
				isTrailer, _ := layout.IsTrailer(block)
				if !isTrailer {
					result.Dump.Blocks[block].Data = data
					result.Dump.Blocks[block].KnownMask = mifare.AllBytesKnown
					result.Dump.Blocks[block].Status = mifare.BlockRead
					continue
				}
				populateTrailerBlock(&result.Dump.Blocks[block], data, verified)
			}
		}
		return nil
	})
	result.FinishedAt = now()
	return result, err
}

func validateSectorKeys(layout mifare.Layout, keys []SectorKeys) (map[int]SectorKeys, error) {
	sectors, err := layout.SectorCount()
	if err != nil {
		return nil, err
	}
	result := make(map[int]SectorKeys, len(keys))
	for _, entry := range keys {
		if entry.Sector < 0 || entry.Sector >= sectors {
			return nil, nfc.NewError("validate sector keys", nfc.CodeInvalidArgument, fmt.Sprintf("sector %d is outside the card layout", entry.Sector), nil)
		}
		if _, exists := result[entry.Sector]; exists {
			return nil, nfc.NewError("validate sector keys", nfc.CodeInvalidArgument, fmt.Sprintf("sector %d has duplicate key entries", entry.Sector), nil)
		}
		result[entry.Sector] = cloneSectorKeys(entry)
	}
	return result, nil
}

func cloneSectorKeys(value SectorKeys) SectorKeys {
	clone := SectorKeys{Sector: value.Sector}
	if value.KeyA != nil {
		key := *value.KeyA
		clone.KeyA = &key
	}
	if value.KeyB != nil {
		key := *value.KeyB
		clone.KeyB = &key
	}
	return clone
}

func verifyProvidedKeys(ctx context.Context, reader nfc.Reader, card nfc.CardInfo, sector int, layout mifare.Layout, provided SectorKeys) (VerifiedSectorKeys, error) {
	verified := VerifiedSectorKeys{Sector: sector}
	block, _ := layout.FirstBlock(sector)
	for _, candidate := range []struct {
		kind nfc.KeyType
		key  *nfc.Key
	}{{nfc.KeyTypeA, provided.KeyA}, {nfc.KeyTypeB, provided.KeyB}} {
		if candidate.key == nil {
			continue
		}
		if err := selectExpectedCard(ctx, reader, card); err != nil {
			return verified, err
		}
		if err := reader.Authenticate(ctx, byte(block), candidate.kind, *candidate.key); err != nil {
			if errors.Is(err, nfc.ErrAuthenticationFailed) {
				continue
			}
			return verified, err
		}
		key := *candidate.key
		if candidate.kind == nfc.KeyTypeA {
			verified.KeyA = &key
		} else {
			verified.KeyB = &key
		}
	}
	return verified, nil
}

func preferredVerifiedKey(keys VerifiedSectorKeys) (nfc.KeyType, nfc.Key, bool) {
	if keys.KeyA != nil {
		return nfc.KeyTypeA, *keys.KeyA, true
	}
	if keys.KeyB != nil {
		return nfc.KeyTypeB, *keys.KeyB, true
	}
	return 0, nfc.Key{}, false
}

func populateTrailerBlock(block *mifare.Block, read [nfc.BlockSize]byte, keys VerifiedSectorKeys) {
	block.Data = read
	// Key A is always masked. Access bytes 6..9 are always readable after a
	// successful authentication.
	block.KnownMask = 0x03c0
	block.Status = mifare.BlockRead
	synthetic := false
	if keys.KeyA != nil {
		copy(block.Data[0:6], keys.KeyA[:])
		block.KnownMask |= 0x003f
		block.KeyASource = "authenticated_key_a"
		synthetic = true
	}
	access, err := mifare.DecodeAccessBits([3]byte{read[6], read[7], read[8]})
	if err != nil {
		block.ErrorCode = "invalid_access_bits"
		block.Error = err.Error()
	}
	keyBReadable := err == nil && access.Groups[3].KeyBReadable()
	if keyBReadable {
		block.KnownMask |= 0xfc00
		block.KeyBSource = "card_read"
	} else if keys.KeyB != nil {
		copy(block.Data[10:16], keys.KeyB[:])
		block.KnownMask |= 0xfc00
		block.KeyBSource = "authenticated_key_b"
		synthetic = true
	}
	if synthetic {
		block.Status = mifare.BlockSynthetic
	}
}

func setBlockError(block *mifare.Block, status mifare.BlockStatus, err error) {
	block.Status = status
	block.Error = err.Error()
	block.ErrorCode = nfcErrorName(err)
}

func nfcErrorName(err error) string {
	code, ok := nfc.ErrorCodeOf(err)
	if !ok {
		return "unknown"
	}
	switch code {
	case nfc.CodeNoCard:
		return "no_card"
	case nfc.CodeTimeout:
		return "timeout"
	case nfc.CodeAuthenticationFailed:
		return "authentication_failed"
	case nfc.CodeDeviceDisconnected:
		return "device_disconnected"
	case nfc.CodeCanceled:
		return "canceled"
	case nfc.CodeCardChanged:
		return "card_changed"
	case nfc.CodeVerificationFailed:
		return "verification_failed"
	default:
		return fmt.Sprintf("nfc_%d", code)
	}
}

func fatalCardOperation(err error) bool {
	return errors.Is(err, nfc.ErrNoCard) || errors.Is(err, nfc.ErrCardChanged) ||
		errors.Is(err, nfc.ErrDeviceDisconnected) || errors.Is(err, nfc.ErrCanceled) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
