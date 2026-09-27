package workflow

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
)

const DumpMetadataVersion = 1

type DumpMetadata struct {
	Version    int                     `json:"version"`
	Complete   bool                    `json:"complete"`
	Layout     string                  `json:"layout"`
	Card       DumpCardMetadata        `json:"card"`
	Device     nfc.DeviceInfo          `json:"device"`
	StartedAt  time.Time               `json:"startedAt"`
	FinishedAt time.Time               `json:"finishedAt"`
	Blocks     []DumpBlockMetadata     `json:"blocks"`
	Keys       []DumpSectorKeyMetadata `json:"keys"`
}

type DumpCardMetadata struct {
	UID  string `json:"uid,omitempty"`
	ATQA string `json:"atqa,omitempty"`
	SAK  byte   `json:"sak"`
}

type DumpBlockMetadata struct {
	Block      int                `json:"block"`
	Sector     int                `json:"sector"`
	Data       string             `json:"data"`
	KnownMask  string             `json:"knownMask"`
	Status     mifare.BlockStatus `json:"status"`
	ErrorCode  string             `json:"errorCode,omitempty"`
	Error      string             `json:"error,omitempty"`
	KeyASource string             `json:"keyASource,omitempty"`
	KeyBSource string             `json:"keyBSource,omitempty"`
}

type DumpSectorKeyMetadata struct {
	Sector int    `json:"sector"`
	KeyA   string `json:"keyA,omitempty"`
	KeyB   string `json:"keyB,omitempty"`
}

func MetadataFromDump(result DumpResult) DumpMetadata {
	metadata := DumpMetadata{
		Version:    DumpMetadataVersion,
		Complete:   result.Complete(),
		Layout:     layoutName(result.Dump.Layout),
		Card:       DumpCardMetadata{UID: strings.ToUpper(hex.EncodeToString(result.Card.UID)), ATQA: strings.ToUpper(hex.EncodeToString(result.Card.ATQA[:])), SAK: result.Card.SAK},
		Device:     result.Device,
		StartedAt:  result.StartedAt,
		FinishedAt: result.FinishedAt,
		Blocks:     make([]DumpBlockMetadata, len(result.Dump.Blocks)),
		Keys:       make([]DumpSectorKeyMetadata, len(result.Keys)),
	}
	for index, block := range result.Dump.Blocks {
		metadata.Blocks[index] = DumpBlockMetadata{
			Block: block.Number, Sector: block.Sector, Data: strings.ToUpper(block.Hex()),
			KnownMask: fmt.Sprintf("%04X", block.KnownMask), Status: block.Status,
			ErrorCode: block.ErrorCode, Error: block.Error, KeyASource: block.KeyASource, KeyBSource: block.KeyBSource,
		}
	}
	for index, keys := range result.Keys {
		entry := DumpSectorKeyMetadata{Sector: keys.Sector}
		if keys.KeyA != nil {
			entry.KeyA = strings.ToUpper(hex.EncodeToString(keys.KeyA[:]))
		}
		if keys.KeyB != nil {
			entry.KeyB = strings.ToUpper(hex.EncodeToString(keys.KeyB[:]))
		}
		metadata.Keys[index] = entry
	}
	return metadata
}

func DumpFromMetadata(metadata DumpMetadata) (DumpResult, error) {
	if metadata.Version != DumpMetadataVersion {
		return DumpResult{}, fmt.Errorf("unsupported NFCX dump metadata version %d", metadata.Version)
	}
	layout, err := parseLayoutName(metadata.Layout)
	if err != nil {
		return DumpResult{}, err
	}
	image, _ := mifare.NewDump(layout)
	if len(metadata.Blocks) != len(image.Blocks) {
		return DumpResult{}, fmt.Errorf("metadata contains %d blocks, expected %d", len(metadata.Blocks), len(image.Blocks))
	}
	for index, stored := range metadata.Blocks {
		if stored.Block != index || stored.Sector != image.Blocks[index].Sector {
			return DumpResult{}, fmt.Errorf("metadata block %d has inconsistent address", index)
		}
		data, err := hex.DecodeString(stored.Data)
		if err != nil || len(data) != nfc.BlockSize {
			return DumpResult{}, fmt.Errorf("metadata block %d data must contain 16 bytes", index)
		}
		var mask uint16
		if len(stored.KnownMask) != 4 {
			return DumpResult{}, fmt.Errorf("metadata block %d has invalid known mask", index)
		}
		if _, err := fmt.Sscanf(stored.KnownMask, "%04X", &mask); err != nil {
			return DumpResult{}, fmt.Errorf("metadata block %d has invalid known mask", index)
		}
		if !validBlockStatus(stored.Status) {
			return DumpResult{}, fmt.Errorf("metadata block %d has invalid status %q", index, stored.Status)
		}
		copy(image.Blocks[index].Data[:], data)
		image.Blocks[index].KnownMask = mask
		image.Blocks[index].Status = stored.Status
		image.Blocks[index].ErrorCode = stored.ErrorCode
		image.Blocks[index].Error = stored.Error
		image.Blocks[index].KeyASource = stored.KeyASource
		image.Blocks[index].KeyBSource = stored.KeyBSource
	}
	if metadata.Complete != image.Complete() {
		return DumpResult{}, fmt.Errorf("metadata completeness flag does not match block certainty")
	}
	uid, err := decodeOptionalHex(metadata.Card.UID, []int{0, 4, 7, 10}, "card UID")
	if err != nil {
		return DumpResult{}, err
	}
	atqa, err := decodeOptionalHex(metadata.Card.ATQA, []int{0, nfc.ATQALength}, "card ATQA")
	if err != nil {
		return DumpResult{}, err
	}
	card := nfc.CardInfo{UID: uid, SAK: metadata.Card.SAK}
	copy(card.ATQA[:], atqa)
	result := DumpResult{Dump: image, Card: card, Device: metadata.Device, StartedAt: metadata.StartedAt, FinishedAt: metadata.FinishedAt}
	result.Keys = make([]VerifiedSectorKeys, len(metadata.Keys))
	seen := make(map[int]struct{}, len(metadata.Keys))
	sectors, _ := layout.SectorCount()
	for index, stored := range metadata.Keys {
		if stored.Sector < 0 || stored.Sector >= sectors {
			return DumpResult{}, fmt.Errorf("metadata key sector %d is outside layout", stored.Sector)
		}
		if _, duplicate := seen[stored.Sector]; duplicate {
			return DumpResult{}, fmt.Errorf("metadata has duplicate keys for sector %d", stored.Sector)
		}
		seen[stored.Sector] = struct{}{}
		result.Keys[index] = VerifiedSectorKeys{Sector: stored.Sector}
		if stored.KeyA != "" {
			key, err := decodeKey(stored.KeyA)
			if err != nil {
				return DumpResult{}, fmt.Errorf("metadata sector %d Key A: %w", stored.Sector, err)
			}
			result.Keys[index].KeyA = &key
		}
		if stored.KeyB != "" {
			key, err := decodeKey(stored.KeyB)
			if err != nil {
				return DumpResult{}, fmt.Errorf("metadata sector %d Key B: %w", stored.Sector, err)
			}
			result.Keys[index].KeyB = &key
		}
	}
	return result, nil
}

// SaveRawDump writes a standards-compatible raw file and a sibling
// <name>.nfcx.json metadata file. Partial images are rejected.
func SaveRawDump(path string, result DumpResult) (string, error) {
	raw, err := result.Dump.Raw()
	if err != nil {
		return "", err
	}
	metadataPath := path + ".nfcx.json"
	encoded, err := json.MarshalIndent(MetadataFromDump(result), "", "  ")
	if err != nil {
		return "", err
	}
	encoded = append(encoded, '\n')
	if err := atomicWriteFile(path, raw); err != nil {
		return "", err
	}
	if err := atomicWriteFile(metadataPath, encoded); err != nil {
		return "", err
	}
	return metadataPath, nil
}

// SaveDumpProject persists complete or partial state without manufacturing a
// raw image for unknown bytes.
func SaveDumpProject(path string, result DumpResult) error {
	encoded, err := json.MarshalIndent(MetadataFromDump(result), "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, append(encoded, '\n'))
}

func LoadDumpProject(path string) (DumpResult, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return DumpResult{}, err
	}
	var metadata DumpMetadata
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return DumpResult{}, err
	}
	return DumpFromMetadata(metadata)
}

// LoadRawDump accepts .bin and .mfd bytes without requiring NFCX metadata. If
// a sidecar exists, it is validated against the raw file before being trusted.
func LoadRawDump(path string) (DumpResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return DumpResult{}, err
	}
	image, err := mifare.ParseRaw(raw)
	if err != nil {
		return DumpResult{}, err
	}
	metadataPath := path + ".nfcx.json"
	result, err := LoadDumpProject(metadataPath)
	if os.IsNotExist(err) {
		if err := image.ValidateTrailers(); err != nil {
			return DumpResult{}, err
		}
		return DumpResult{Dump: image}, nil
	}
	if err != nil {
		return DumpResult{}, err
	}
	stored, err := result.Dump.Raw()
	if err != nil {
		return DumpResult{}, err
	}
	if !equalBytes(stored, raw) {
		return DumpResult{}, fmt.Errorf("raw dump does not match its NFCX metadata sidecar")
	}
	if err := result.Dump.ValidateTrailers(); err != nil {
		return DumpResult{}, err
	}
	if _, err := result.Dump.ValidateBCC(len(result.Card.UID)); err != nil {
		return DumpResult{}, err
	}
	return result, nil
}

func atomicWriteFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".nfcx-write-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func layoutName(layout mifare.Layout) string {
	switch layout {
	case mifare.Classic1K:
		return "classic_1k"
	case mifare.Classic4K:
		return "classic_4k"
	default:
		return "unknown"
	}
}

func parseLayoutName(name string) (mifare.Layout, error) {
	switch name {
	case "classic_1k":
		return mifare.Classic1K, nil
	case "classic_4k":
		return mifare.Classic4K, nil
	default:
		return 0, fmt.Errorf("unsupported dump layout %q", name)
	}
}

func validBlockStatus(status mifare.BlockStatus) bool {
	switch status {
	case mifare.BlockUnread, mifare.BlockRead, mifare.BlockAuthFailed, mifare.BlockReadFailed, mifare.BlockSynthetic, mifare.BlockVerified:
		return true
	default:
		return false
	}
}

func decodeOptionalHex(value string, lengths []int, name string) ([]byte, error) {
	data, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%s is not hexadecimal", name)
	}
	for _, length := range lengths {
		if len(data) == length {
			return data, nil
		}
	}
	return nil, fmt.Errorf("%s has unsupported length %d", name, len(data))
}

func decodeKey(value string) (nfc.Key, error) {
	data, err := hex.DecodeString(value)
	if err != nil {
		return nfc.Key{}, err
	}
	return nfc.KeyFromBytes(data)
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
