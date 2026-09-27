package nfc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/nfc"
)

func TestOpErrorMatchesCategoryAndCause(t *testing.T) {
	err := nfc.NewError("select", nfc.CodeTimeout, "native timeout", context.DeadlineExceeded)

	if !errors.Is(err, nfc.ErrTimeout) {
		t.Fatal("timeout error does not match ErrTimeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("timeout error does not retain context deadline cause")
	}
	code, ok := nfc.ErrorCodeOf(err)
	if !ok || code != nfc.CodeTimeout {
		t.Fatalf("ErrorCodeOf() = %v, %v; want %v, true", code, ok, nfc.CodeTimeout)
	}
}

func TestErrorCodeOfSentinel(t *testing.T) {
	code, ok := nfc.ErrorCodeOf(nfc.ErrDeviceDisconnected)
	if !ok || code != nfc.CodeDeviceDisconnected {
		t.Fatalf("ErrorCodeOf() = %v, %v; want %v, true", code, ok, nfc.CodeDeviceDisconnected)
	}
}

func TestPermissionErrorCategory(t *testing.T) {
	err := nfc.NewError("open", nfc.CodePermission, "serial port is not accessible", nil)
	if !errors.Is(err, nfc.ErrPermission) {
		t.Fatalf("permission error does not match ErrPermission: %v", err)
	}
}

func TestClassicOperationErrorCategories(t *testing.T) {
	for _, test := range []struct {
		code nfc.ErrorCode
		want error
	}{
		{nfc.CodeCardChanged, nfc.ErrCardChanged},
		{nfc.CodeNotAuthenticated, nfc.ErrNotAuthenticated},
		{nfc.CodeVerificationFailed, nfc.ErrVerificationFailed},
	} {
		err := nfc.NewError("classic operation", test.code, "", nil)
		if !errors.Is(err, test.want) {
			t.Errorf("error code %d produced %v; want category %v", test.code, err, test.want)
		}
	}
}

func TestCardInfoCloneCopiesUID(t *testing.T) {
	original := nfc.CardInfo{UID: []byte{0x01, 0x02, 0x03, 0x04}}
	clone := original.Clone()
	clone.UID[0] = 0xff
	if original.UID[0] != 0x01 {
		t.Fatal("Clone shares UID storage with original")
	}
}

func TestKeyTypeValidation(t *testing.T) {
	if !nfc.KeyTypeA.Valid() || !nfc.KeyTypeB.Valid() {
		t.Fatal("known key types must be valid")
	}
	if nfc.KeyType(2).Valid() {
		t.Fatal("unknown key type must be invalid")
	}
}

func TestKeyFromBytesRequiresExactLengthAndCopiesInput(t *testing.T) {
	for _, length := range []int{0, 5, 7} {
		if _, err := nfc.KeyFromBytes(make([]byte, length)); !errors.Is(err, nfc.ErrInvalidArgument) {
			t.Errorf("KeyFromBytes(%d bytes) error = %v; want ErrInvalidArgument", length, err)
		}
	}

	input := []byte{1, 2, 3, 4, 5, 6}
	key, err := nfc.KeyFromBytes(input)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = 0xff
	if key != (nfc.Key{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("KeyFromBytes() = %x; input storage was not copied", key)
	}
}
