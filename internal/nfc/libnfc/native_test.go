package libnfc

import (
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/nfc"
)

func TestNativeErrorMapping(t *testing.T) {
	tests := []struct {
		code int
		want error
	}{
		{nativeNoCard, nfc.ErrNoCard},
		{nativeTimeout, nfc.ErrTimeout},
		{nativeAuthenticationFailed, nfc.ErrAuthenticationFailed},
		{nativeDeviceDisconnected, nfc.ErrDeviceDisconnected},
		{nativeIO, nfc.ErrIO},
		{nativeCanceled, nfc.ErrCanceled},
		{nativeInvalidArgument, nfc.ErrInvalidArgument},
		{nativeNotOpen, nfc.ErrNotOpen},
		{nativeUnsupported, nfc.ErrUnsupported},
		{nativeBusy, nfc.ErrBusy},
		{nativeInternal, nfc.ErrInternal},
		{nativePermission, nfc.ErrPermission},
		{nativeCardChanged, nfc.ErrCardChanged},
		{nativeNotAuthenticated, nfc.ErrNotAuthenticated},
		{nativeVerificationFailed, nfc.ErrVerificationFailed},
		{999, nfc.ErrInternal},
	}
	for _, test := range tests {
		err := (nativeResult{code: test.code, detail: "native detail"}).err("test")
		if !errors.Is(err, test.want) {
			t.Errorf("native code %d produced %v; want category %v", test.code, err, test.want)
		}
	}
	if err := (nativeResult{code: nativeOK}).err("test"); err != nil {
		t.Fatalf("native success produced error %v", err)
	}
}

func TestAuthenticationCommandMapping(t *testing.T) {
	for _, test := range []struct {
		keyType nfc.KeyType
		want    byte
		valid   bool
	}{
		{nfc.KeyTypeA, 0x60, true},
		{nfc.KeyTypeB, 0x61, true},
		{nfc.KeyType(2), 0, false},
	} {
		got, valid := authenticationCommand(test.keyType)
		if got != test.want || valid != test.valid {
			t.Errorf("authenticationCommand(%d) = %#x, %v; want %#x, %v", test.keyType, got, valid, test.want, test.valid)
		}
	}
}
