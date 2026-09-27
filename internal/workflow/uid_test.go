package workflow_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
	"github.com/BennyThink/NFCX/internal/workflow"
)

type recordingUIDBackups struct {
	order  *[]string
	err    error
	backup workflow.UIDBackup
}

func (s *recordingUIDBackups) Save(_ context.Context, backup workflow.UIDBackup) (string, error) {
	if s.order != nil {
		*s.order = append(*s.order, "backup")
	}
	s.backup = backup
	if s.err != nil {
		return "", s.err
	}
	return "/safe/uid-backup.json", nil
}

func uidFixtureReader(t *testing.T, order *[]string, changeUID bool, writeErr error, afterWrite func()) (*mock.Reader, nfc.CardInfo, *[nfc.BlockSize]byte) {
	t.Helper()
	card := classic1KCard(1)
	block0 := &[nfc.BlockSize]byte{1, 2, 3, 4, 4, 0x08, 0x04, 0x00, 0x62, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69}
	trailer := [nfc.BlockSize]byte{6: 0xff, 7: 0x07, 8: 0x80, 9: 0x69}
	reader := openMockReader(t, mock.ReaderFuncs{
		CardInfo:     func(context.Context) (nfc.CardInfo, error) { return card.Clone(), nil },
		Authenticate: func(context.Context, byte, nfc.KeyType, nfc.Key) error { return nil },
		ReadBlock: func(_ context.Context, block byte) ([nfc.BlockSize]byte, error) {
			if block == 3 {
				return trailer, nil
			}
			return *block0, nil
		},
		WriteManufacturer: func(_ context.Context, data [nfc.BlockSize]byte) error {
			*order = append(*order, "write")
			if changeUID {
				*block0 = data
				card.UID = append([]byte(nil), data[0:4]...)
			}
			if afterWrite != nil {
				afterWrite()
			}
			return writeErr
		},
	})
	return reader, card, block0
}

func TestUIDWriteBacksUpPreservesManufacturerBytesAndVerifies(t *testing.T) {
	var order []string
	reader, card, oldBlock := uidFixtureReader(t, &order, true, nil, nil)
	original := *oldBlock
	backups := &recordingUIDBackups{order: &order}
	service := workflow.NewUIDService(directExecutor{reader: reader}, backups)
	key := nfc.Key{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	request := workflow.UIDWriteRequest{
		TaskID: "uid-1", Card: card, Device: nfc.DeviceInfo{ConnString: "mock:test"}, NewUID: []byte{5, 6, 7, 8},
		Sector0Keys: workflow.SectorKeys{Sector: 0, KeyA: &key},
	}
	result, err := service.Write(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "backup,write" {
		t.Fatalf("operation order = %v", order)
	}
	if !result.Verified || result.BackupPath == "" || result.NewBlock0[4] != 0x0c {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.NewBlock0 != *oldBlock || result.OldBlock0 != original {
		t.Fatalf("block results old=%x new=%x actual=%x", result.OldBlock0, result.NewBlock0, *oldBlock)
	}
	if result.NewBlock0[5] != original[5] || result.NewBlock0[15] != original[15] {
		t.Fatal("manufacturer bytes 5..15 were changed")
	}
	if backups.backup.OldBlock0 != original || backups.backup.Method != "special_uid_auto" {
		t.Fatalf("unexpected backup: %+v", backups.backup)
	}
}

func TestUIDWriteVerifiesIndeterminateWriteResponse(t *testing.T) {
	var order []string
	responseErr := nfc.NewError("write manufacturer block", nfc.CodeIO, "RF Transmission Error", nil)
	reader, card, _ := uidFixtureReader(t, &order, true, responseErr, nil)
	service := workflow.NewUIDService(directExecutor{reader: reader}, &recordingUIDBackups{order: &order})
	key := nfc.Key{}
	result, err := service.Write(context.Background(), workflow.UIDWriteRequest{
		Card: card, NewUID: []byte{5, 6, 7, 8}, Sector0Keys: workflow.SectorKeys{Sector: 0, KeyA: &key},
	}, nil)
	if err != nil || !result.Verified {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestUIDWriteRejectsIndeterminateResponseWhenUIDDidNotChange(t *testing.T) {
	var order []string
	responseErr := nfc.NewError("write manufacturer block", nfc.CodeIO, "RF Transmission Error", nil)
	reader, card, _ := uidFixtureReader(t, &order, false, responseErr, nil)
	service := workflow.NewUIDService(directExecutor{reader: reader}, &recordingUIDBackups{order: &order})
	key := nfc.Key{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result, err := service.Write(ctx, workflow.UIDWriteRequest{
		Card: card, NewUID: []byte{5, 6, 7, 8}, Sector0Keys: workflow.SectorKeys{Sector: 0, KeyA: &key},
	}, nil)
	if !errors.Is(err, nfc.ErrVerificationFailed) || result.Verified {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestUIDWriteRejectsUnsupportedUIDLengthsBeforeHardware(t *testing.T) {
	service := workflow.NewUIDService(nil, &recordingUIDBackups{})
	for _, uid := range [][]byte{make([]byte, 7), make([]byte, 10)} {
		_, err := service.Write(context.Background(), workflow.UIDWriteRequest{
			Card: classic1KCard(1), NewUID: uid, Sector0Keys: workflow.SectorKeys{Sector: 0},
		}, nil)
		if !errors.Is(err, nfc.ErrInvalidArgument) {
			t.Fatalf("UID length %d error = %v", len(uid), err)
		}
	}
}

func TestUIDWriteDoesNotMutateWhenBackupFails(t *testing.T) {
	var order []string
	reader, card, _ := uidFixtureReader(t, &order, true, nil, nil)
	service := workflow.NewUIDService(directExecutor{reader: reader}, &recordingUIDBackups{order: &order, err: errors.New("disk full")})
	key := nfc.Key{}
	_, err := service.Write(context.Background(), workflow.UIDWriteRequest{
		Card: card, NewUID: []byte{5, 6, 7, 8}, Sector0Keys: workflow.SectorKeys{Sector: 0, KeyA: &key},
	}, nil)
	if !errors.Is(err, nfc.ErrIO) || strings.Join(order, ",") != "backup" {
		t.Fatalf("error=%v order=%v", err, order)
	}
}

func TestUIDWriteTreatsAcceptedCommandWithoutNewUIDAsFailure(t *testing.T) {
	var order []string
	reader, card, _ := uidFixtureReader(t, &order, false, nil, nil)
	service := workflow.NewUIDService(directExecutor{reader: reader}, &recordingUIDBackups{order: &order})
	key := nfc.Key{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result, err := service.Write(ctx, workflow.UIDWriteRequest{
		Card: card, NewUID: []byte{5, 6, 7, 8}, Sector0Keys: workflow.SectorKeys{Sector: 0, KeyA: &key},
	}, nil)
	if !errors.Is(err, nfc.ErrTimeout) || result.Verified || result.BackupPath == "" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestUIDWriteCancellationDuringReselectRemainsCancellation(t *testing.T) {
	var order []string
	ctx, cancel := context.WithCancel(context.Background())
	reader, card, _ := uidFixtureReader(t, &order, false, nil, cancel)
	service := workflow.NewUIDService(directExecutor{reader: reader}, &recordingUIDBackups{order: &order})
	key := nfc.Key{}
	result, err := service.Write(ctx, workflow.UIDWriteRequest{
		Card: card, NewUID: []byte{5, 6, 7, 8}, Sector0Keys: workflow.SectorKeys{Sector: 0, KeyA: &key},
	}, nil)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, nfc.ErrCanceled) || result.BackupPath == "" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestFileUIDBackupStoreWritesRecoveryJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")
	store := workflow.FileUIDBackupStore{Root: root}
	path, err := store.Save(context.Background(), workflow.UIDBackup{
		TaskID: "uid:task/1", Card: classic1KCard(1), Method: "cuid_gen2_ordinary",
		OldBlock0: [nfc.BlockSize]byte{1, 2, 3, 4, 4}, NewBlock0: [nfc.BlockSize]byte{5, 6, 7, 8, 12},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"oldBlock0": "0102030404`) || !strings.Contains(filepath.Base(path), "-uidtask1-") {
		t.Fatalf("path=%s data=%s", path, data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode=%v", info.Mode().Perm())
	}
}
