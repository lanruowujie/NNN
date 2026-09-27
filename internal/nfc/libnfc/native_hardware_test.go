//go:build libnfc && cgo && libnfc_hardware

package libnfc

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
)

func TestHardwareReaderLifecycleAndCardInfo(t *testing.T) {
	connString := os.Getenv("LIBNFC_DEVICE")
	if connString == "" {
		t.Fatal("LIBNFC_DEVICE is required for the explicit hardware test")
	}

	backend := NewBackend()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	devices, err := backend.ListDevices(ctx)
	cancel()
	if err != nil {
		t.Fatalf("ListDevices() error = %v", err)
	}
	found := false
	for _, device := range devices {
		if device.ConnString == connString {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("configured connstring %q was not enumerated: %+v", connString, devices)
	}

	before := nativeResources()
	reader := backend.NewReader()
	for iteration := 0; iteration < 100; iteration++ {
		ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
		err = reader.Open(ctx, connString)
		cancel()
		if err != nil {
			t.Fatalf("Open() iteration %d error = %v", iteration, err)
		}
		if err = reader.Close(); err != nil {
			t.Fatalf("Close() iteration %d error = %v", iteration, err)
		}
	}
	if after := nativeResources(); after != before {
		t.Fatalf("resources after 100 open/close cycles = %+v; want %+v", after, before)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	if err = reader.Open(ctx, connString); err != nil {
		cancel()
		t.Fatalf("final Open() error = %v", err)
	}
	cancel()
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	}()

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	card, err := reader.CardInfo(ctx)
	cancel()
	if errors.Is(err, nfc.ErrNoCard) {
		t.Fatal("CardInfo() found no card; place an owned ISO14443A card on the reader and retry")
	}
	if err != nil {
		t.Fatalf("CardInfo() error = %v", err)
	}
	if length := len(card.UID); length != 4 && length != 7 && length != 10 {
		t.Fatalf("UID length = %d; want 4, 7, or 10", length)
	}
	// Polling and workflow preflights call CardInfo repeatedly without closing
	// the reader. This catches PN532 UART regressions where an unrestricted
	// second selection times out while the first target remains selected.
	for iteration := 0; iteration < 20; iteration++ {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		current, cardErr := reader.CardInfo(ctx)
		cancel()
		if cardErr != nil {
			t.Fatalf("repeated CardInfo() iteration %d error = %v", iteration, cardErr)
		}
		if !nfc.SameCard(card, current) {
			t.Fatalf("repeated CardInfo() iteration %d card = %+v; want %+v", iteration, current, card)
		}
	}
}

func TestHardwareMIFAREClassicKnownKeyIO(t *testing.T) {
	connString := os.Getenv("LIBNFC_DEVICE")
	if connString == "" {
		t.Fatal("LIBNFC_DEVICE is required for the explicit hardware test")
	}
	key := hardwareKey(t)
	reader := NewReader()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := reader.Open(ctx, connString); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	card, err := reader.CardInfo(ctx)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	layout := hardwareLayout(t, card)

	// Block 0 is read-only in NFCX but authenticating and reading it proves the
	// known Key A path without mutating the card.
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	err = reader.Authenticate(ctx, 0, nfc.KeyTypeA, key)
	if err == nil {
		_, err = reader.ReadBlock(ctx, 0)
	}
	if err == nil {
		_, err = reader.ReadBlock(ctx, 1)
	}
	cancel()
	if err != nil {
		t.Fatalf("known-key sector 0 block read failed: %v", err)
	}

	// Derive a key that differs from the already-proven Key A. Authentication
	// must classify it explicitly instead of surfacing a generic RF timeout.
	wrongKey := key
	wrongKey[0] ^= 0x01
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	_, err = reader.CardInfo(ctx)
	if err == nil {
		err = reader.Authenticate(ctx, 0, nfc.KeyTypeA, wrongKey)
	}
	cancel()
	if !errors.Is(err, nfc.ErrAuthenticationFailed) {
		t.Fatalf("wrong Key A error = %v; want ErrAuthenticationFailed", err)
	}

	if err := reader.WriteBlock(context.Background(), 0, [nfc.BlockSize]byte{}); !errors.Is(err, nfc.ErrInvalidArgument) {
		t.Fatalf("ordinary block 0 write error = %v; want ErrInvalidArgument", err)
	}

	if os.Getenv("NFCX_HARDWARE_WRITE_CONFIRM") != "I_OWN_THIS_CARD" {
		t.Log("write acceptance skipped; set NFCX_HARDWARE_WRITE_CONFIRM=I_OWN_THIS_CARD plus NFCX_TEST_WRITE_BLOCK and NFCX_TEST_WRITE_HEX")
		return
	}
	block := hardwareWriteBlock(t, layout)
	data := hardwareWriteData(t)

	// CardInfo deliberately reselects the target and invalidates the earlier
	// authentication before the destructive portion begins.
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	actualCard, err := reader.CardInfo(ctx)
	if err == nil && !nfc.SameCard(card, actualCard) {
		err = nfc.ErrCardChanged
	}
	if err == nil {
		err = reader.Authenticate(ctx, block, nfc.KeyTypeA, key)
	}
	var original [nfc.BlockSize]byte
	if err == nil {
		original, err = reader.ReadBlock(ctx, block)
	}
	cancel()
	if err != nil {
		t.Fatalf("pre-write read failed: %v", err)
	}

	restoreNeeded := true
	defer func() {
		if !restoreNeeded {
			return
		}
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer restoreCancel()
		current, restoreErr := reader.CardInfo(restoreCtx)
		if restoreErr == nil && !nfc.SameCard(card, current) {
			restoreErr = nfc.ErrCardChanged
		}
		if restoreErr == nil {
			restoreErr = reader.Authenticate(restoreCtx, block, nfc.KeyTypeA, key)
		}
		if restoreErr == nil {
			restoreErr = reader.WriteBlock(restoreCtx, block, original)
		}
		if restoreErr != nil {
			t.Errorf("IMPORTANT: failed to restore original block %d: %v", block, restoreErr)
		}
	}()

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	err = reader.WriteBlock(ctx, block, data)
	cancel()
	if err != nil {
		t.Fatalf("test write or immediate verification failed: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	if err = reader.WriteBlock(ctx, block, original); err == nil {
		restoreNeeded = false
	}
	cancel()
	if err != nil {
		t.Fatalf("failed to restore original block %d: %v", block, err)
	}
}

func hardwareKey(t *testing.T) nfc.Key {
	t.Helper()
	value := os.Getenv("NFCX_TEST_KEY_A")
	if value == "" {
		value = "FFFFFFFFFFFF"
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("NFCX_TEST_KEY_A must be 12 hexadecimal digits: %v", err)
	}
	key, err := nfc.KeyFromBytes(decoded)
	if err != nil {
		t.Fatalf("NFCX_TEST_KEY_A: %v", err)
	}
	return key
}

func hardwareLayout(t *testing.T, card nfc.CardInfo) mifare.Layout {
	t.Helper()
	switch nfc.InferCardType(card) {
	case nfc.CardTypeMIFAREClassic1K:
		return mifare.Classic1K
	case nfc.CardTypeMIFAREClassic4K:
		return mifare.Classic4K
	default:
		t.Fatalf("selected card SAK %#02x is not Classic 1K/4K", card.SAK)
		return 0
	}
}

func hardwareWriteBlock(t *testing.T, layout mifare.Layout) byte {
	t.Helper()
	value := os.Getenv("NFCX_TEST_WRITE_BLOCK")
	block, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("NFCX_TEST_WRITE_BLOCK must be an integer: %v", err)
	}
	if _, err := layout.SectorForBlock(block); err != nil {
		t.Fatal(err)
	}
	isTrailer, _ := layout.IsTrailer(block)
	if block == 0 || isTrailer {
		t.Fatalf("NFCX_TEST_WRITE_BLOCK=%d is protected; choose a non-zero data block", block)
	}
	return byte(block)
}

func hardwareWriteData(t *testing.T) [nfc.BlockSize]byte {
	t.Helper()
	decoded, err := hex.DecodeString(os.Getenv("NFCX_TEST_WRITE_HEX"))
	if err != nil {
		t.Fatalf("NFCX_TEST_WRITE_HEX must be 32 hexadecimal digits: %v", err)
	}
	if len(decoded) != nfc.BlockSize {
		t.Fatalf("NFCX_TEST_WRITE_HEX decoded length = %d; want %d", len(decoded), nfc.BlockSize)
	}
	var data [nfc.BlockSize]byte
	copy(data[:], decoded)
	return data
}
