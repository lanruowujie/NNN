package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/workbench"
	"github.com/BennyThink/NFCX/internal/workflow"
)

type fixedDialogs struct {
	open string
	save string
}

func (d fixedDialogs) OpenFile(string, string) (string, error) { return d.open, nil }
func (d fixedDialogs) SaveFile(string, string, string) (string, error) {
	return d.save, nil
}

func TestKeyServicePersistsAndDedicatedKeyDTOShowsFullValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "keys.json")
	service := &Service{emitter: noopEmitter{}, keyStore: keys.NewStore(), keyStorePath: path}
	const customKey = "9A8B7C6D5E4F"
	dtos, err := service.AddKey(customKey)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(dtos)
	if !strings.Contains(string(payload), `"value":"`+customKey+`"`) || strings.Contains(string(payload), "••••") {
		t.Fatalf("dedicated key DTO did not show the full key: %s", payload)
	}
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 8}
	customID := ""
	for _, dto := range dtos {
		if dto.Value == customKey {
			customID = dto.ID
			break
		}
	}
	if err := service.keyStore.Verify(keys.CardID(card), 0, nfc.KeyTypeA, customID); err != nil {
		t.Fatal(err)
	}
	sectorPayload, _ := json.Marshal(sectorKeyDTOs(service.keyStore, &card))
	if !strings.Contains(string(sectorPayload), `"keyA":"`+customKey+`"`) || strings.Contains(string(sectorPayload), "••••") {
		t.Fatalf("sector key DTO did not show the full key: %s", sectorPayload)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("key store permissions = %o", info.Mode().Perm())
	}
	reloaded := keys.NewStore()
	if err := reloaded.Load(path); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range reloaded.Entries() {
		found = found || entry.Hex() == customKey
	}
	if !found {
		t.Fatal("persisted user key was not reloaded")
	}
}

func TestImportReportsInvalidAndDuplicateLinesAndExportIsExplicit(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "input.dic")
	output := filepath.Join(directory, "output.dic")
	const importedKey = "9A8B7C6D5E4F"
	if err := os.WriteFile(input, []byte(importedKey+"\n9A 8B 7C 6D 5E 4F\ninvalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		emitter: noopEmitter{}, keyStore: keys.NewStore(), keyStorePath: filepath.Join(directory, "keys.json"),
		dialogs: fixedDialogs{open: input, save: output},
	}
	report, err := service.ImportKeyDictionary()
	if err != nil {
		t.Fatal(err)
	}
	if report.Added != 1 || report.Duplicates != 1 || len(report.Issues) != 1 || report.Issues[0].Line != 3 {
		t.Fatalf("import report = %+v", report)
	}
	exportResult, err := service.ExportKeyDictionary()
	if err != nil {
		t.Fatal(err)
	}
	if exportResult.Count != len(service.keyStore.Entries()) || exportResult.Count == 0 {
		t.Fatalf("export count = %d, catalog size = %d", exportResult.Count, len(service.keyStore.Entries()))
	}
	exported, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(exported), importedKey+"\n") || !strings.Contains(string(exported), "FFFFFFFFFFFF\n") {
		t.Fatalf("exported dictionary omitted custom or built-in keys: %q", exported)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("exported dictionary permissions = %o", info.Mode().Perm())
	}
}

func TestLoadDumpRestoresCardScopedVerifiedKeys(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "card.nfcx.json")
	image, err := mifare.NewDump(mifare.Classic1K)
	if err != nil {
		t.Fatal(err)
	}
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 8}
	key := nfc.Key{1, 2, 3, 4, 5, 6}
	result := workflow.DumpResult{
		Dump: image, Card: card,
		Keys: []workflow.VerifiedSectorKeys{{Sector: 0, KeyA: &key}},
	}
	if err := workflow.SaveDumpProject(path, result); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		emitter: noopEmitter{}, keyStore: keys.NewStore(), bench: workbench.New(),
		dialogs: fixedDialogs{open: path},
	}
	if _, err := service.LoadDump(false); err != nil {
		t.Fatal(err)
	}
	matches := service.keyStore.Verified(keys.CardID(card))
	if len(matches) != 1 || matches[0].Sector != 0 || matches[0].KeyType != nfc.KeyTypeA {
		t.Fatalf("verified keys after load = %+v", matches)
	}
}
