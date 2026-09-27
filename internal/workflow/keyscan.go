package workflow

import (
	"context"
	"errors"
	"time"

	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/nfc"
)

type KeyScanRequest struct {
	Card nfc.CardInfo
}

type KeyScanMatch struct {
	Sector     int
	KeyType    nfc.KeyType
	KeyID      string
	Source     keys.Source
	VerifiedAt time.Time
}

type KeyScanProgress struct {
	Sector    int
	KeyType   nfc.KeyType
	Attempted int
	Found     int
	KeyID     string
	Source    keys.Source
	Matched   bool
}

type KeyScanResult struct {
	Card      nfc.CardInfo
	Attempted int
	Matches   []KeyScanMatch
	Cancelled bool
}

type KeyScanService struct {
	devices ReaderExecutor
	keys    *keys.Store
	now     func() time.Time
}

func NewKeyScanService(devices ReaderExecutor, store *keys.Store) *KeyScanService {
	return &KeyScanService{devices: devices, keys: store, now: func() time.Time { return time.Now().UTC() }}
}

// Scan authenticates each Key A/Key B slot using known candidates. Successful
// matches are committed to the store before progress is emitted, so cancellation
// never rolls back already confirmed work.
func (s *KeyScanService) Scan(ctx context.Context, request KeyScanRequest, emit func(KeyScanProgress)) (KeyScanResult, error) {
	request.Card = request.Card.Clone()
	result := KeyScanResult{Card: request.Card.Clone()}
	if s == nil || s.devices == nil || s.keys == nil {
		return result, nfc.NewError("scan keys", nfc.CodeNotOpen, "device manager or key store is unavailable", nil)
	}
	layout, err := layoutForCard(request.Card)
	if err != nil {
		return result, nfc.NewError("scan keys", nfc.CodeUnsupported, "card is not MIFARE Classic 1K/4K", err)
	}
	if emit == nil {
		emit = func(KeyScanProgress) {}
	}
	cardID := keys.CardID(request.Card)
	sectors, _ := layout.SectorCount()
	err = s.devices.WithReader(ctx, func(operationCtx context.Context, reader nfc.Reader) error {
		for sector := 0; sector < sectors; sector++ {
			block, _ := layout.FirstBlock(sector)
			for _, keyType := range []nfc.KeyType{nfc.KeyTypeA, nfc.KeyTypeB} {
				for _, candidate := range s.keys.Candidates(cardID, sector, keyType) {
					if err := operationCtx.Err(); err != nil {
						return err
					}
					if err := selectExpectedCard(operationCtx, reader, request.Card); err != nil {
						return err
					}
					result.Attempted++
					authErr := reader.Authenticate(operationCtx, byte(block), keyType, candidate.Value)
					progress := KeyScanProgress{
						Sector: sector, KeyType: keyType, Attempted: result.Attempted,
						Found: len(result.Matches), KeyID: candidate.ID,
					}
					if authErr == nil {
						if err := s.keys.Verify(cardID, sector, keyType, candidate.ID); err != nil {
							return err
						}
						verifiedAt := time.Now().UTC()
						if s.now != nil {
							verifiedAt = s.now()
						}
						if len(candidate.Sources) != 0 {
							progress.Source = candidate.Sources[0]
						}
						match := KeyScanMatch{Sector: sector, KeyType: keyType, KeyID: candidate.ID, Source: progress.Source, VerifiedAt: verifiedAt}
						result.Matches = append(result.Matches, match)
						progress.Matched = true
						progress.Found = len(result.Matches)
						emit(progress)
						break
					}
					emit(progress)
					if !errors.Is(authErr, nfc.ErrAuthenticationFailed) {
						return authErr
					}
				}
			}
		}
		return nil
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, nfc.ErrCanceled) {
		result.Cancelled = true
	}
	return result, err
}
