// Package workbench owns the editable in-memory copy of a card dump. The GUI
// only receives snapshots and asks this model to perform byte-level edits.
package workbench

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/workflow"
)

var ErrNoDump = errors.New("workbench has no dump")

type BlockKind string

const (
	BlockManufacturer BlockKind = "manufacturer"
	BlockData         BlockKind = "data"
	BlockTrailer      BlockKind = "trailer"
)

type ByteDiff struct {
	Offset   int
	Before   byte
	After    byte
	WasKnown bool
	NowKnown bool
}

type BlockDiff struct {
	Block  int
	Sector int
	Kind   BlockKind
	Bytes  []ByteDiff
}

type Validation struct {
	Valid    bool
	Errors   []string
	Warnings []string
}

type MergeConflict struct {
	Block  int
	Offset int
}

type KeyMergeConflict struct {
	Sector  int
	KeyType nfc.KeyType
}

type MergeReport struct {
	Loaded       bool
	FilledBytes  int
	Conflicts    []MergeConflict
	KeyConflicts []KeyMergeConflict
}

type Snapshot struct {
	Loaded      bool
	Path        string
	Dirty       bool
	DirtyBlocks []int
	Result      workflow.DumpResult
	Diffs       []BlockDiff
}

type edit struct {
	block  int
	before mifare.Block
	after  mifare.Block
}

type Model struct {
	mu       sync.RWMutex
	loaded   bool
	path     string
	baseline workflow.DumpResult
	working  workflow.DumpResult
	history  []edit
}

func New() *Model { return &Model{} }

func cloneResult(value workflow.DumpResult) workflow.DumpResult {
	value.Dump = value.Dump.Clone()
	value.Card = value.Card.Clone()
	value.Keys = append([]workflow.VerifiedSectorKeys(nil), value.Keys...)
	for index := range value.Keys {
		if value.Keys[index].KeyA != nil {
			key := *value.Keys[index].KeyA
			value.Keys[index].KeyA = &key
		}
		if value.Keys[index].KeyB != nil {
			key := *value.Keys[index].KeyB
			value.Keys[index].KeyB = &key
		}
	}
	return value
}

func (m *Model) Load(result workflow.DumpResult, path string) error {
	if !result.Dump.Layout.Valid() || len(result.Dump.Blocks) == 0 {
		return errors.New("a valid MIFARE Classic dump is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loaded = true
	m.path = path
	m.baseline = cloneResult(result)
	m.working = cloneResult(result)
	m.history = nil
	return nil
}

// MergeReadResult incorporates a fresh in-process card read without replacing
// known bytes or keys already present in the workbench. Background additions
// are merged into both baseline and working copies so existing user edits stay
// dirty while newly read bytes do not appear as edits.
func (m *Model) MergeReadResult(result workflow.DumpResult) (MergeReport, error) {
	if !result.Dump.Layout.Valid() || len(result.Dump.Blocks) == 0 {
		return MergeReport{}, errors.New("a valid MIFARE Classic dump is required")
	}
	blockCount, err := result.Dump.Layout.BlockCount()
	if err != nil || len(result.Dump.Blocks) != blockCount {
		return MergeReport{}, errors.New("fresh card read has an invalid block count")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.loaded {
		m.loaded = true
		m.path = ""
		m.baseline = cloneResult(result)
		m.working = cloneResult(result)
		m.history = nil
		return MergeReport{Loaded: true}, nil
	}
	if m.working.Dump.Layout != result.Dump.Layout || !nfc.SameCard(m.working.Card, result.Card) {
		return MergeReport{}, errors.New("fresh card read does not match the loaded workbench")
	}
	report := MergeReport{}
	for index := range m.working.Dump.Blocks {
		mergeBlock(&m.baseline.Dump.Blocks[index], result.Dump.Blocks[index], nil)
		mergeBlock(&m.working.Dump.Blocks[index], result.Dump.Blocks[index], &report)
	}
	for index := range m.history {
		incoming := result.Dump.Blocks[m.history[index].block]
		mergeBlock(&m.history[index].before, incoming, nil)
		mergeBlock(&m.history[index].after, incoming, nil)
	}
	mergeResultKeys(&m.baseline, result, nil)
	mergeResultKeys(&m.working, result, &report)
	m.baseline.Device, m.working.Device = result.Device, result.Device
	m.baseline.FinishedAt, m.working.FinishedAt = result.FinishedAt, result.FinishedAt
	return report, nil
}

func mergeBlock(destination *mifare.Block, incoming mifare.Block, report *MergeReport) {
	filled := false
	for offset := 0; offset < nfc.BlockSize; offset++ {
		mask := uint16(1 << offset)
		known, offered := destination.KnownMask&mask != 0, incoming.KnownMask&mask != 0
		switch {
		case !known && offered:
			destination.Data[offset] = incoming.Data[offset]
			destination.KnownMask |= mask
			filled = true
			if report != nil {
				report.FilledBytes++
			}
		case known && offered && destination.Data[offset] != incoming.Data[offset]:
			if report != nil {
				report.Conflicts = append(report.Conflicts, MergeConflict{Block: destination.Number, Offset: offset})
			}
		}
	}
	if filled {
		destination.Error = ""
		destination.ErrorCode = ""
		if destination.Complete() {
			destination.Status = incoming.Status
		}
	}
}

func mergeResultKeys(destination *workflow.DumpResult, incoming workflow.DumpResult, report *MergeReport) {
	sectors, err := destination.Dump.Layout.SectorCount()
	if err != nil {
		return
	}
	if len(destination.Keys) != sectors {
		expanded := make([]workflow.VerifiedSectorKeys, sectors)
		for sector := range expanded {
			expanded[sector].Sector = sector
		}
		for _, existing := range destination.Keys {
			if existing.Sector >= 0 && existing.Sector < sectors {
				expanded[existing.Sector] = existing
			}
		}
		destination.Keys = expanded
	}
	for _, offered := range incoming.Keys {
		if offered.Sector < 0 || offered.Sector >= len(destination.Keys) {
			continue
		}
		stored := &destination.Keys[offered.Sector]
		mergeResultKey(&stored.KeyA, offered.KeyA, offered.Sector, nfc.KeyTypeA, report)
		mergeResultKey(&stored.KeyB, offered.KeyB, offered.Sector, nfc.KeyTypeB, report)
	}
}

func mergeResultKey(destination **nfc.Key, incoming *nfc.Key, sector int, keyType nfc.KeyType, report *MergeReport) {
	if incoming == nil {
		return
	}
	if *destination == nil {
		key := *incoming
		*destination = &key
		return
	}
	if **destination != *incoming && report != nil {
		report.KeyConflicts = append(report.KeyConflicts, KeyMergeConflict{Sector: sector, KeyType: keyType})
	}
}

func (m *Model) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loaded = false
	m.path = ""
	m.baseline = workflow.DumpResult{}
	m.working = workflow.DumpResult{}
	m.history = nil
}

func (m *Model) Result() (workflow.DumpResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.loaded {
		return workflow.DumpResult{}, ErrNoDump
	}
	return cloneResult(m.working), nil
}

func (m *Model) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.loaded {
		return Snapshot{}
	}
	diffs := calculateDiffs(m.baseline.Dump, m.working.Dump)
	dirtyBlocks := make([]int, len(diffs))
	for index, diff := range diffs {
		dirtyBlocks[index] = diff.Block
	}
	return Snapshot{
		Loaded: true, Path: m.path, Dirty: len(diffs) != 0, DirtyBlocks: dirtyBlocks,
		Result: cloneResult(m.working), Diffs: diffs,
	}
}

func (m *Model) EditHex(block int, value string) error {
	compact := strings.Map(func(character rune) rune {
		switch character {
		case ' ', '\t', '\r', '\n':
			return -1
		default:
			return character
		}
	}, value)
	decoded, err := hex.DecodeString(compact)
	if err != nil || len(decoded) != nfc.BlockSize {
		return fmt.Errorf("block data must contain exactly 16 hexadecimal bytes")
	}
	var data [nfc.BlockSize]byte
	copy(data[:], decoded)
	return m.editBlock(block, data)
}

func (m *Model) EditASCII(block int, value string) error {
	encoded := []byte(value)
	if !utf8.ValidString(value) || len(encoded) > nfc.BlockSize {
		return fmt.Errorf("ASCII edit must contain at most 16 ASCII bytes")
	}
	for _, value := range encoded {
		if value > 0x7f {
			return fmt.Errorf("ASCII edit must contain at most 16 ASCII bytes")
		}
	}
	var data [nfc.BlockSize]byte
	copy(data[:], encoded)
	return m.editBlock(block, data)
}

func (m *Model) editBlock(block int, data [nfc.BlockSize]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.loaded {
		return ErrNoDump
	}
	if block < 0 || block >= len(m.working.Dump.Blocks) {
		return fmt.Errorf("block %d is outside the loaded dump", block)
	}
	before := m.working.Dump.Blocks[block]
	after := before
	after.Data = data
	after.KnownMask = mifare.AllBytesKnown
	after.Status = mifare.BlockSynthetic
	after.Error = ""
	after.ErrorCode = ""
	if before.Data == after.Data && before.KnownMask == after.KnownMask {
		return nil
	}
	m.working.Dump.Blocks[block] = after
	m.history = append(m.history, edit{block: block, before: before, after: after})
	return nil
}

func (m *Model) Undo() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.loaded {
		return ErrNoDump
	}
	if len(m.history) == 0 {
		return nil
	}
	last := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.working.Dump.Blocks[last.block] = last.before
	return nil
}

func (m *Model) Revert() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.loaded {
		return ErrNoDump
	}
	m.working = cloneResult(m.baseline)
	m.history = nil
	return nil
}

// MarkSaved makes the successfully persisted working copy the new baseline.
func (m *Model) MarkSaved(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.loaded {
		return ErrNoDump
	}
	m.path = path
	m.baseline = cloneResult(m.working)
	m.history = nil
	return nil
}

func (m *Model) ValidateForSave() Validation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.loaded {
		return Validation{Errors: []string{ErrNoDump.Error()}}
	}
	return validate(m.working)
}

func validate(result workflow.DumpResult) Validation {
	validation := Validation{Valid: true}
	if !result.Complete() {
		validation.Valid = false
		validation.Errors = append(validation.Errors, "dump 包含未知字节，只能保存为 NFCX 工程")
		return validation
	}
	if _, err := result.Dump.ValidateBCC(len(result.Card.UID)); err != nil {
		validation.Valid = false
		validation.Errors = append(validation.Errors, err.Error())
	}
	if err := result.Dump.ValidateTrailers(); err != nil {
		validation.Valid = false
		validation.Errors = append(validation.Errors, err.Error())
	}
	return validation
}

func calculateDiffs(baseline, working mifare.Dump) []BlockDiff {
	if baseline.Layout != working.Layout || len(baseline.Blocks) != len(working.Blocks) {
		return nil
	}
	result := make([]BlockDiff, 0)
	for block := range working.Blocks {
		before, after := baseline.Blocks[block], working.Blocks[block]
		diff := BlockDiff{Block: block, Sector: after.Sector, Kind: KindForBlock(working.Layout, block)}
		for offset := 0; offset < nfc.BlockSize; offset++ {
			mask := uint16(1 << offset)
			wasKnown, nowKnown := before.KnownMask&mask != 0, after.KnownMask&mask != 0
			if before.Data[offset] != after.Data[offset] || wasKnown != nowKnown {
				diff.Bytes = append(diff.Bytes, ByteDiff{Offset: offset, Before: before.Data[offset], After: after.Data[offset], WasKnown: wasKnown, NowKnown: nowKnown})
			}
		}
		if len(diff.Bytes) != 0 {
			result = append(result, diff)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Block < result[j].Block })
	return result
}

func KindForBlock(layout mifare.Layout, block int) BlockKind {
	if block == 0 {
		return BlockManufacturer
	}
	if trailer, err := layout.IsTrailer(block); err == nil && trailer {
		return BlockTrailer
	}
	return BlockData
}

func ASCII(data [nfc.BlockSize]byte) string {
	var output strings.Builder
	for _, value := range data {
		if value >= 0x20 && value <= 0x7e {
			output.WriteByte(value)
		} else {
			output.WriteRune('·')
		}
	}
	return output.String()
}

type AccessSummary struct {
	Valid  bool
	Error  string
	Groups [4]AccessGroup
}

type AccessGroup struct {
	Group            int
	Code             string
	DataWriteKeys    string
	TrailerWriteKeys string
	KeyBReadable     bool
}

func ExplainAccess(block mifare.Block) AccessSummary {
	bits, err := mifare.DecodeAccessBits([3]byte{block.Data[6], block.Data[7], block.Data[8]})
	if err != nil {
		return AccessSummary{Error: err.Error()}
	}
	summary := AccessSummary{Valid: true}
	for index, condition := range bits.Groups {
		summary.Groups[index] = AccessGroup{
			Group: index, Code: fmt.Sprintf("%03b", condition.Code()),
			DataWriteKeys:    keyMaskLabel(condition.DataWriteKeys()),
			TrailerWriteKeys: keyMaskLabel(condition.FullTrailerWriteKeys()),
			KeyBReadable:     condition.KeyBReadable(),
		}
	}
	return summary
}

func keyMaskLabel(mask mifare.KeyMask) string {
	switch {
	case mask.AllowsA() && mask.AllowsB():
		return "Key A 或 Key B"
	case mask.AllowsA():
		return "Key A"
	case mask.AllowsB():
		return "Key B"
	default:
		return "不可写"
	}
}
