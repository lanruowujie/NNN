package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
)

type uidFallbackFunc func(context.Context, UIDFallbackRequest, func(UIDWriteEvent)) error

func (f uidFallbackFunc) Write(ctx context.Context, request UIDFallbackRequest, emit func(UIDWriteEvent)) error {
	return f(ctx, request, emit)
}

type uidTestExecutor struct{ reader nfc.Reader }

func (e uidTestExecutor) WithReader(ctx context.Context, operation func(context.Context, nfc.Reader) error) error {
	return operation(ctx, e.reader)
}

type uidTestBackups struct{}

func (uidTestBackups) Save(context.Context, UIDBackup) (string, error) { return "/backup.json", nil }

func TestUIDWriteFallsBackToGen1AAndStillVerifiesFullBlock(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{4, 0}, SAK: 0x08}
	block0 := [nfc.BlockSize]byte{1, 2, 3, 4, 4, 0x08, 0x04, 0x00, 0x62, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69}
	trailer := [nfc.BlockSize]byte{6: 0xff, 7: 0x07, 8: 0x80, 9: 0x69}
	reader := mock.NewReaderWithFuncs(mock.ReaderFuncs{
		CardInfo:     func(context.Context) (nfc.CardInfo, error) { return card.Clone(), nil },
		Authenticate: func(context.Context, byte, nfc.KeyType, nfc.Key) error { return nil },
		ReadBlock: func(_ context.Context, block byte) ([nfc.BlockSize]byte, error) {
			if block == 3 {
				return trailer, nil
			}
			return block0, nil
		},
		WriteManufacturer: func(context.Context, [nfc.BlockSize]byte) error {
			return nfc.NewError("write manufacturer block", nfc.CodeIO, "RF Transmission Error", nil)
		},
	})
	if err := reader.Open(context.Background(), "mock:uid"); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	fallbackCalls := 0
	fallback := uidFallbackFunc(func(_ context.Context, request UIDFallbackRequest, _ func(UIDWriteEvent)) error {
		fallbackCalls++
		block0 = request.Block0
		card.UID = append([]byte(nil), request.Block0[0:4]...)
		return nil
	})
	service := NewUIDService(uidTestExecutor{reader: reader}, uidTestBackups{}, fallback)
	service.reselectLimit = 2 * time.Millisecond
	service.retryDelay = time.Millisecond
	key := nfc.Key{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	result, err := service.Write(context.Background(), UIDWriteRequest{
		TaskID: "uid-fallback", Card: card.Clone(), Device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/test"},
		NewUID: []byte{5, 6, 7, 8}, Sector0Keys: SectorKeys{Sector: 0, KeyA: &key},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fallbackCalls != 1 || !result.Verified || block0 != result.NewBlock0 {
		t.Fatalf("fallbackCalls=%d result=%+v block0=%x", fallbackCalls, result, block0)
	}
}

func TestUIDWriteDoesNotFallbackWhenCardDisappears(t *testing.T) {
	card := nfc.CardInfo{UID: []byte{1, 2, 3, 4}, ATQA: [2]byte{4, 0}, SAK: 0x08}
	block0 := [nfc.BlockSize]byte{1, 2, 3, 4, 4}
	trailer := [nfc.BlockSize]byte{6: 0xff, 7: 0x07, 8: 0x80, 9: 0x69}
	written := false
	reader := mock.NewReaderWithFuncs(mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) {
			if written {
				return nfc.CardInfo{}, nfc.NewError("card info", nfc.CodeNoCard, "no card", nil)
			}
			return card.Clone(), nil
		},
		Authenticate: func(context.Context, byte, nfc.KeyType, nfc.Key) error { return nil },
		ReadBlock: func(_ context.Context, block byte) ([nfc.BlockSize]byte, error) {
			if block == 3 {
				return trailer, nil
			}
			return block0, nil
		},
		WriteManufacturer: func(context.Context, [nfc.BlockSize]byte) error { written = true; return nil },
	})
	if err := reader.Open(context.Background(), "mock:uid"); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	fallbackCalled := false
	service := NewUIDService(uidTestExecutor{reader: reader}, uidTestBackups{}, uidFallbackFunc(func(context.Context, UIDFallbackRequest, func(UIDWriteEvent)) error {
		fallbackCalled = true
		return nil
	}))
	service.reselectLimit = 2 * time.Millisecond
	service.retryDelay = time.Millisecond
	key := nfc.Key{}
	_, err := service.Write(context.Background(), UIDWriteRequest{Card: card, NewUID: []byte{5, 6, 7, 8}, Sector0Keys: SectorKeys{Sector: 0, KeyA: &key}}, nil)
	if err == nil || fallbackCalled || !errors.Is(err, nfc.ErrVerificationFailed) {
		t.Fatalf("error=%v fallbackCalled=%v", err, fallbackCalled)
	}
}
