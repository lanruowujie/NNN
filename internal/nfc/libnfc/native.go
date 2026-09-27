package libnfc

import "github.com/BennyThink/NFCX/internal/nfc"

// These values mirror nfcx_error_code in shim.h. They are deliberately
// independent of libnfc's native error numbers.
const (
	nativeOK = iota
	nativeNoCard
	nativeTimeout
	nativeAuthenticationFailed
	nativeDeviceDisconnected
	nativeIO
	nativeCanceled
	nativeInvalidArgument
	nativeNotOpen
	nativeUnsupported
	nativeBusy
	nativeInternal
	nativePermission
	nativeCardChanged
	nativeNotAuthenticated
	nativeVerificationFailed
)

// MIFARE Classic command bytes live only in the libnfc backend. Application
// and workflow packages use semantic KeyType and Reader operations instead.
const (
	mifareCommandAuthenticateA byte = 0x60
	mifareCommandAuthenticateB byte = 0x61
	mifareCommandRead          byte = 0x30
	mifareCommandWrite         byte = 0xa0
)

func authenticationCommand(keyType nfc.KeyType) (byte, bool) {
	switch keyType {
	case nfc.KeyTypeA:
		return mifareCommandAuthenticateA, true
	case nfc.KeyTypeB:
		return mifareCommandAuthenticateB, true
	default:
		return 0, false
	}
}

type nativeResult struct {
	code   int
	detail string
}

func (r nativeResult) err(op string) error {
	if r.code == nativeOK {
		return nil
	}
	return nfc.NewError(op, goErrorCode(r.code), r.detail, nil)
}

func goErrorCode(code int) nfc.ErrorCode {
	switch code {
	case nativeNoCard:
		return nfc.CodeNoCard
	case nativeTimeout:
		return nfc.CodeTimeout
	case nativeAuthenticationFailed:
		return nfc.CodeAuthenticationFailed
	case nativeDeviceDisconnected:
		return nfc.CodeDeviceDisconnected
	case nativeIO:
		return nfc.CodeIO
	case nativeCanceled:
		return nfc.CodeCanceled
	case nativeInvalidArgument:
		return nfc.CodeInvalidArgument
	case nativeNotOpen:
		return nfc.CodeNotOpen
	case nativeUnsupported:
		return nfc.CodeUnsupported
	case nativeBusy:
		return nfc.CodeBusy
	case nativeInternal:
		return nfc.CodeInternal
	case nativePermission:
		return nfc.CodePermission
	case nativeCardChanged:
		return nfc.CodeCardChanged
	case nativeNotAuthenticated:
		return nfc.CodeNotAuthenticated
	case nativeVerificationFailed:
		return nfc.CodeVerificationFailed
	default:
		return nfc.CodeInternal
	}
}

type nativeHandle interface {
	isNativeHandle()
}

type nativeDriver interface {
	listDevices() ([]nfc.DeviceInfo, nativeResult)
	open(connString string) (nativeHandle, nativeResult)
	deviceName(nativeHandle) (string, nativeResult)
	close(nativeHandle) nativeResult
	abort(nativeHandle) nativeResult
	cardInfo(nativeHandle) (nfc.CardInfo, nativeResult)
	authenticate(nativeHandle, byte, byte, nfc.Key) nativeResult
	readBlock(nativeHandle, byte) ([nfc.BlockSize]byte, nativeResult)
	writeBlock(nativeHandle, byte, [nfc.BlockSize]byte) nativeResult
	writeManufacturerBlock(nativeHandle, [nfc.BlockSize]byte) nativeResult
}
