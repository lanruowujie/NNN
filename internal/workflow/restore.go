package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
)

type RestorePhase string

const (
	RestorePreflight RestorePhase = "preflight"
	RestoreExecute   RestorePhase = "execute"
	RestoreVerify    RestorePhase = "verify"
	RestoreComplete  RestorePhase = "complete"
	RestoreFailed    RestorePhase = "failed"
)

type RestoreBlockStatus string

const (
	RestoreNotWritten          RestoreBlockStatus = "not_written"
	RestoreSkippedUnchanged    RestoreBlockStatus = "skipped_unchanged"
	RestoreSkippedProtected    RestoreBlockStatus = "skipped_protected"
	RestoreWritten             RestoreBlockStatus = "written"
	RestoreVerified            RestoreBlockStatus = "verified"
	RestorePreconditionFailed  RestoreBlockStatus = "precondition_failed"
	RestoreWriteOutcomeUnknown RestoreBlockStatus = "write_outcome_unknown"
	RestoreVerifyFailed        RestoreBlockStatus = "verification_failed"
)

type RestoreRequest struct {
	Card            nfc.CardInfo
	Dump            mifare.Dump
	SourceUIDLength int
	TargetKeys      []SectorKeys
	// Blocks limits the restore to explicit addresses. Nil retains whole-card
	// restore behaviour. Manufacturer block 0 remains protected in all modes.
	Blocks []int
}

type RestoreOperation struct {
	Block   int
	Sector  int
	Trailer bool
	KeyType nfc.KeyType
	Key     nfc.Key
	Data    [nfc.BlockSize]byte
}

type RestorePlan struct {
	Card       nfc.CardInfo
	Layout     mifare.Layout
	Operations []RestoreOperation
}

type RestoreBlockResult struct {
	Block             int                `json:"block"`
	Sector            int                `json:"sector"`
	Trailer           bool               `json:"trailer"`
	Status            RestoreBlockStatus `json:"status"`
	Attempted         bool               `json:"attempted"`
	Written           bool               `json:"written"`
	WriteOutcomeKnown bool               `json:"writeOutcomeKnown"`
	Verified          bool               `json:"verified"`
	ErrorCode         string             `json:"errorCode,omitempty"`
	Error             string             `json:"error,omitempty"`
}

type RestoreResult struct {
	Phase           RestorePhase         `json:"phase"`
	Blocks          []RestoreBlockResult `json:"blocks"`
	WrittenBlocks   []int                `json:"writtenBlocks"`
	VerifiedBlocks  []int                `json:"verifiedBlocks"`
	UnwrittenBlocks []int                `json:"unwrittenBlocks"`
	FailedBlock     *int                 `json:"failedBlock,omitempty"`
}

type RestoreEvent struct {
	Phase   RestorePhase `json:"phase"`
	Block   int          `json:"block,omitempty"`
	Message string       `json:"message"`
}

type RestoreService struct{ devices ReaderExecutor }

func NewRestoreService(devices ReaderExecutor) *RestoreService {
	return &RestoreService{devices: devices}
}

// Preflight performs every non-mutating validation and authentication check.
func (s *RestoreService) Preflight(ctx context.Context, request RestoreRequest) (RestorePlan, error) {
	request.Card = request.Card.Clone()
	if s == nil || s.devices == nil {
		return RestorePlan{}, nfc.NewError("restore preflight", nfc.CodeNotOpen, "device manager is unavailable", nil)
	}
	var plan RestorePlan
	err := s.devices.WithReader(ctx, func(operationCtx context.Context, reader nfc.Reader) error {
		var err error
		plan, err = buildRestorePlan(operationCtx, reader, request)
		return err
	})
	return plan, err
}

// Restore keeps preflight, execution and verification under one exclusive
// device lease. Verification is immediate even though it is surfaced as an
// explicit phase event for the GUI.
func (s *RestoreService) Restore(ctx context.Context, request RestoreRequest, emit func(RestoreEvent)) (RestoreResult, error) {
	request.Card = request.Card.Clone()
	result := newRestoreResult(request)
	if s == nil || s.devices == nil {
		err := nfc.NewError("restore", nfc.CodeNotOpen, "device manager is unavailable", nil)
		result.Phase = RestoreFailed
		return result, err
	}
	if emit == nil {
		emit = func(RestoreEvent) {}
	}
	emit(RestoreEvent{Phase: RestorePreflight, Block: -1, Message: "validating dump and target card"})

	err := s.devices.WithReader(ctx, func(operationCtx context.Context, reader nfc.Reader) error {
		plan, err := buildRestorePlan(operationCtx, reader, request)
		if err != nil {
			return err
		}
		trailerWriter, supportsTrailers := reader.(nfc.ClassicTrailerWriter)
		for _, operation := range plan.Operations {
			emit(RestoreEvent{Phase: RestoreExecute, Block: operation.Block, Message: "writing block"})
			entry := &result.Blocks[operation.Block]
			if err := selectExpectedCard(operationCtx, reader, plan.Card); err != nil {
				markRestoreFailure(entry, false, false, err)
				return err
			}
			if err := reader.Authenticate(operationCtx, byte(operation.Block), operation.KeyType, operation.Key); err != nil {
				markRestoreFailure(entry, false, false, err)
				return err
			}
			entry.Attempted = true
			if !operation.Trailer {
				err := reader.WriteBlock(operationCtx, byte(operation.Block), operation.Data)
				if err != nil {
					markRestoreFailure(entry, true, errors.Is(err, nfc.ErrVerificationFailed), err)
					return err
				}
				entry.Status, entry.Written, entry.WriteOutcomeKnown, entry.Verified = RestoreVerified, true, true, true
				emit(RestoreEvent{Phase: RestoreVerify, Block: operation.Block, Message: "block readback verified"})
				continue
			}

			if !supportsTrailers {
				return nfc.NewError("restore", nfc.CodeUnsupported, "reader does not support protected sector trailer writes", nil)
			}
			if err := trailerWriter.WriteSectorTrailer(operationCtx, byte(operation.Block), operation.Data); err != nil {
				markRestoreFailure(entry, true, false, err)
				return err
			}
			entry.Status, entry.Written, entry.WriteOutcomeKnown = RestoreWritten, true, true
			emit(RestoreEvent{Phase: RestoreVerify, Block: operation.Block, Message: "verifying replacement trailer and keys"})
			if err := verifyRestoredTrailer(operationCtx, reader, plan.Card, operation); err != nil {
				markRestoreFailure(entry, true, true, err)
				return err
			}
			entry.Status, entry.Verified = RestoreVerified, true
		}
		return nil
	})
	if err != nil {
		result.Phase = RestoreFailed
		finalizeRestoreResult(&result)
		return result, err
	}
	result.Phase = RestoreComplete
	finalizeRestoreResult(&result)
	emit(RestoreEvent{Phase: RestoreComplete, Block: -1, Message: "restore completed"})
	return result, nil
}

func buildRestorePlan(ctx context.Context, reader nfc.Reader, request RestoreRequest) (RestorePlan, error) {
	layout, err := layoutForCard(request.Card)
	if err != nil {
		return RestorePlan{}, nfc.NewError("restore preflight", nfc.CodeUnsupported, "target card is not MIFARE Classic 1K/4K", err)
	}
	if request.Dump.Layout != layout {
		return RestorePlan{}, nfc.NewError("restore preflight", nfc.CodeInvalidArgument, "dump capacity does not match the target card", nil)
	}
	if !request.Dump.Complete() {
		return RestorePlan{}, nfc.NewError("restore preflight", nfc.CodeInvalidArgument, "partial dump cannot be restored", mifare.ErrIncompleteDump)
	}
	if _, err := request.Dump.ValidateBCC(request.SourceUIDLength); err != nil {
		return RestorePlan{}, nfc.NewError("restore preflight", nfc.CodeInvalidArgument, err.Error(), err)
	}
	if err := request.Dump.ValidateTrailers(); err != nil {
		return RestorePlan{}, nfc.NewError("restore preflight", nfc.CodeInvalidArgument, err.Error(), err)
	}
	keyMap, err := validateSectorKeys(layout, request.TargetKeys)
	if err != nil {
		return RestorePlan{}, err
	}
	selected, err := selectedRestoreBlocks(layout, request.Blocks)
	if err != nil {
		return RestorePlan{}, err
	}
	if err := selectExpectedCard(ctx, reader, request.Card); err != nil {
		return RestorePlan{}, err
	}

	sectors, _ := layout.SectorCount()
	dataOperations := make([]RestoreOperation, 0)
	trailerOperations := make([]RestoreOperation, 0, sectors)
	for sector := 0; sector < sectors; sector++ {
		first, _ := layout.FirstBlock(sector)
		count, _ := layout.BlocksInSector(sector)
		sectorSelected := false
		for block := first; block < first+count; block++ {
			if block != 0 && selected[block] {
				sectorSelected = true
				break
			}
		}
		if !sectorSelected {
			continue
		}
		verified, err := verifyProvidedKeys(ctx, reader, request.Card, sector, layout, keyMap[sector])
		if err != nil {
			return RestorePlan{}, err
		}
		if _, _, ok := preferredVerifiedKey(verified); !ok {
			return RestorePlan{}, nfc.NewError("restore preflight", nfc.CodeAuthenticationFailed, fmt.Sprintf("no supplied key authenticated target sector %d", sector), nil)
		}
		keyType, key, _ := preferredVerifiedKey(verified)
		trailerBlock, _ := layout.TrailerBlock(sector)
		if err := selectExpectedCard(ctx, reader, request.Card); err != nil {
			return RestorePlan{}, err
		}
		if err := reader.Authenticate(ctx, byte(trailerBlock), keyType, key); err != nil {
			return RestorePlan{}, err
		}
		currentTrailer, err := reader.ReadBlock(ctx, byte(trailerBlock))
		if err != nil {
			return RestorePlan{}, err
		}
		currentAccess, err := mifare.DecodeAccessBits([3]byte{currentTrailer[6], currentTrailer[7], currentTrailer[8]})
		if err != nil {
			return RestorePlan{}, nfc.NewError("restore preflight", nfc.CodeInvalidArgument, fmt.Sprintf("target sector %d has invalid access bits", sector), err)
		}

		for block := first; block < first+count-1; block++ {
			if block == 0 {
				continue
			}
			if !selected[block] {
				continue
			}
			group, _ := layout.AccessGroupForBlock(block)
			kind, value, ok := choosePermittedKey(currentAccess.Groups[group].DataWriteKeys(), verified)
			if !ok {
				return RestorePlan{}, nfc.NewError("restore preflight", nfc.CodeInvalidArgument, fmt.Sprintf("target access conditions do not permit writing block %d with a verified key", block), nil)
			}
			dataOperations = append(dataOperations, RestoreOperation{Block: block, Sector: sector, KeyType: kind, Key: value, Data: request.Dump.Blocks[block].Data})
		}
		if selected[trailerBlock] {
			kind, value, ok := choosePermittedKey(currentAccess.Groups[3].FullTrailerWriteKeys(), verified)
			if !ok {
				return RestorePlan{}, nfc.NewError("restore preflight", nfc.CodeInvalidArgument, fmt.Sprintf("target access conditions do not permit a complete trailer write in sector %d", sector), nil)
			}
			trailerOperations = append(trailerOperations, RestoreOperation{Block: trailerBlock, Sector: sector, Trailer: true, KeyType: kind, Key: value, Data: request.Dump.Blocks[trailerBlock].Data})
		}
	}
	operations := append(dataOperations, trailerOperations...)
	return RestorePlan{Card: request.Card.Clone(), Layout: layout, Operations: operations}, nil
}

func selectedRestoreBlocks(layout mifare.Layout, requested []int) (map[int]bool, error) {
	count, err := layout.BlockCount()
	if err != nil {
		return nil, err
	}
	selected := make(map[int]bool, count)
	if requested == nil {
		for block := 1; block < count; block++ {
			selected[block] = true
		}
		return selected, nil
	}
	for _, block := range requested {
		if block < 0 || block >= count {
			return nil, nfc.NewError("restore preflight", nfc.CodeInvalidArgument, fmt.Sprintf("block %d is outside the card layout", block), nil)
		}
		if selected[block] {
			return nil, nfc.NewError("restore preflight", nfc.CodeInvalidArgument, fmt.Sprintf("block %d is selected more than once", block), nil)
		}
		if block != 0 {
			selected[block] = true
		}
	}
	return selected, nil
}

func choosePermittedKey(mask mifare.KeyMask, keys VerifiedSectorKeys) (nfc.KeyType, nfc.Key, bool) {
	if mask.AllowsA() && keys.KeyA != nil {
		return nfc.KeyTypeA, *keys.KeyA, true
	}
	if mask.AllowsB() && keys.KeyB != nil {
		return nfc.KeyTypeB, *keys.KeyB, true
	}
	return 0, nfc.Key{}, false
}

func verifyRestoredTrailer(ctx context.Context, reader nfc.Reader, card nfc.CardInfo, operation RestoreOperation) error {
	newA := nfc.Key(operation.Data[0:6])
	newB := nfc.Key(operation.Data[10:16])
	if err := selectExpectedCard(ctx, reader, card); err != nil {
		return err
	}
	if err := reader.Authenticate(ctx, byte(operation.Block), nfc.KeyTypeA, newA); err != nil {
		return nfc.NewError("verify sector trailer", nfc.CodeVerificationFailed, "replacement Key A did not authenticate", err)
	}
	actual, err := reader.ReadBlock(ctx, byte(operation.Block))
	if err != nil {
		return err
	}
	if actual[6] != operation.Data[6] || actual[7] != operation.Data[7] || actual[8] != operation.Data[8] || actual[9] != operation.Data[9] {
		return nfc.NewError("verify sector trailer", nfc.CodeVerificationFailed, "replacement access bytes do not match readback", nil)
	}
	access, _ := mifare.DecodeAccessBits([3]byte{operation.Data[6], operation.Data[7], operation.Data[8]})
	if access.Groups[3].KeyBReadable() {
		if actual[10] != operation.Data[10] || actual[11] != operation.Data[11] || actual[12] != operation.Data[12] || actual[13] != operation.Data[13] || actual[14] != operation.Data[14] || actual[15] != operation.Data[15] {
			return nfc.NewError("verify sector trailer", nfc.CodeVerificationFailed, "readable replacement Key B bytes do not match", nil)
		}
		return nil
	}
	if err := selectExpectedCard(ctx, reader, card); err != nil {
		return err
	}
	if err := reader.Authenticate(ctx, byte(operation.Block), nfc.KeyTypeB, newB); err != nil {
		return nfc.NewError("verify sector trailer", nfc.CodeVerificationFailed, "replacement Key B did not authenticate", err)
	}
	return nil
}

func newRestoreResult(request RestoreRequest) RestoreResult {
	dump := request.Dump
	selected, _ := selectedRestoreBlocks(dump.Layout, request.Blocks)
	result := RestoreResult{Phase: RestorePreflight, Blocks: make([]RestoreBlockResult, len(dump.Blocks))}
	for index := range result.Blocks {
		sector := 0
		trailer := false
		if dump.Layout.Valid() {
			sector, _ = dump.Layout.SectorForBlock(index)
			trailer, _ = dump.Layout.IsTrailer(index)
		}
		status := RestoreSkippedUnchanged
		if index == 0 {
			status = RestoreSkippedProtected
		} else if selected[index] {
			status = RestoreNotWritten
		}
		result.Blocks[index] = RestoreBlockResult{Block: index, Sector: sector, Trailer: trailer, Status: status}
	}
	return result
}

func markRestoreFailure(entry *RestoreBlockResult, attempted, written bool, err error) {
	entry.Attempted = attempted
	entry.Written = written
	entry.WriteOutcomeKnown = !attempted || written
	entry.Status = RestorePreconditionFailed
	if attempted && !written {
		entry.Status = RestoreWriteOutcomeUnknown
	} else if written {
		entry.Status = RestoreVerifyFailed
	}
	entry.Error = err.Error()
	entry.ErrorCode = nfcErrorName(err)
}

func finalizeRestoreResult(result *RestoreResult) {
	for index := range result.Blocks {
		entry := &result.Blocks[index]
		if entry.Written {
			result.WrittenBlocks = append(result.WrittenBlocks, entry.Block)
		}
		if entry.Verified {
			result.VerifiedBlocks = append(result.VerifiedBlocks, entry.Block)
		}
		if entry.Status == RestoreNotWritten {
			result.UnwrittenBlocks = append(result.UnwrittenBlocks, entry.Block)
		}
		if entry.Status == RestorePreconditionFailed || entry.Status == RestoreWriteOutcomeUnknown || entry.Status == RestoreVerifyFailed {
			block := entry.Block
			result.FailedBlock = &block
		}
	}
}
