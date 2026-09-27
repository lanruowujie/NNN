package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/BennyThink/NFCX/internal/nfc"
)

func TestDashboardDTOJSONSerialization(t *testing.T) {
	service := NewService(nil)
	payload, err := json.Marshal(service.Dashboard())
	if err != nil {
		t.Fatalf("marshal dashboard: %v", err)
	}

	jsonText := string(payload)
	for _, field := range []string{`"selectedDevice"`, `"connection"`, `"present"`, `"uidLength"`, `"actions"`} {
		if !strings.Contains(jsonText, field) {
			t.Errorf("serialized dashboard is missing %s", field)
		}
	}
	if strings.Contains(jsonText, "FFFFFFFFFFFF") {
		t.Fatal("serialized dashboard must not expose complete key material")
	}

	var decoded DashboardDTO
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal dashboard: %v", err)
	}
	if len(decoded.Devices) != 0 || decoded.Card.Present || decoded.Connection.Status != "disconnected" {
		t.Fatalf("unexpected decoded dashboard: %+v", decoded)
	}
}

func TestCardDTOLabelsInferenceAsNonAuthoritative(t *testing.T) {
	dto := cardDTO(nfc.CardInfo{UID: []byte{0x04, 0xa1, 0xb2, 0xc3}, ATQA: [2]byte{0x00, 0x04}, SAK: 0x08})
	if dto.UID != "04 A1 B2 C3" || dto.UIDLength != 4 || dto.Type != "MIFARE Classic 1K" || !dto.Inferred {
		t.Fatalf("unexpected card DTO: %+v", dto)
	}
	unknown := cardDTO(nfc.CardInfo{UID: []byte{1, 2, 3, 4}, SAK: 0x20})
	if unknown.Type != "未知" || unknown.Inferred {
		t.Fatalf("unknown card was overstated: %+v", unknown)
	}
}

func TestClassicErrorCodeNames(t *testing.T) {
	for _, test := range []struct {
		code nfc.ErrorCode
		want string
	}{
		{nfc.CodeAuthenticationFailed, "authentication_failed"},
		{nfc.CodeCardChanged, "card_changed"},
		{nfc.CodeNotAuthenticated, "not_authenticated"},
		{nfc.CodeVerificationFailed, "verification_failed"},
	} {
		if got := errorCodeName(test.code); got != test.want {
			t.Errorf("errorCodeName(%d) = %q; want %q", test.code, got, test.want)
		}
	}
}

func TestExternalTaskEventDTOCarriesStructuredDiagnostics(t *testing.T) {
	exitCode := 17
	payload, err := json.Marshal(TaskEventDetailDTO{
		TaskID: "external-1", Kind: "external_fixture", Type: TaskEventProgress,
		Phase: "running", Engine: "nfcx-fake-engine", Version: "1.0",
		Stream: "stderr", Log: "raw output", Sequence: 9, ExitCode: &exitCode,
		RecoveryError: "reopen failed", Message: "diagnostic",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, field := range []string{`"engine":"nfcx-fake-engine"`, `"stream":"stderr"`, `"log":"raw output"`, `"sequence":9`, `"exitCode":17`, `"recoveryError":"reopen failed"`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("external task DTO is missing %s: %s", field, encoded)
		}
	}
}

func TestMFoCResultDTOContainsCountsButNoKeyMaterial(t *testing.T) {
	payload, err := json.Marshal(TaskEventDetailDTO{
		TaskID: "mfoc-1", Kind: "mfoc", Type: TaskEventProgress,
		MFoC: &MFoCResultDTO{OutputValidated: true, Claimed: 32, Added: 20, Existing: 10, Rejected: 1, Conflicts: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, field := range []string{`"outputValidated":true`, `"claimed":32`, `"added":20`, `"rejected":1`, `"conflicts":1`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("mfoc DTO is missing %s: %s", field, encoded)
		}
	}
	if strings.Contains(strings.ToLower(encoded), "keya") || strings.Contains(strings.ToLower(encoded), "keyb") {
		t.Fatalf("mfoc result leaked key fields: %s", encoded)
	}
}

func TestMFCUKResultDTOContainsStageSummaryButNoKeyMaterial(t *testing.T) {
	payload, err := json.Marshal(TaskEventDetailDTO{
		TaskID: "mfcuk-1", Kind: "mfcuk_pipeline", Type: TaskEventProgress,
		MFCUK: &MFCUKResultDTO{
			CandidateOutputValidated: true, Candidates: 2, Verified: 1, Rejected: 1,
			MFCUKVersion: "0.3.8", MFCUKDurationMillis: 1200, MFoCOutputValidated: true,
			MFoCVersion: "0.10.7", MFoCDurationMillis: 2300, MFoCAdded: 30,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, field := range []string{`"candidates":2`, `"verified":1`, `"rejected":1`, `"mfcukVersion":"0.3.8"`, `"mfocVersion":"0.10.7"`, `"mfocAdded":30`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("mfcuk DTO is missing %s: %s", field, encoded)
		}
	}
	if strings.Contains(strings.ToLower(encoded), "value") || strings.Contains(strings.ToLower(encoded), "keya") || strings.Contains(strings.ToLower(encoded), "keyb") {
		t.Fatalf("mfcuk result leaked key fields: %s", encoded)
	}
}

func TestPN532DeviceDTOAdvertisesSeparateAttackCapabilities(t *testing.T) {
	dto := deviceDTO(nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"})
	if !dto.Nested || !dto.Darkside || !dto.Hardnested {
		t.Fatalf("device DTO=%+v", dto)
	}
}

func TestKeyRecoveryTaskDTOCarriesFiveStepReportWithoutKeyMaterial(t *testing.T) {
	payload, err := json.Marshal(TaskEventDetailDTO{
		TaskID: "key-recovery-1", Kind: "key_recovery", Type: TaskEventProgress,
		Step: 3, TotalSteps: 5, StepStatus: "running", Indeterminate: true,
		Recovery: &KeyRecoveryResultDTO{
			Outcome: "partial", VerifiedKeySlots: 1, CoveredSectors: 1, ReadableSectors: 1, TotalSectors: 16,
			Steps: []KeyRecoveryStepDTO{{Number: 1, Stage: "common_keys", Label: "扫描常见密钥", Status: "completed"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, field := range []string{`"kind":"key_recovery"`, `"step":3`, `"totalSteps":5`, `"stepStatus":"running"`, `"indeterminate":true`, `"outcome":"partial"`, `"readableSectors":1`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("key recovery DTO is missing %s: %s", field, encoded)
		}
	}
	if strings.Contains(strings.ToLower(encoded), "ffffffffffff") || strings.Contains(strings.ToLower(encoded), `"keya"`) || strings.Contains(strings.ToLower(encoded), `"keyb"`) {
		t.Fatalf("key recovery report leaked key material: %s", encoded)
	}
}
