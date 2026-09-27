package app

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/workbench"
	"github.com/BennyThink/NFCX/internal/workflow"
)

type KeyDTO struct {
	ID        string   `json:"id"`
	Value     string   `json:"value"`
	Sources   []string `json:"sources"`
	Builtin   bool     `json:"builtin"`
	Removable bool     `json:"removable"`
}

type ImportIssueDTO struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

type ImportReportDTO struct {
	Cancelled  bool             `json:"cancelled"`
	Path       string           `json:"path,omitempty"`
	Valid      int              `json:"valid"`
	Added      int              `json:"added"`
	Duplicates int              `json:"duplicates"`
	Ignored    int              `json:"ignored"`
	Issues     []ImportIssueDTO `json:"issues"`
	Keys       []KeyDTO         `json:"keys"`
}

type ExportResultDTO struct {
	Cancelled bool   `json:"cancelled"`
	Path      string `json:"path,omitempty"`
	Count     int    `json:"count"`
}

type WorkbenchDTO struct {
	Loaded      bool                   `json:"loaded"`
	Saved       bool                   `json:"saved"`
	Name        string                 `json:"name,omitempty"`
	Layout      string                 `json:"layout,omitempty"`
	Complete    bool                   `json:"complete"`
	Dirty       bool                   `json:"dirty"`
	DirtyBlocks []int                  `json:"dirtyBlocks"`
	Blocks      []WorkbenchBlockDTO    `json:"blocks"`
	Diffs       []WorkbenchDiffDTO     `json:"diffs"`
	Validation  WorkbenchValidationDTO `json:"validation"`
	SourceUID   string                 `json:"sourceUid,omitempty"`
	UIDLength   int                    `json:"uidLength"`
}

type WorkbenchBlockDTO struct {
	Block     int               `json:"block"`
	Sector    int               `json:"sector"`
	Kind      string            `json:"kind"`
	Hex       string            `json:"hex"`
	ASCII     string            `json:"ascii"`
	KnownMask string            `json:"knownMask"`
	Status    string            `json:"status"`
	Dirty     bool              `json:"dirty"`
	Warning   string            `json:"warning,omitempty"`
	Access    *AccessSummaryDTO `json:"access,omitempty"`
}

// EditableBlockDTO is returned after an explicit editor action and contains the
// same plaintext block bytes shown in the workbench snapshot.
type EditableBlockDTO struct {
	Block   int               `json:"block"`
	Sector  int               `json:"sector"`
	Kind    string            `json:"kind"`
	Hex     string            `json:"hex"`
	ASCII   string            `json:"ascii"`
	Warning string            `json:"warning,omitempty"`
	Access  *AccessSummaryDTO `json:"access,omitempty"`
}

type WorkbenchDiffDTO struct {
	Block  int           `json:"block"`
	Sector int           `json:"sector"`
	Kind   string        `json:"kind"`
	Bytes  []ByteDiffDTO `json:"bytes"`
}

type ByteDiffDTO struct {
	Offset   int    `json:"offset"`
	Before   string `json:"before"`
	After    string `json:"after"`
	WasKnown bool   `json:"wasKnown"`
	NowKnown bool   `json:"nowKnown"`
}

type WorkbenchValidationDTO struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

type AccessSummaryDTO struct {
	Valid  bool             `json:"valid"`
	Error  string           `json:"error,omitempty"`
	Groups []AccessGroupDTO `json:"groups"`
}

type AccessGroupDTO struct {
	Group            int    `json:"group"`
	Code             string `json:"code"`
	DataWriteKeys    string `json:"dataWriteKeys"`
	TrailerWriteKeys string `json:"trailerWriteKeys"`
	KeyBReadable     bool   `json:"keyBReadable"`
}

type WritePreflightDTO struct {
	Token         string              `json:"token,omitempty"`
	Mode          string              `json:"mode"`
	Valid         bool                `json:"valid"`
	Blocks        []int               `json:"blocks"`
	TrailerBlocks []int               `json:"trailerBlocks"`
	SkippedBlock0 bool                `json:"skippedBlock0"`
	Warnings      []string            `json:"warnings"`
	Error         string              `json:"error,omitempty"`
	Access        []TrailerAccessDTO  `json:"access"`
	UID           *UIDWritePreviewDTO `json:"uid,omitempty"`
}

type WritePreflightRequestDTO struct {
	Mode     string `json:"mode"`
	WriteUID bool   `json:"writeUid"`
	UID      string `json:"uid,omitempty"`
}

type TrailerAccessDTO struct {
	Block  int              `json:"block"`
	Sector int              `json:"sector"`
	Access AccessSummaryDTO `json:"access"`
}

type WriteStartRequestDTO struct {
	Token           string `json:"token"`
	ConfirmTrailers bool   `json:"confirmTrailers"`
	ConfirmUID      string `json:"confirmUid,omitempty"`
}

type UIDWritePreviewDTO struct {
	CurrentUID string `json:"currentUid"`
	NewUID     string `json:"newUid"`
	BCC        string `json:"bcc"`
	OldBlock0  string `json:"oldBlock0"`
	NewBlock0  string `json:"newBlock0"`
}

type UIDWritePreflightDTO struct {
	Token   string              `json:"token,omitempty"`
	Valid   bool                `json:"valid"`
	Preview *UIDWritePreviewDTO `json:"preview,omitempty"`
	Warning string              `json:"warning,omitempty"`
	Error   string              `json:"error,omitempty"`
}

type UIDWriteStartRequestDTO struct {
	Token        string `json:"token"`
	Confirmation string `json:"confirmation"`
}

type UIDWriteResultDTO struct {
	OldUID       string `json:"oldUid"`
	NewUID       string `json:"newUid,omitempty"`
	OldBlock0    string `json:"oldBlock0"`
	NewBlock0    string `json:"newBlock0"`
	ActualBlock0 string `json:"actualBlock0,omitempty"`
	BackupPath   string `json:"backupPath,omitempty"`
	Verified     bool   `json:"verified"`
}

type SaveResultDTO struct {
	Cancelled bool   `json:"cancelled"`
	Path      string `json:"path,omitempty"`
	Sidecar   string `json:"sidecar,omitempty"`
}

type TaskEventDetailDTO struct {
	TaskID        string                `json:"taskId"`
	Kind          string                `json:"kind"`
	Type          string                `json:"type"`
	Phase         string                `json:"phase,omitempty"`
	Progress      int                   `json:"progress"`
	Sector        int                   `json:"sector,omitempty"`
	KeyType       string                `json:"keyType,omitempty"`
	Attempted     int                   `json:"attempted,omitempty"`
	Found         int                   `json:"found,omitempty"`
	Block         int                   `json:"block,omitempty"`
	Message       string                `json:"message"`
	Time          string                `json:"time"`
	Workbench     *WorkbenchDTO         `json:"workbench,omitempty"`
	Restore       *RestoreResultDTO     `json:"restore,omitempty"`
	Engine        string                `json:"engine,omitempty"`
	Version       string                `json:"version,omitempty"`
	Stream        string                `json:"stream,omitempty"`
	Log           string                `json:"log,omitempty"`
	Sequence      uint64                `json:"sequence,omitempty"`
	ExitCode      *int                  `json:"exitCode,omitempty"`
	RecoveryError string                `json:"recoveryError,omitempty"`
	MFoC          *MFoCResultDTO        `json:"mfoc,omitempty"`
	MFCUK         *MFCUKResultDTO       `json:"mfcuk,omitempty"`
	Step          int                   `json:"step,omitempty"`
	TotalSteps    int                   `json:"totalSteps,omitempty"`
	StepStatus    string                `json:"stepStatus,omitempty"`
	Indeterminate bool                  `json:"indeterminate,omitempty"`
	Recovery      *KeyRecoveryResultDTO `json:"recovery,omitempty"`
	UID           *UIDWriteResultDTO    `json:"uid,omitempty"`
}

type KeyRecoveryStartRequestDTO struct {
	Authorized   bool `json:"authorized"`
	DiscardDirty bool `json:"discardDirty"`
}

type KeyRecoveryStepDTO struct {
	Number  int    `json:"number"`
	Stage   string `json:"stage"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type KeyRecoveryResultDTO struct {
	Outcome             string               `json:"outcome"`
	Steps               []KeyRecoveryStepDTO `json:"steps"`
	VerifiedKeySlots    int                  `json:"verifiedKeySlots"`
	CoveredSectors      int                  `json:"coveredSectors"`
	ReadableSectors     int                  `json:"readableSectors"`
	TotalSectors        int                  `json:"totalSectors"`
	DumpComplete        bool                 `json:"dumpComplete"`
	FailedStep          int                  `json:"failedStep,omitempty"`
	FailedStage         string               `json:"failedStage,omitempty"`
	Reason              string               `json:"reason,omitempty"`
	HardnestedAvailable bool                 `json:"hardnestedAvailable"`
	ReaderRecovered     bool                 `json:"readerRecovered"`
}

type MFoCStartRequestDTO struct {
	Authorized bool `json:"authorized"`
}

type MFoCResultDTO struct {
	OutputValidated bool `json:"outputValidated"`
	Claimed         int  `json:"claimed"`
	Added           int  `json:"added"`
	Existing        int  `json:"existing"`
	Rejected        int  `json:"rejected"`
	Conflicts       int  `json:"conflicts"`
	FilledBytes     int  `json:"filledBytes"`
	DataConflicts   int  `json:"dataConflicts"`
	KeyConflicts    int  `json:"keyConflicts"`
}

type MFCUKStartRequestDTO struct {
	Authorized bool `json:"authorized"`
}

type MFCUKResultDTO struct {
	CandidateOutputValidated bool   `json:"candidateOutputValidated"`
	Candidates               int    `json:"candidates"`
	Verified                 int    `json:"verified"`
	Existing                 int    `json:"existing"`
	Rejected                 int    `json:"rejected"`
	Conflicts                int    `json:"conflicts"`
	MFCUKVersion             string `json:"mfcukVersion,omitempty"`
	MFCUKDurationMillis      int64  `json:"mfcukDurationMillis"`
	MFoCOutputValidated      bool   `json:"mfocOutputValidated"`
	MFoCVersion              string `json:"mfocVersion,omitempty"`
	MFoCDurationMillis       int64  `json:"mfocDurationMillis"`
	MFoCAdded                int    `json:"mfocAdded"`
	MFoCRejected             int    `json:"mfocRejected"`
	FilledBytes              int    `json:"filledBytes"`
	DataConflicts            int    `json:"dataConflicts"`
	KeyConflicts             int    `json:"keyConflicts"`
}

func keyDTOs(store *keys.Store) []KeyDTO {
	if store == nil {
		return []KeyDTO{}
	}
	entries := store.Entries()
	result := make([]KeyDTO, 0, len(entries))
	for _, entry := range entries {
		sources := make([]string, len(entry.Sources))
		builtin := false
		for index, source := range entry.Sources {
			sources[index] = string(source)
			builtin = builtin || source == keys.SourceBuiltin
		}
		result = append(result, KeyDTO{ID: entry.ID, Value: entry.Hex(), Sources: sources, Builtin: builtin, Removable: !builtin || len(sources) > 1})
	}
	return result
}

func sectorKeyDTOs(store *keys.Store, card *nfc.CardInfo) []SectorKeyDTO {
	if store == nil || card == nil {
		return []SectorKeyDTO{}
	}
	var layout mifare.Layout
	switch nfc.InferCardType(*card) {
	case nfc.CardTypeMIFAREClassic1K:
		layout = mifare.Classic1K
	case nfc.CardTypeMIFAREClassic4K:
		layout = mifare.Classic4K
	default:
		return []SectorKeyDTO{}
	}
	sectors, _ := layout.SectorCount()
	result := make([]SectorKeyDTO, sectors)
	for sector := range result {
		result[sector] = SectorKeyDTO{Sector: sector, KeyAStatus: "unknown", KeyBStatus: "unknown"}
	}
	for _, match := range store.Verified(keys.CardID(*card)) {
		record, exists := store.Record(match.KeyID)
		if !exists || match.Sector < 0 || match.Sector >= len(result) {
			continue
		}
		if match.KeyType == nfc.KeyTypeA {
			result[match.Sector].KeyAStatus = "verified"
			result[match.Sector].KeyAID = match.KeyID
			result[match.Sector].KeyA = record.Hex()
		} else {
			result[match.Sector].KeyBStatus = "verified"
			result[match.Sector].KeyBID = match.KeyID
			result[match.Sector].KeyB = record.Hex()
		}
	}
	return result
}

func workbenchDTO(snapshot workbench.Snapshot) WorkbenchDTO {
	if !snapshot.Loaded {
		return WorkbenchDTO{Blocks: []WorkbenchBlockDTO{}, Diffs: []WorkbenchDiffDTO{}, DirtyBlocks: []int{}, Validation: WorkbenchValidationDTO{Valid: false, Errors: []string{"尚未加载 dump"}, Warnings: []string{}}}
	}
	dirty := make(map[int]bool, len(snapshot.DirtyBlocks))
	for _, block := range snapshot.DirtyBlocks {
		dirty[block] = true
	}
	blocks := make([]WorkbenchBlockDTO, len(snapshot.Result.Dump.Blocks))
	for index, block := range snapshot.Result.Dump.Blocks {
		kind := workbench.KindForBlock(snapshot.Result.Dump.Layout, index)
		dto := WorkbenchBlockDTO{
			Block: index, Sector: block.Sector, Kind: string(kind), Hex: visibleBlockHex(block, kind), ASCII: visibleBlockASCII(block, kind),
			KnownMask: fmt.Sprintf("%04X", block.KnownMask), Status: string(block.Status), Dirty: dirty[index],
		}
		if kind == workbench.BlockManufacturer {
			dto.Warning = "Manufacturer block：本阶段保存可用，但写卡时始终跳过"
		}
		if kind == workbench.BlockTrailer {
			dto.Warning = "Sector trailer：写入会改变密钥或访问权限，需要额外确认"
			explanation := accessDTO(workbench.ExplainAccess(block))
			dto.Access = &explanation
		}
		blocks[index] = dto
	}
	diffs := make([]WorkbenchDiffDTO, len(snapshot.Diffs))
	for index, diff := range snapshot.Diffs {
		dto := WorkbenchDiffDTO{Block: diff.Block, Sector: diff.Sector, Kind: string(diff.Kind), Bytes: make([]ByteDiffDTO, len(diff.Bytes))}
		for byteIndex, item := range diff.Bytes {
			dto.Bytes[byteIndex] = ByteDiffDTO{
				Offset: item.Offset, Before: fmt.Sprintf("%02X", item.Before), After: fmt.Sprintf("%02X", item.After),
				WasKnown: item.WasKnown, NowKnown: item.NowKnown,
			}
		}
		diffs[index] = dto
	}
	validation := snapshotValidation(snapshot)
	name := ""
	if snapshot.Path != "" {
		name = filepath.Base(snapshot.Path)
	}
	return WorkbenchDTO{
		Loaded: true, Saved: snapshot.Path != "", Name: name, Layout: dumpLayoutName(snapshot.Result.Dump.Layout), Complete: snapshot.Result.Complete(),
		Dirty: snapshot.Dirty, DirtyBlocks: append([]int(nil), snapshot.DirtyBlocks...), Blocks: blocks, Diffs: diffs, Validation: validation,
		SourceUID: formatHex(snapshot.Result.Card.UID), UIDLength: len(snapshot.Result.Card.UID),
	}
}

func visibleBlockHex(block mifare.Block, _ workbench.BlockKind) string {
	return strings.ToUpper(block.Hex())
}

func visibleBlockASCII(block mifare.Block, _ workbench.BlockKind) string {
	return workbench.ASCII(block.Data)
}

func snapshotValidation(snapshot workbench.Snapshot) WorkbenchValidationDTO {
	validation := WorkbenchValidationDTO{Valid: true, Errors: []string{}, Warnings: []string{}}
	if !snapshot.Result.Complete() {
		validation.Valid = false
		validation.Errors = append(validation.Errors, "dump 包含未知字节，只能保存为 NFCX 工程，不能写卡")
		return validation
	}
	if _, err := snapshot.Result.Dump.ValidateBCC(len(snapshot.Result.Card.UID)); err != nil {
		validation.Valid = false
		validation.Errors = append(validation.Errors, err.Error())
	}
	if err := snapshot.Result.Dump.ValidateTrailers(); err != nil {
		validation.Valid = false
		validation.Errors = append(validation.Errors, err.Error())
	}
	for _, block := range snapshot.DirtyBlocks {
		kind := workbench.KindForBlock(snapshot.Result.Dump.Layout, block)
		if kind == workbench.BlockManufacturer {
			validation.Warnings = append(validation.Warnings, "block 0 已修改，但写卡时会跳过")
		} else if kind == workbench.BlockTrailer {
			validation.Warnings = append(validation.Warnings, fmt.Sprintf("block %d 是 sector trailer，写入前需要额外确认", block))
		}
	}
	return validation
}

func accessDTO(summary workbench.AccessSummary) AccessSummaryDTO {
	dto := AccessSummaryDTO{Valid: summary.Valid, Error: summary.Error, Groups: make([]AccessGroupDTO, len(summary.Groups))}
	for index, group := range summary.Groups {
		dto.Groups[index] = AccessGroupDTO{Group: group.Group, Code: group.Code, DataWriteKeys: group.DataWriteKeys, TrailerWriteKeys: group.TrailerWriteKeys, KeyBReadable: group.KeyBReadable}
	}
	return dto
}

func blockDTOs(snapshot workbench.Snapshot) []BlockDTO {
	if !snapshot.Loaded {
		return []BlockDTO{}
	}
	result := make([]BlockDTO, 0, len(snapshot.Result.Dump.Blocks))
	for index, block := range snapshot.Result.Dump.Blocks {
		kind := workbench.KindForBlock(snapshot.Result.Dump.Layout, index)
		result = append(result, BlockDTO{Block: index, Sector: block.Sector, Kind: string(kind), Hex: visibleBlockHex(block, kind), Status: string(block.Status)})
	}
	return result
}

func verifiedSectorKeys(store *keys.Store, card nfc.CardInfo, layout mifare.Layout) []workflow.SectorKeys {
	sectors, _ := layout.SectorCount()
	result := make([]workflow.SectorKeys, sectors)
	for sector := range result {
		result[sector].Sector = sector
	}
	for _, match := range store.Verified(keys.CardID(card)) {
		if match.Sector < 0 || match.Sector >= sectors {
			continue
		}
		record, exists := store.Record(match.KeyID)
		if !exists {
			continue
		}
		key := record.Value
		if match.KeyType == nfc.KeyTypeA {
			result[match.Sector].KeyA = &key
		} else {
			result[match.Sector].KeyB = &key
		}
	}
	return result
}

func keyTypeName(keyType nfc.KeyType) string {
	if keyType == nfc.KeyTypeB {
		return "B"
	}
	return "A"
}

func sortedInts(values []int) []int {
	result := append([]int(nil), values...)
	sort.Ints(result)
	return result
}
