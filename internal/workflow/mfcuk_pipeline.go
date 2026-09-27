package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
)

var (
	ErrMFCUKAuthorization       = errors.New("authorized-card confirmation is required")
	ErrMFCUKClassic1K           = errors.New("MFCUK pipeline supports only MIFARE Classic 1K")
	ErrMFCUKDarkside            = errors.New("selected reader is not enabled for Darkside attacks")
	ErrMFCUKSeedAlreadyKnown    = errors.New("a verified key already exists; use MFOC directly")
	ErrMFCUKNoVerifiedCandidate = errors.New("mfcuk produced no candidate accepted by libnfc")
)

const (
	defaultMFCUKPipelineTimeout = 60 * time.Minute
	defaultMFCUKStageTimeout    = 30 * time.Minute
	defaultMFoCStageTimeout     = 30 * time.Minute
)

type MFCUKEngineFactory func(attack.MFCUKInvocation) attack.AttackEngine

type MFCUKPipelineRequest struct {
	TaskID          string
	Card            nfc.CardInfo
	Device          nfc.DeviceInfo
	Authorized      bool
	Timeout         time.Duration
	MFCUKTimeout    time.Duration
	MFoCTimeout     time.Duration
	TemporaryPolicy attack.TemporaryPolicy
}

type MFCUKCandidateStatus string

const (
	MFCUKCandidateVerified MFCUKCandidateStatus = "verified"
	MFCUKCandidateExisting MFCUKCandidateStatus = "existing"
	MFCUKCandidateRejected MFCUKCandidateStatus = "rejected"
	MFCUKCandidateConflict MFCUKCandidateStatus = "conflict"
)

type MFCUKCandidateClaim struct {
	Candidate     attack.MFCUKCandidate
	Status        MFCUKCandidateStatus
	KeyID         string
	ExistingKeyID string
	Error         string
}

type MFCUKPipelineResult struct {
	Card       nfc.CardInfo
	MFCUK      attack.AttackResult
	Candidates []MFCUKCandidateClaim
	Verified   int
	Existing   int
	Rejected   int
	Conflicts  int
	MFoC       MFoCResult
}

type MFCUKPipelineService struct {
	released     attack.ReleasedDeviceController
	readers      ReaderExecutor
	keys         *keys.Store
	mfcukFactory MFCUKEngineFactory
	mfoc         *MFoCService
}

func NewMFCUKPipelineService(released attack.ReleasedDeviceController, readers ReaderExecutor, store *keys.Store, mfcukFactory MFCUKEngineFactory, mfoc *MFoCService) *MFCUKPipelineService {
	return &MFCUKPipelineService{released: released, readers: readers, keys: store, mfcukFactory: mfcukFactory, mfoc: mfoc}
}

func (s *MFCUKPipelineService) Run(ctx context.Context, request MFCUKPipelineRequest, emit func(attack.AttackEvent)) (MFCUKPipelineResult, error) {
	request.Card = request.Card.Clone()
	result := MFCUKPipelineResult{Card: request.Card.Clone()}
	if ctx == nil || s == nil || s.released == nil || s.readers == nil || s.keys == nil || s.mfcukFactory == nil || s.mfoc == nil {
		return result, attack.ErrUnavailable
	}
	if !request.Authorized {
		return result, ErrMFCUKAuthorization
	}
	if nfc.InferCardType(request.Card) != nfc.CardTypeMIFAREClassic1K {
		return result, ErrMFCUKClassic1K
	}
	capabilities := device.CapabilitiesFor(request.Device)
	if !capabilities.Darkside {
		return result, ErrMFCUKDarkside
	}
	if !capabilities.Nested {
		return result, ErrMFoCNested
	}
	if len(s.keys.Verified(keys.CardID(request.Card))) != 0 {
		return result, ErrMFCUKSeedAlreadyKnown
	}
	totalTimeout := request.Timeout
	if totalTimeout <= 0 {
		totalTimeout = defaultMFCUKPipelineTimeout
	}
	pipelineCtx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()
	sequenced := newPipelineEmitter(emit)
	sequenced(attack.AttackEvent{Engine: "mfcuk-pipeline", State: attack.StatePreflight, Message: "MFCUK → MFOC preflight completed"})

	mfcukTimeout := request.MFCUKTimeout
	if mfcukTimeout <= 0 {
		mfcukTimeout = defaultMFCUKStageTimeout
	}
	sequenced(attack.AttackEvent{Engine: "mfcuk", State: attack.StateMFCUKStage, Message: "stage 1/2: recovering candidate keys with MFCUK"})
	expected := request.Card.Clone()
	engine := s.mfcukFactory(attack.MFCUKInvocation{Card: request.Card})
	external, runErr := attack.NewCoordinator(s.released, engine).Run(pipelineCtx, attack.AttackRequest{
		TaskID: request.TaskID + "-mfcuk", Timeout: mfcukTimeout, TemporaryPolicy: request.TemporaryPolicy, ExpectedCard: &expected,
	}, sequenced)
	result.MFCUK = external
	if runErr != nil {
		return result, runErr
	}
	if external.RecoveryError != nil {
		return result, &attack.Error{Code: attack.CodeValidation, Engine: external.Engine, ExitCode: external.ExitCode, Version: external.Version, Detail: "reader recovery failed before MFCUK candidate verification", Cause: external.RecoveryError}
	}
	candidates, err := attack.MFCUKCandidatesFromArtifacts(external.Artifacts)
	if err != nil {
		return result, &attack.Error{Code: attack.CodeValidation, Engine: external.Engine, ExitCode: external.ExitCode, Version: external.Version, Detail: err.Error(), Cause: err}
	}
	result.Candidates = make([]MFCUKCandidateClaim, len(candidates))
	sequenced(attack.AttackEvent{Engine: "mfcuk", State: attack.StateVerifyingSeed, Message: fmt.Sprintf("verifying all %d MFCUK candidates with libnfc", len(candidates))})
	err = s.readers.WithReader(pipelineCtx, func(operationCtx context.Context, reader nfc.Reader) error {
		for index, candidate := range candidates {
			if err := operationCtx.Err(); err != nil {
				return err
			}
			claim := &result.Candidates[index]
			claim.Candidate = candidate
			if candidate.Sector < 0 || candidate.Sector >= mifare.Classic1KSectors || !candidate.Type.Valid() {
				claim.Status, claim.Error = MFCUKCandidateRejected, "candidate slot is invalid"
				result.Rejected++
				continue
			}
			if err := selectExpectedCard(operationCtx, reader, request.Card); err != nil {
				return err
			}
			block, _ := mifare.Classic1K.FirstBlock(candidate.Sector)
			if authErr := reader.Authenticate(operationCtx, byte(block), candidate.Type, candidate.Value); authErr != nil {
				if errors.Is(authErr, nfc.ErrAuthenticationFailed) {
					claim.Status, claim.Error = MFCUKCandidateRejected, authErr.Error()
					result.Rejected++
					sequenced(attack.AttackEvent{Engine: "mfcuk", State: attack.StateVerifyingSeed, Message: fmt.Sprintf("MFCUK candidate %d/%d was rejected by libnfc", index+1, len(candidates))})
					continue
				}
				return authErr
			}
			merged, mergeErr := s.keys.MergeVerified(keys.CardID(request.Card), candidate.Sector, candidate.Type, candidate.Value, keys.SourceAttackEngine)
			if mergeErr != nil {
				return mergeErr
			}
			switch merged.Status {
			case keys.VerificationAdded:
				claim.Status, claim.KeyID = MFCUKCandidateVerified, merged.Record.ID
				result.Verified++
			case keys.VerificationExisting:
				claim.Status, claim.KeyID = MFCUKCandidateExisting, merged.Record.ID
				result.Existing++
			case keys.VerificationConflict:
				claim.Status, claim.ExistingKeyID = MFCUKCandidateConflict, merged.ExistingKeyID
				result.Conflicts++
			}
			sequenced(attack.AttackEvent{Engine: "mfcuk", State: attack.StateVerifyingSeed, Message: fmt.Sprintf("MFCUK candidate %d/%d: %s", index+1, len(candidates), claim.Status)})
		}
		return nil
	})
	if err != nil {
		return result, normalizePipelineContextError(ctx, pipelineCtx, err)
	}
	if result.Verified+result.Existing == 0 {
		return result, ErrMFCUKNoVerifiedCandidate
	}

	mfocTimeout := request.MFoCTimeout
	if mfocTimeout <= 0 {
		mfocTimeout = defaultMFoCStageTimeout
	}
	sequenced(attack.AttackEvent{Engine: "mfoc", State: attack.StateMFoCStage, Message: "stage 2/2: recovering remaining keys with MFOC"})
	mfocResult, err := s.mfoc.Run(pipelineCtx, MFoCRequest{
		TaskID: request.TaskID + "-mfoc", Card: request.Card, Device: request.Device, Authorized: true,
		Timeout: mfocTimeout, TemporaryPolicy: request.TemporaryPolicy,
	}, sequenced)
	result.MFoC = mfocResult
	if err != nil {
		return result, normalizePipelineContextError(ctx, pipelineCtx, err)
	}
	return result, nil
}

func normalizePipelineContextError(parent, pipeline context.Context, err error) error {
	if errors.Is(parent.Err(), context.Canceled) {
		return &attack.Error{Code: attack.CodeCancelled, Engine: "mfcuk-pipeline", ExitCode: -1, Detail: "pipeline cancelled by user", Cause: parent.Err()}
	}
	if errors.Is(pipeline.Err(), context.DeadlineExceeded) {
		return &attack.Error{Code: attack.CodeTimeout, Engine: "mfcuk-pipeline", ExitCode: -1, Detail: "pipeline deadline exceeded", Cause: pipeline.Err()}
	}
	return err
}

func newPipelineEmitter(emit func(attack.AttackEvent)) func(attack.AttackEvent) {
	var mutex sync.Mutex
	var sequence uint64
	return func(event attack.AttackEvent) {
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
