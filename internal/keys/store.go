// Package keys owns MIFARE Classic key parsing, provenance and card-scoped
// verification state.
package keys

import (
	"bufio"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/BennyThink/NFCX/internal/nfc"
)

type Source string

const (
	SourceBuiltin      Source = "builtin"
	SourceUserInput    Source = "user_input"
	SourceFileImport   Source = "file_import"
	SourceReadContext  Source = "read_context"
	SourceAttackEngine Source = "attack_engine"
)

func (s Source) Valid() bool {
	switch s {
	case SourceBuiltin, SourceUserInput, SourceFileImport, SourceReadContext, SourceAttackEngine:
		return true
	default:
		return false
	}
}

type Record struct {
	ID       string
	Value    nfc.Key
	Sources  []Source
	AddedAt  time.Time
	LastUsed time.Time
}

func (r Record) Hex() string {
	return strings.ToUpper(hex.EncodeToString(r.Value[:]))
}

func (r Record) HasSource(source Source) bool {
	for _, candidate := range r.Sources {
		if candidate == source {
			return true
		}
	}
	return false
}

type Verification struct {
	CardID     string
	Sector     int
	KeyType    nfc.KeyType
	KeyID      string
	VerifiedAt time.Time
}

type VerificationMergeStatus string

const (
	VerificationAdded    VerificationMergeStatus = "added"
	VerificationExisting VerificationMergeStatus = "existing"
	VerificationConflict VerificationMergeStatus = "conflict"
)

type VerificationMerge struct {
	Status        VerificationMergeStatus
	Record        Record
	ExistingKeyID string
}

type ImportIssue struct {
	Line    int
	Message string
}

type ParseReport struct {
	Keys       []nfc.Key
	Valid      int
	Duplicates int
	Ignored    int
	Issues     []ImportIssue
}

func ParseDictionary(reader io.Reader) (ParseReport, error) {
	if reader == nil {
		return ParseReport{}, errors.New("dictionary reader is required")
	}
	var report ParseReport
	seen := make(map[nfc.Key]struct{})
	scanner := bufio.NewScanner(reader)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if comment := strings.IndexByte(line, '#'); comment >= 0 {
			line = strings.TrimSpace(line[:comment])
		}
		if line == "" {
			report.Ignored++
			continue
		}
		compact := strings.Map(func(value rune) rune {
			if unicode.IsSpace(value) {
				return -1
			}
			return value
		}, line)
		if len(compact) != nfc.KeySize*2 {
			report.Issues = append(report.Issues, ImportIssue{Line: lineNumber, Message: "密钥必须是 12 位十六进制"})
			continue
		}
		decoded, err := hex.DecodeString(compact)
		if err != nil {
			report.Issues = append(report.Issues, ImportIssue{Line: lineNumber, Message: "密钥包含非十六进制字符"})
			continue
		}
		key, _ := nfc.KeyFromBytes(decoded)
		if _, duplicate := seen[key]; duplicate {
			report.Duplicates++
			continue
		}
		seen[key] = struct{}{}
		report.Keys = append(report.Keys, key)
		report.Valid++
	}
	if err := scanner.Err(); err != nil {
		return ParseReport{}, err
	}
	return report, nil
}

func ParseKey(value string) (nfc.Key, error) {
	report, err := ParseDictionary(strings.NewReader(value))
	if err != nil {
		return nfc.Key{}, err
	}
	if len(report.Issues) != 0 || len(report.Keys) != 1 {
		return nfc.Key{}, errors.New("MIFARE Classic 密钥必须是 12 位十六进制")
	}
	return report.Keys[0], nil
}

//go:embed data/common.dic
var builtinDictionary string

func Builtin() []nfc.Key {
	report, err := ParseDictionary(strings.NewReader(builtinDictionary))
	if err != nil {
		panic(err)
	}
	return append([]nfc.Key(nil), report.Keys...)
}

type Store struct {
	mu            sync.RWMutex
	records       map[string]*Record
	byValue       map[nfc.Key]string
	verifications map[string]Verification
	now           func() time.Time
}

func NewStore() *Store {
	store := &Store{
		records: make(map[string]*Record), byValue: make(map[nfc.Key]string),
		verifications: make(map[string]Verification), now: func() time.Time { return time.Now().UTC() },
	}
	for _, key := range Builtin() {
		store.addLocked(key, SourceBuiltin)
	}
	return store
}

func keyID(value nfc.Key) string {
	digest := sha256.Sum256(value[:])
	return "key-" + hex.EncodeToString(digest[:8])
}

func verificationID(cardID string, sector int, keyType nfc.KeyType) string {
	return fmt.Sprintf("%s/%d/%d", cardID, sector, keyType)
}

// CardID is deliberately based on the complete selection identity used by
// NFCX. Verification remains card-scoped and is never inferred from a key's
// success on an unrelated card.
func CardID(card nfc.CardInfo) string {
	return fmt.Sprintf("%X/%X/%02X", card.UID, card.ATQA, card.SAK)
}

func (s *Store) Add(value nfc.Key, source Source) (Record, bool, error) {
	if s == nil || !source.Valid() {
		return Record{}, false, errors.New("valid key source is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addLocked(value, source)
}

func (s *Store) addLocked(value nfc.Key, source Source) (Record, bool, error) {
	if id, exists := s.byValue[value]; exists {
		record := s.records[id]
		if !record.HasSource(source) {
			record.Sources = append(record.Sources, source)
			sort.SliceStable(record.Sources, func(i, j int) bool { return sourceRank(record.Sources[i]) < sourceRank(record.Sources[j]) })
		}
		return cloneRecord(*record), false, nil
	}
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now()
	}
	record := &Record{ID: keyID(value), Value: value, Sources: []Source{source}, AddedAt: now}
	s.records[record.ID] = record
	s.byValue[value] = record.ID
	return cloneRecord(*record), true, nil
}

func (s *Store) AddHex(value string, source Source) (Record, bool, error) {
	key, err := ParseKey(value)
	if err != nil {
		return Record{}, false, err
	}
	return s.Add(key, source)
}

func (s *Store) Import(reader io.Reader, source Source) (ParseReport, int, error) {
	report, err := ParseDictionary(reader)
	if err != nil {
		return ParseReport{}, 0, err
	}
	added := 0
	for _, key := range report.Keys {
		_, created, err := s.Add(key, source)
		if err != nil {
			return ParseReport{}, added, err
		}
		if created {
			added++
		} else {
			report.Duplicates++
		}
	}
	return report, added, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.records[id]
	if !exists {
		return fmt.Errorf("key %q was not found", id)
	}
	kept := record.Sources[:0]
	for _, source := range record.Sources {
		if source == SourceBuiltin {
			kept = append(kept, source)
		}
	}
	if len(kept) != 0 {
		record.Sources = append([]Source(nil), kept...)
		return nil
	}
	delete(s.records, id)
	delete(s.byValue, record.Value)
	for matchID, match := range s.verifications {
		if match.KeyID == id {
			delete(s.verifications, matchID)
		}
	}
	return nil
}

func (s *Store) Entries() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Record, 0, len(s.records))
	for _, record := range s.records {
		result = append(result, cloneRecord(*record))
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := recordRank(result[i]), recordRank(result[j])
		if left != right {
			return left < right
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func cloneRecord(value Record) Record {
	value.Sources = append([]Source(nil), value.Sources...)
	return value
}

func sourceRank(source Source) int {
	switch source {
	case SourceReadContext:
		return 0
	case SourceUserInput:
		return 1
	case SourceFileImport:
		return 2
	case SourceAttackEngine:
		return 3
	case SourceBuiltin:
		return 4
	default:
		return 5
	}
}

func recordRank(record Record) int {
	rank := 99
	for _, source := range record.Sources {
		if candidate := sourceRank(source); candidate < rank {
			rank = candidate
		}
	}
	return rank
}

func (s *Store) Verify(cardID string, sector int, keyType nfc.KeyType, keyID string) error {
	if cardID == "" || sector < 0 || !keyType.Valid() {
		return errors.New("card, sector and key type are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.records[keyID]
	if !exists {
		return fmt.Errorf("key %q was not found", keyID)
	}
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now()
	}
	record.LastUsed = now
	s.verifications[verificationID(cardID, sector, keyType)] = Verification{
		CardID: cardID, Sector: sector, KeyType: keyType, KeyID: keyID, VerifiedAt: now,
	}
	return nil
}

// MergeVerified atomically imports a freshly authenticated key without
// replacing a different verification already stored for the same card slot.
// A conflicting candidate is not added to the global key catalog.
func (s *Store) MergeVerified(cardID string, sector int, keyType nfc.KeyType, value nfc.Key, source Source) (VerificationMerge, error) {
	if s == nil || cardID == "" || sector < 0 || !keyType.Valid() || !source.Valid() {
		return VerificationMerge{}, errors.New("card, sector, key type and source are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	verificationKey := verificationID(cardID, sector, keyType)
	if existing, ok := s.verifications[verificationKey]; ok {
		record, present := s.records[existing.KeyID]
		if !present {
			return VerificationMerge{}, fmt.Errorf("verified key %q was not found", existing.KeyID)
		}
		if record.Value != value {
			return VerificationMerge{Status: VerificationConflict, ExistingKeyID: existing.KeyID}, nil
		}
		if !record.HasSource(source) {
			record.Sources = append(record.Sources, source)
			sort.SliceStable(record.Sources, func(i, j int) bool { return sourceRank(record.Sources[i]) < sourceRank(record.Sources[j]) })
		}
		now := time.Now().UTC()
		if s.now != nil {
			now = s.now()
		}
		record.LastUsed = now
		existing.VerifiedAt = now
		s.verifications[verificationKey] = existing
		return VerificationMerge{Status: VerificationExisting, Record: cloneRecord(*record), ExistingKeyID: existing.KeyID}, nil
	}
	record, _, err := s.addLocked(value, source)
	if err != nil {
		return VerificationMerge{}, err
	}
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now()
	}
	stored := s.records[record.ID]
	stored.LastUsed = now
	s.verifications[verificationKey] = Verification{CardID: cardID, Sector: sector, KeyType: keyType, KeyID: record.ID, VerifiedAt: now}
	return VerificationMerge{Status: VerificationAdded, Record: cloneRecord(*stored)}, nil
}

func (s *Store) Verified(cardID string) []Verification {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Verification, 0)
	for _, match := range s.verifications {
		if match.CardID == cardID {
			result = append(result, match)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Sector != result[j].Sector {
			return result[i].Sector < result[j].Sector
		}
		return result[i].KeyType < result[j].KeyType
	})
	return result
}

// Candidates returns the deterministic priority order for one authentication
// slot: its exact prior match, other successes on this card, then user and
// imported entries, and finally the built-in dictionary.
func (s *Store) Candidates(cardID string, sector int, keyType nfc.KeyType) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]Record, 0, len(s.records))
	for _, record := range s.records {
		entries = append(entries, cloneRecord(*record))
	}
	exact := ""
	if match, ok := s.verifications[verificationID(cardID, sector, keyType)]; ok {
		exact = match.KeyID
	}
	succeeded := make(map[string]time.Time)
	for _, match := range s.verifications {
		if match.CardID == cardID {
			if previous, ok := succeeded[match.KeyID]; !ok || match.VerifiedAt.After(previous) {
				succeeded[match.KeyID] = match.VerifiedAt
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := candidateRank(entries[i], exact, succeeded), candidateRank(entries[j], exact, succeeded)
		if left != right {
			return left < right
		}
		leftTime, leftOK := succeeded[entries[i].ID]
		rightTime, rightOK := succeeded[entries[j].ID]
		if leftOK && rightOK && !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return entries[i].ID < entries[j].ID
	})
	return entries
}

func candidateRank(record Record, exact string, succeeded map[string]time.Time) int {
	if record.ID == exact {
		return -2
	}
	if _, ok := succeeded[record.ID]; ok {
		return -1
	}
	return recordRank(record)
}

func (s *Store) Record(id string) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, exists := s.records[id]
	if !exists {
		return Record{}, false
	}
	return cloneRecord(*record), true
}

type persistedStore struct {
	Version int            `json:"version"`
	Keys    []persistedKey `json:"keys"`
}

type persistedKey struct {
	Value   string   `json:"value"`
	Sources []Source `json:"sources"`
}

func (s *Store) Load(path string) error {
	encoded, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var stored persistedStore
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return err
	}
	if stored.Version != 1 {
		return fmt.Errorf("unsupported key store version %d", stored.Version)
	}
	for _, item := range stored.Keys {
		key, err := ParseKey(item.Value)
		if err != nil {
			return err
		}
		for _, source := range item.Sources {
			if source == SourceBuiltin || !source.Valid() {
				continue
			}
			if _, _, err := s.Add(key, source); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) Save(path string) error {
	stored := persistedStore{Version: 1}
	for _, record := range s.Entries() {
		item := persistedKey{Value: strings.ToUpper(hex.EncodeToString(record.Value[:]))}
		for _, source := range record.Sources {
			if source != SourceBuiltin {
				item.Sources = append(item.Sources, source)
			}
		}
		if len(item.Sources) != 0 {
			stored.Keys = append(stored.Keys, item)
		}
	}
	encoded, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".nfcx-keys-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(encoded, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func (s *Store) Export(writer io.Writer) (int, error) {
	if writer == nil {
		return 0, errors.New("export writer is required")
	}
	count := 0
	for _, record := range s.Entries() {
		if _, err := fmt.Fprintln(writer, record.Hex()); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
