package mifare

import (
	"encoding/hex"
	"errors"
	"fmt"
)

type BlockStatus string

const (
	BlockUnread     BlockStatus = "unread"
	BlockRead       BlockStatus = "read"
	BlockAuthFailed BlockStatus = "auth_failed"
	BlockReadFailed BlockStatus = "read_failed"
	BlockSynthetic  BlockStatus = "synthetic"
	BlockVerified   BlockStatus = "verified"
)

const AllBytesKnown uint16 = 0xffff

var ErrIncompleteDump = errors.New("MIFARE Classic dump is incomplete")

// Block retains byte-level certainty so masked trailer keys can never be
// confused with real zero bytes.
type Block struct {
	Number     int
	Sector     int
	Data       [BlockSize]byte
	KnownMask  uint16
	Status     BlockStatus
	ErrorCode  string
	Error      string
	KeyASource string
	KeyBSource string
}

func (b Block) Complete() bool { return b.KnownMask == AllBytesKnown }

// Dump is a geometry-aware in-memory image. Blocks always retain their raw
// address, including unavailable blocks in a partial read.
type Dump struct {
	Layout Layout
	Blocks []Block
}

func (d Dump) Clone() Dump {
	clone := d
	clone.Blocks = append([]Block(nil), d.Blocks...)
	return clone
}

func NewDump(layout Layout) (Dump, error) {
	count, err := layout.BlockCount()
	if err != nil {
		return Dump{}, err
	}
	result := Dump{Layout: layout, Blocks: make([]Block, count)}
	for block := range result.Blocks {
		sector, _ := layout.SectorForBlock(block)
		result.Blocks[block] = Block{Number: block, Sector: sector, Status: BlockUnread}
	}
	return result, nil
}

func ParseRaw(raw []byte) (Dump, error) {
	var layout Layout
	switch len(raw) {
	case Classic1KDumpSize:
		layout = Classic1K
	case Classic4KDumpSize:
		layout = Classic4K
	default:
		return Dump{}, fmt.Errorf("raw MIFARE Classic dump must be exactly %d or %d bytes, got %d", Classic1KDumpSize, Classic4KDumpSize, len(raw))
	}
	result, _ := NewDump(layout)
	for index := range result.Blocks {
		copy(result.Blocks[index].Data[:], raw[index*BlockSize:(index+1)*BlockSize])
		result.Blocks[index].KnownMask = AllBytesKnown
		result.Blocks[index].Status = BlockRead
	}
	return result, nil
}

func (d Dump) Complete() bool {
	count, err := d.Layout.BlockCount()
	if err != nil || len(d.Blocks) != count {
		return false
	}
	for index, block := range d.Blocks {
		if block.Number != index || !block.Complete() {
			return false
		}
	}
	return true
}

func (d Dump) Raw() ([]byte, error) {
	if !d.Complete() {
		return nil, ErrIncompleteDump
	}
	raw := make([]byte, 0, len(d.Blocks)*BlockSize)
	for _, block := range d.Blocks {
		raw = append(raw, block.Data[:]...)
	}
	return raw, nil
}

type BCCStatus string

const (
	BCCValid         BCCStatus = "valid"
	BCCInvalid       BCCStatus = "invalid"
	BCCNotApplicable BCCStatus = "not_applicable"
)

// BCCFor4ByteUID returns the XOR check byte stored at manufacturer-block byte
// four. Longer UIDs use a different cascade layout and are rejected here.
func BCCFor4ByteUID(uid []byte) (byte, error) {
	if len(uid) != 4 {
		return 0, fmt.Errorf("BCC calculation requires an exact 4-byte UID, got %d bytes", len(uid))
	}
	return uid[0] ^ uid[1] ^ uid[2] ^ uid[3], nil
}

// ValidateBCC checks the manufacturer-block BCC only when a four-byte source
// UID is known. A raw file alone cannot reliably distinguish 4-byte and 7-byte
// Classic manufacturer layouts.
func (d Dump) ValidateBCC(sourceUIDLength int) (BCCStatus, error) {
	if sourceUIDLength != 4 {
		return BCCNotApplicable, nil
	}
	if len(d.Blocks) == 0 || d.Blocks[0].KnownMask&0x1f != 0x1f {
		return BCCInvalid, fmt.Errorf("manufacturer block UID and BCC bytes are incomplete")
	}
	b := d.Blocks[0].Data
	want, _ := BCCFor4ByteUID(b[0:4])
	if b[4] != want {
		return BCCInvalid, fmt.Errorf("manufacturer block BCC is %02X, expected %02X", b[4], want)
	}
	return BCCValid, nil
}

// ValidateTrailers rejects malformed complement bits without modifying them.
func (d Dump) ValidateTrailers() error {
	if !d.Complete() {
		return ErrIncompleteDump
	}
	sectors, _ := d.Layout.SectorCount()
	for sector := 0; sector < sectors; sector++ {
		block, _ := d.Layout.TrailerBlock(sector)
		data := d.Blocks[block].Data
		if _, err := DecodeAccessBits([3]byte{data[6], data[7], data[8]}); err != nil {
			return fmt.Errorf("sector %d trailer: %w", sector, err)
		}
	}
	return nil
}

func (b Block) Hex() string { return hex.EncodeToString(b.Data[:]) }
