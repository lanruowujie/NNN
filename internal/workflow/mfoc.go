package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
)

var (
	ErrMFoCAuthorization = errors.New("authorized-card confirmation is required")
	ErrMFoCSeedRequired  = errors.New("at least one NFCX-verified key is required")
	ErrMFoCNested        = errors.New("selected reader is not enabled for Nested attacks")
)

type MFoCEngineFactory func(attack.MFoCInvocation) attack.AttackEngine

type MFoCRequest struct {
	TaskID          string
	Card            nfc.CardInfo
	Device          nfc.DeviceInfo
	Authorized      bool
	Timeout         time.Duration
	TemporaryPolicy attack.TemporaryPolicy
}

type MFoCClaimStatus string

const (
	MFoCClaimVerified MFoCClaimStatus = "verified"
	MFoCClaimExisting MFoCClaimStatus = "existing"
	MFoCClaimRejected MFoCClaimStatus = "rejected"
	MFoCClaimConflict MFoCClaimStatus = "conflict"
)

type MFoCClaim struct {
	Sector        int
	KeyType       nfc.KeyType
	Value         nfc.Key
	Status        MFoCClaimStatus
	KeyID         string
	ExistingKeyID string
	Error         string
}

type MFoCResult struct {
	Card      nfc.CardInfo
	External  attack.AttackResult
	Claims    []MFoCClaim
	Added     int
	Existing  int
	Rejected  int
	Conflicts int
}

type MFoCService struct {
	released      attack.ReleasedDeviceController
	readers       ReaderExecutor
	keys          *keys.Store
	engineFactory MFoCEngineFactory
}

func NewMFoCService(released attack.ReleasedDeviceController, readers ReaderExecutor, store *keys.Store, factory MFoCEngineFactory) *MFoCService {
	return &MFoCService{released: released, readers: readers, keys: store, engineFactory: factory}
}

func (s *MFoCService) Run(ctx context.Context, request MFoCRequest, emit func(attack.AttackEvent)) (MFoCResult, error) {
	return s.run(ctx, request, emit, device.CapabilitiesFor(request.Device).Nested, ErrMFoCNested, "mfoc")
}

func (s *MFoCService) run(ctx context.Context, request MFoCRequest, emit func(attack.AttackEvent), supported bool, capabilityErr error, engineLabel string) (MFoCResult, error) {
	request.Card = request.Card.Clone()
	result := MFoCResult{Card: request.Card.Clone()}
	if s == nil || s.released == nil || s.readers == nil || s.keys == nil || s.engineFactory == nil {
		return result, attack.ErrUnavailable
	}
	if !request.Authorized {
		return result, ErrMFoCAuthorization
	}
	if !supported {
		return result, capabilityErr
	}
	layout, err := layoutForCard(request.Card)
	if err != nil {
		return result, err
	}
	known := s.verifiedSeeds(request.Card, layout)
	if len(known) == 0 {
		return result, ErrMFoCSeedRequired
	}
	engine := s.engineFactory(attack.MFoCInvocation{Card: request.Card, KnownKeys: known})
	coordinator := attack.NewCoordinator(s.released, engine)
	expected := request.Card.Clone()
	external, runErr := coordinator.Run(ctx, attack.AttackRequest{
		TaskID: request.TaskID, Timeout: request.Timeout, TemporaryPolicy: request.TemporaryPolicy,
		ExpectedCard: &expected,
	}, emit)
	result.External = external
	if runErr != nil {
		return result, runErr
	}
	if external.RecoveryError != nil {
		return result, &attack.Error{Code: attack.CodeValidation, Engine: external.Engine, ExitCode: external.ExitCode, Version: external.Version, Detail: "reader recovery failed before " + engineLabel + " key verification", Cause: external.RecoveryError}
	}
	image, err := mfocArtifactDump(external.Artifacts)
	if err != nil {
		return result, &attack.Error{Code: attack.CodeValidation, Engine: external.Engine, ExitCode: external.ExitCode, Version: external.Version, Detail: err.Error(), Cause: err}
	}
	result.Claims = extractMFoCClaims(image)
	emitMFoCEvent(emit, attack.AttackEvent{Engine: external.Engine, State: attack.StateVerifyingKeys, Message: "re-authenticating " + engineLabel + " candidate keys with libnfc"})
	cardID := keys.CardID(request.Card)
	err = s.readers.WithReader(ctx, func(operationCtx context.Context, reader nfc.Reader) error {
		for index := range result.Claims {
			if err := operationCtx.Err(); err != nil {
				return err
			}
			claim := &result.Claims[index]
			if err := selectExpectedCard(operationCtx, reader, request.Card); err != nil {
				return err
			}
			block, _ := layout.FirstBlock(claim.Sector)
			authErr := reader.Authenticate(operationCtx, byte(block), claim.KeyType, claim.Value)
			if authErr != nil {
				if errors.Is(authErr, nfc.ErrAuthenticationFailed) {
					claim.Status = MFoCClaimRejected
					claim.Error = authErr.Error()
					result.Rejected++
					emitMFoCEvent(emit, attack.AttackEvent{Engine: external.Engine, State: attack.StateVerifyingKeys, Message: fmt.Sprintf("sector %d key %s was rejected by libnfc", claim.Sector, mfocKeyTypeName(claim.KeyType))})
					continue
				}
				return authErr
			}
			merged, mergeErr := s.keys.MergeVerified(cardID, claim.Sector, claim.KeyType, claim.Value, keys.SourceAttackEngine)
			if mergeErr != nil {
				return mergeErr
			}
			switch merged.Status {
			case keys.VerificationAdded:
				claim.Status, claim.KeyID = MFoCClaimVerified, merged.Record.ID
				result.Added++
			case keys.VerificationExisting:
				claim.Status, claim.KeyID = MFoCClaimExisting, merged.Record.ID
				result.Existing++
			case keys.VerificationConflict:
				claim.Status, claim.ExistingKeyID = MFoCClaimConflict, merged.ExistingKeyID
				result.Conflicts++
			}
			emitMFoCEvent(emit, attack.AttackEvent{Engine: external.Engine, State: attack.StateVerifyingKeys, Message: fmt.Sprintf("sector %d key %s: %s", claim.Sector, mfocKeyTypeName(claim.KeyType), claim.Status)})
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	emitMFoCEvent(emit, attack.AttackEvent{Engine: external.Engine, State: attack.StateMergingResult, Message: "verified " + engineLabel + " keys are ready for conflict-safe merge"})
	return result, nil
}

func (s *MFoCService) verifiedSeeds(card nfc.CardInfo, layout mifare.Layout) []attack.MFoCKnownKey {
	sectors, _ := layout.SectorCount()
	verified := s.keys.Verified(keys.CardID(card))
	result := make([]attack.MFoCKnownKey, 0, len(verified))
	for _, match := range verified {
		if match.Sector < 0 || match.Sector >= sectors || !match.KeyType.Valid() {
			continue
		}
		record, ok := s.keys.Record(match.KeyID)
		if !ok {
			continue
		}
		result = append(result, attack.MFoCKnownKey{Sector: match.Sector, Type: match.KeyType, Value: record.Value})
	}
	return result
}

func mfocArtifactDump(artifacts []attack.AttackArtifact) (mifare.Dump, error) {
	for _, artifact := range artifacts {
		if artifact.MediaType == "application/vnd.nfcx.mifare-dump" {
			return mifare.ParseRaw(artifact.Data)
		}
	}
	return mifare.Dump{}, errors.New("validated mfoc dump artifact is missing")
}

func extractMFoCClaims(image mifare.Dump) []MFoCClaim {
	sectors, err := image.Layout.SectorCount()
	if err != nil {
		return nil
	}
	result := make([]MFoCClaim, 0, sectors*2)
	for sector := 0; sector < sectors; sector++ {
		trailer, _ := image.Layout.TrailerBlock(sector)
		data := image.Blocks[trailer].Data
		result = append(result,
			MFoCClaim{Sector: sector, KeyType: nfc.KeyTypeA, Value: nfc.Key(data[0:6])},
			MFoCClaim{Sector: sector, KeyType: nfc.KeyTypeB, Value: nfc.Key(data[10:16])},
		)
	}
	return result
}

func emitMFoCEvent(emit func(attack.AttackEvent), event attack.AttackEvent) {
	if emit == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	emit(event)
}

func mfocKeyTypeName(value nfc.KeyType) string {
	if value == nfc.KeyTypeB {
		return "B"
	}
	return "A"
}
