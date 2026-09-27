package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func fingerprintRequest(t *testing.T) workflow.RestoreRequest {
	t.Helper()
	image, err := mifare.NewDump(mifare.Classic1K)
	if err != nil {
		t.Fatal(err)
	}
	for index := range image.Blocks {
		image.Blocks[index].KnownMask = mifare.AllBytesKnown
	}
	return workflow.RestoreRequest{
		Card: nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 8},
		Dump: image, SourceUIDLength: 4, Blocks: []int{1},
	}
}

func TestRestoreFingerprintBindsDumpCardAndSelection(t *testing.T) {
	request := fingerprintRequest(t)
	baseline, err := restoreFingerprint(request)
	if err != nil {
		t.Fatal(err)
	}

	changedDump := request
	changedDump.Dump = request.Dump.Clone()
	changedDump.Dump.Blocks[1].Data[0] = 1
	changedCard := request
	changedCard.Card.UID = []byte{4, 3, 2, 1}
	changedSelection := request
	changedSelection.Blocks = []int{2}
	for name, candidate := range map[string]workflow.RestoreRequest{
		"dump": changedDump, "card": changedCard, "selection": changedSelection,
	} {
		fingerprint, err := restoreFingerprint(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if fingerprint == baseline {
			t.Fatalf("%s change did not invalidate the preflight fingerprint", name)
		}
	}
}

func TestTrailerWriteRequiresConfirmationAndConsumesToken(t *testing.T) {
	service := &Service{
		preflights: map[string]writePreflightRecord{
			"token": {request: fingerprintRequest(t), fingerprint: "unused", trailers: []int{3}, expires: time.Now().Add(time.Minute)},
		},
	}
	if _, err := service.StartWrite(WriteStartRequestDTO{Token: "token"}); !errors.Is(err, ErrTrailerConfirm) {
		t.Fatalf("StartWrite() error = %v, want ErrTrailerConfirm", err)
	}
	if _, err := service.StartWrite(WriteStartRequestDTO{Token: "token", ConfirmTrailers: true}); !errors.Is(err, ErrPreflightRequired) {
		t.Fatalf("reused token error = %v, want ErrPreflightRequired", err)
	}
}

func TestRestoreUIDWriteRequiresExactConfirmationAndConsumesToken(t *testing.T) {
	uidRequest := workflow.UIDWriteRequest{NewUID: []byte{5, 6, 7, 8}}
	service := &Service{
		preflights: map[string]writePreflightRecord{
			"token": {
				request: fingerprintRequest(t), fingerprint: "unused", uidRequest: &uidRequest,
				uidPlan: &workflow.UIDWritePlan{}, expires: time.Now().Add(time.Minute),
			},
		},
	}
	if _, err := service.StartWrite(WriteStartRequestDTO{Token: "token", ConfirmUID: "05060709"}); !errors.Is(err, ErrUIDConfirm) {
		t.Fatalf("StartWrite() error = %v, want ErrUIDConfirm", err)
	}
	if _, err := service.StartWrite(WriteStartRequestDTO{Token: "token", ConfirmUID: "05060708"}); !errors.Is(err, ErrPreflightRequired) {
		t.Fatalf("reused token error = %v, want ErrPreflightRequired", err)
	}
}

func TestStandaloneUIDWriteRequiresExactConfirmationAndConsumesToken(t *testing.T) {
	service := &Service{
		uidPreflights: map[string]uidPreflightRecord{
			"token": {request: workflow.UIDWriteRequest{NewUID: []byte{5, 6, 7, 8}}, expires: time.Now().Add(time.Minute)},
		},
	}
	if _, err := service.StartUIDWrite(UIDWriteStartRequestDTO{Token: "token", Confirmation: "05-06-07-09"}); !errors.Is(err, ErrUIDConfirm) {
		t.Fatalf("StartUIDWrite() error = %v, want ErrUIDConfirm", err)
	}
	if _, err := service.StartUIDWrite(UIDWriteStartRequestDTO{Token: "token", Confirmation: "05-06-07-08"}); !errors.Is(err, ErrPreflightRequired) {
		t.Fatalf("reused token error = %v, want ErrPreflightRequired", err)
	}
}

func TestParseFourByteUID(t *testing.T) {
	for _, value := range []string{"01020304", "01 02 03 04", "01:02:03:04", "01-02-03-04"} {
		uid, err := parseFourByteUID(value)
		if err != nil || string(uid) != string([]byte{1, 2, 3, 4}) {
			t.Fatalf("parseFourByteUID(%q) = %x, %v", value, uid, err)
		}
	}
	for _, value := range []string{"", "010203", "0102030405", "01020Z04"} {
		if _, err := parseFourByteUID(value); !errors.Is(err, nfc.ErrInvalidArgument) {
			t.Fatalf("parseFourByteUID(%q) error = %v", value, err)
		}
	}
}

func TestPostRestoreUIDAccessMustRemainWritable(t *testing.T) {
	image, err := mifare.NewDump(mifare.Classic1K)
	if err != nil {
		t.Fatal(err)
	}
	setAccess := func(group0, trailer mifare.AccessCondition) {
		bits := mifare.AccessBits{}
		bits.Groups[0], bits.Groups[3] = group0, trailer
		encoded := mifare.EncodeAccessBits(bits)
		copy(image.Blocks[3].Data[6:9], encoded[:])
	}

	setAccess(mifare.AccessCondition{}, mifare.AccessCondition{})
	if err := validatePostRestoreUIDAccess(image); err != nil {
		t.Fatalf("Key A writable transport configuration was rejected: %v", err)
	}
	setAccess(mifare.AccessCondition{C2: true}, mifare.AccessCondition{})
	if err := validatePostRestoreUIDAccess(image); !errors.Is(err, nfc.ErrPermission) {
		t.Fatalf("read-only data group error = %v", err)
	}
	setAccess(mifare.AccessCondition{C1: true}, mifare.AccessCondition{})
	if err := validatePostRestoreUIDAccess(image); !errors.Is(err, nfc.ErrPermission) {
		t.Fatalf("readable Key B configuration error = %v", err)
	}
	setAccess(mifare.AccessCondition{C1: true}, mifare.AccessCondition{C2: true, C3: true})
	if err := validatePostRestoreUIDAccess(image); err != nil {
		t.Fatalf("authenticating Key B configuration was rejected: %v", err)
	}
}

func TestRegisterUIDKeyMigratesCardScopedVerifications(t *testing.T) {
	store := keys.NewStore()
	oldCard := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 8}
	newCard := nfc.CardInfo{UID: []byte{5, 6, 7, 8}, ATQA: oldCard.ATQA, SAK: oldCard.SAK}
	keyA := nfc.Key{1, 2, 3, 4, 5, 6}
	keyB := nfc.Key{6, 5, 4, 3, 2, 1}
	if _, err := store.MergeVerified(keys.CardID(oldCard), 0, nfc.KeyTypeA, keyA, keys.SourceUserInput); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergeVerified(keys.CardID(oldCard), 5, nfc.KeyTypeB, keyB, keys.SourceFileImport); err != nil {
		t.Fatal(err)
	}
	service := &Service{emitter: noopEmitter{}, keyStore: store}
	service.registerUIDKey(workflow.UIDWriteResult{
		OldCard: oldCard, NewCard: newCard, KeyType: nfc.KeyTypeA, Key: keyA, Verified: true,
	})
	matches := store.Verified(keys.CardID(newCard))
	if len(matches) != 2 || matches[0].Sector != 0 || matches[0].KeyType != nfc.KeyTypeA || matches[1].Sector != 5 || matches[1].KeyType != nfc.KeyTypeB {
		t.Fatalf("new-card verifications = %+v", matches)
	}
}

func TestUIDWriteResultDTODoesNotExposeAuthenticationKey(t *testing.T) {
	secret := nfc.Key{0xde, 0xad, 0xbe, 0xef, 0xca, 0xfe}
	payload, err := json.Marshal(uidWriteResultDTO(workflow.UIDWriteResult{
		OldCard: nfc.CardInfo{UID: []byte{1, 2, 3, 4}}, KeyType: nfc.KeyTypeA, Key: secret,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(string(payload)), "DEADBEEFCAFE") {
		t.Fatalf("UID result exposed an authentication key: %s", payload)
	}
}

func TestOperationCancellationReachesWorker(t *testing.T) {
	service := &Service{emitter: noopEmitter{}, operations: make(map[string]*operationTask)}
	started := make(chan struct{})
	stopped := make(chan struct{})
	task, err := service.startOperation("test", "测试任务", func(ctx context.Context, _ string) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := service.CancelTask(task.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("worker did not receive cancellation")
	}
}
