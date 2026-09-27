package mock_test

import (
	"context"
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
)

func TestReaderDefaultCardStates(t *testing.T) {
	reader := mock.NewReader()
	if _, err := reader.CardInfo(context.Background()); !errors.Is(err, nfc.ErrNotOpen) {
		t.Fatalf("CardInfo() before Open error = %v; want ErrNotOpen", err)
	}
	if err := reader.Open(context.Background(), "mock:test"); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := reader.CardInfo(context.Background()); !errors.Is(err, nfc.ErrNoCard) {
		t.Fatalf("CardInfo() error = %v; want ErrNoCard", err)
	}

	reader.SetCardResult(nfc.CardInfo{UID: []byte{1, 2, 3, 4}}, nfc.ErrTimeout)
	if _, err := reader.CardInfo(context.Background()); !errors.Is(err, nfc.ErrTimeout) {
		t.Fatalf("CardInfo() error = %v; want ErrTimeout", err)
	}
	reader.SetCardResult(nfc.CardInfo{}, nfc.ErrDeviceDisconnected)
	if _, err := reader.CardInfo(context.Background()); !errors.Is(err, nfc.ErrDeviceDisconnected) {
		t.Fatalf("CardInfo() error = %v; want ErrDeviceDisconnected", err)
	}
}

func TestReaderCloseIsIdempotent(t *testing.T) {
	reader := mock.NewReader()
	if err := reader.Open(context.Background(), "mock:test"); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	open, _, opens, closes := reader.State()
	if open || opens != 1 || closes != 1 {
		t.Fatalf("State() = %v, %d, %d; want false, 1, 1", open, opens, closes)
	}
}

func TestReaderHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := mock.NewReader()
	err := reader.Open(ctx, "mock:test")
	if !errors.Is(err, nfc.ErrCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Open() error = %v; want NFC and context cancellation", err)
	}
}

func TestEnumeratorCopiesResults(t *testing.T) {
	enumerator := mock.NewEnumerator([]nfc.DeviceInfo{{Name: "Mock", ConnString: "mock:test"}}, nil)
	first, err := enumerator.ListDevices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first[0].Name = "changed"
	second, err := enumerator.ListDevices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Name != "Mock" {
		t.Fatal("enumerator returned shared result storage")
	}
}
