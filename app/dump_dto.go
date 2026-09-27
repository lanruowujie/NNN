package app

import (
	"fmt"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/workbench"
	"github.com/BennyThink/NFCX/internal/workflow"
)

// DumpResultDTO exposes block certainty and provenance without sending secret
// key values in this legacy result DTO. Dedicated key and workbench DTOs expose
// complete values where the GUI presents them.
type DumpResultDTO struct {
	Complete bool               `json:"complete"`
	Layout   string             `json:"layout"`
	Blocks   []DumpBlockDTO     `json:"blocks"`
	Keys     []DumpSectorKeyDTO `json:"keys"`
	Card     CardDTO            `json:"card"`
	Device   DeviceDTO          `json:"device"`
}

type DumpBlockDTO struct {
	Block      int    `json:"block"`
	Sector     int    `json:"sector"`
	Hex        string `json:"hex"`
	KnownMask  string `json:"knownMask"`
	Status     string `json:"status"`
	ErrorCode  string `json:"errorCode,omitempty"`
	Error      string `json:"error,omitempty"`
	KeyASource string `json:"keyASource,omitempty"`
	KeyBSource string `json:"keyBSource,omitempty"`
}

type DumpSectorKeyDTO struct {
	Sector     int    `json:"sector"`
	KeyAStatus string `json:"keyAStatus"`
	KeyBStatus string `json:"keyBStatus"`
}

// RestoreResultDTO is deliberately separate from the workflow type so Wails
// bindings never need to expose a RestorePlan containing authentication keys.
type RestoreResultDTO struct {
	Phase           string                  `json:"phase"`
	Blocks          []RestoreBlockResultDTO `json:"blocks"`
	WrittenBlocks   []int                   `json:"writtenBlocks"`
	VerifiedBlocks  []int                   `json:"verifiedBlocks"`
	UnwrittenBlocks []int                   `json:"unwrittenBlocks"`
	FailedBlock     *int                    `json:"failedBlock,omitempty"`
}

type RestoreBlockResultDTO struct {
	Block             int    `json:"block"`
	Sector            int    `json:"sector"`
	Trailer           bool   `json:"trailer"`
	Status            string `json:"status"`
	Attempted         bool   `json:"attempted"`
	Written           bool   `json:"written"`
	WriteOutcomeKnown bool   `json:"writeOutcomeKnown"`
	Verified          bool   `json:"verified"`
	ErrorCode         string `json:"errorCode,omitempty"`
	Error             string `json:"error,omitempty"`
}

func dumpResultDTO(result workflow.DumpResult) DumpResultDTO {
	dto := DumpResultDTO{
		Complete: result.Complete(), Layout: dumpLayoutName(result.Dump.Layout),
		Blocks: make([]DumpBlockDTO, len(result.Dump.Blocks)), Keys: make([]DumpSectorKeyDTO, len(result.Keys)),
		Card: cardDTO(result.Card), Device: deviceDTO(result.Device),
	}
	for index, block := range result.Dump.Blocks {
		kind := workbench.KindForBlock(result.Dump.Layout, index)
		dto.Blocks[index] = DumpBlockDTO{
			Block: block.Number, Sector: block.Sector, Hex: visibleBlockHex(block, kind), KnownMask: fmt.Sprintf("%04X", block.KnownMask),
			Status: string(block.Status), ErrorCode: block.ErrorCode, Error: block.Error, KeyASource: block.KeyASource, KeyBSource: block.KeyBSource,
		}
	}
	for index, keys := range result.Keys {
		dto.Keys[index] = DumpSectorKeyDTO{Sector: keys.Sector, KeyAStatus: "unavailable", KeyBStatus: "unavailable"}
		if keys.KeyA != nil {
			dto.Keys[index].KeyAStatus = "verified"
		}
		if keys.KeyB != nil {
			dto.Keys[index].KeyBStatus = "verified"
		}
	}
	return dto
}

func dumpLayoutName(layout mifare.Layout) string {
	switch layout {
	case mifare.Classic1K:
		return "classic_1k"
	case mifare.Classic4K:
		return "classic_4k"
	default:
		return "unknown"
	}
}

func restoreResultDTO(result workflow.RestoreResult) RestoreResultDTO {
	var failedBlock *int
	if result.FailedBlock != nil {
		value := *result.FailedBlock
		failedBlock = &value
	}
	dto := RestoreResultDTO{
		Phase: string(result.Phase), Blocks: make([]RestoreBlockResultDTO, len(result.Blocks)),
		WrittenBlocks: append([]int(nil), result.WrittenBlocks...), VerifiedBlocks: append([]int(nil), result.VerifiedBlocks...),
		UnwrittenBlocks: append([]int(nil), result.UnwrittenBlocks...), FailedBlock: failedBlock,
	}
	for index, block := range result.Blocks {
		dto.Blocks[index] = RestoreBlockResultDTO{
			Block: block.Block, Sector: block.Sector, Trailer: block.Trailer, Status: string(block.Status),
			Attempted: block.Attempted, Written: block.Written, WriteOutcomeKnown: block.WriteOutcomeKnown,
			Verified: block.Verified, ErrorCode: block.ErrorCode, Error: block.Error,
		}
	}
	return dto
}
