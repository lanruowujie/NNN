package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
	"github.com/BennyThink/NFCX/internal/workflow"
)

type directExecutor struct{ reader nfc.Reader }

func (e directExecutor) WithReader(ctx context.Context, operation func(context.Context, nfc.Reader) error) error {
	return operation(ctx, e.reader)
}

func classic1KCard(uid byte) nfc.CardInfo {
	return nfc.CardInfo{UID: []byte{uid, 2, 3, 4}, ATQA: [2]byte{0, 4}, SAK: 0x08}
}

func openMockReader(t *testing.T, funcs mock.ReaderFuncs) *mock.Reader {
	t.Helper()
	reader := mock.NewReaderWithFuncs(funcs)
	if err := reader.Open(context.Background(), "mock:test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	return reader
}

func TestClassicIOReadSelectsAuthenticatesAndReads(t *testing.T) {
	card := classic1KCard(1)
	want := [nfc.BlockSize]byte{0xde, 0xad, 0xbe, 0xef}
	var calls []string
	reader := openMockReader(t, mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) {
			calls = append(calls, "select")
			return card, nil
		},
		Authenticate: func(_ context.Context, block byte, keyType nfc.KeyType, key nfc.Key) error {
			calls = append(calls, "authenticate")
			if block != 1 || keyType != nfc.KeyTypeA || key != (nfc.Key{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}) {
				t.Fatalf("unexpected authentication arguments: %d %d %x", block, keyType, key)
			}
			return nil
		},
		ReadBlock: func(_ context.Context, block byte) ([nfc.BlockSize]byte, error) {
			calls = append(calls, "read")
			return want, nil
		},
	})
	service := workflow.NewClassicIO(directExecutor{reader: reader})
	got, err := service.ReadBlock(context.Background(), workflow.ClassicBlockRequest{
		Card: card, Block: 1, KeyType: nfc.KeyTypeA,
		Key: nfc.Key{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	})
	if err != nil || got != want {
		t.Fatalf("ReadBlock() = %x, %v; want %x", got, err, want)
	}
	if got := strings.Join(calls, ","); got != "select,authenticate,read" {
		t.Fatalf("calls = %s; want select,authenticate,read", got)
	}
}

func TestClassicIORejectsReplacementCardBeforeAuthentication(t *testing.T) {
	expected := classic1KCard(1)
	authenticated := false
	reader := openMockReader(t, mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) { return classic1KCard(9), nil },
		Authenticate: func(context.Context, byte, nfc.KeyType, nfc.Key) error {
			authenticated = true
			return nil
		},
	})
	service := workflow.NewClassicIO(directExecutor{reader: reader})
	_, err := service.ReadBlock(context.Background(), workflow.ClassicBlockRequest{Card: expected, Block: 1, KeyType: nfc.KeyTypeA})
	if !errors.Is(err, nfc.ErrCardChanged) {
		t.Fatalf("ReadBlock() error = %v; want ErrCardChanged", err)
	}
	if authenticated {
		t.Fatal("replacement card was authenticated")
	}
}

func TestClassicIOWriteSelectsAuthenticatesAndWrites(t *testing.T) {
	card := classic1KCard(1)
	data := [nfc.BlockSize]byte{1, 2, 3}
	var calls []string
	reader := openMockReader(t, mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) {
			calls = append(calls, "select")
			return card, nil
		},
		Authenticate: func(_ context.Context, block byte, keyType nfc.KeyType, _ nfc.Key) error {
			calls = append(calls, "authenticate")
			if block != 2 || keyType != nfc.KeyTypeB {
				t.Fatalf("authentication = block %d type %d; want block 2 Key B", block, keyType)
			}
			return nil
		},
		WriteBlock: func(_ context.Context, block byte, got [nfc.BlockSize]byte) error {
			calls = append(calls, "write")
			if block != 2 || got != data {
				t.Fatalf("write = block %d data %x", block, got)
			}
			return nil
		},
	})
	service := workflow.NewClassicIO(directExecutor{reader: reader})
	if err := service.WriteBlock(context.Background(), workflow.ClassicBlockRequest{Card: card, Block: 2, KeyType: nfc.KeyTypeB}, data); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "select,authenticate,write" {
		t.Fatalf("calls = %s; want select,authenticate,write", got)
	}
}

func TestClassicIOValidatesBlocksAndProtectedWrites(t *testing.T) {
	card := classic1KCard(1)
	service := workflow.NewClassicIO(nil)
	for _, block := range []int{-1, 64} {
		_, err := service.ReadBlock(context.Background(), workflow.ClassicBlockRequest{Card: card, Block: block, KeyType: nfc.KeyTypeA})
		if !errors.Is(err, nfc.ErrInvalidArgument) {
			t.Errorf("ReadBlock(block=%d) error = %v; want ErrInvalidArgument", block, err)
		}
	}
	for _, block := range []int{0, 3} {
		err := service.WriteBlock(context.Background(), workflow.ClassicBlockRequest{Card: card, Block: block, KeyType: nfc.KeyTypeB}, [nfc.BlockSize]byte{})
		if !errors.Is(err, nfc.ErrInvalidArgument) {
			t.Errorf("WriteBlock(block=%d) error = %v; want ErrInvalidArgument", block, err)
		}
	}
}

func TestClassicIOPropagatesOperationErrors(t *testing.T) {
	card := classic1KCard(1)
	for _, want := range []error{nfc.ErrAuthenticationFailed, nfc.ErrNoCard, nfc.ErrDeviceDisconnected} {
		reader := openMockReader(t, mock.ReaderFuncs{
			CardInfo:     func(context.Context) (nfc.CardInfo, error) { return card, nil },
			Authenticate: func(context.Context, byte, nfc.KeyType, nfc.Key) error { return want },
		})
		service := workflow.NewClassicIO(directExecutor{reader: reader})
		_, err := service.ReadBlock(context.Background(), workflow.ClassicBlockRequest{Card: card, Block: 1, KeyType: nfc.KeyTypeA})
		if !errors.Is(err, want) {
			t.Errorf("ReadBlock() error = %v; want %v", err, want)
		}
	}
}
