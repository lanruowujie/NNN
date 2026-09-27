//go:build !libnfc || !cgo

package libnfc

import "github.com/BennyThink/NFCX/internal/nfc"

type stubHandle struct{}

func (*stubHandle) isNativeHandle() {}

type stubNative struct{}

func platformNative() nativeDriver { return stubNative{} }

func platformRuntimeVersion() (string, nativeResult) {
	return "", stubNative{}.unavailable()
}

func (stubNative) unavailable() nativeResult {
	return nativeResult{code: nativeUnsupported, detail: "libnfc support requires CGO and the libnfc build tag"}
}

func (s stubNative) listDevices() ([]nfc.DeviceInfo, nativeResult) {
	return nil, s.unavailable()
}

func (s stubNative) open(string) (nativeHandle, nativeResult) {
	return nil, s.unavailable()
}

func (s stubNative) deviceName(nativeHandle) (string, nativeResult) {
	return "", s.unavailable()
}

func (s stubNative) close(nativeHandle) nativeResult { return s.unavailable() }
func (s stubNative) abort(nativeHandle) nativeResult { return s.unavailable() }

func (s stubNative) cardInfo(nativeHandle) (nfc.CardInfo, nativeResult) {
	return nfc.CardInfo{}, s.unavailable()
}

func (s stubNative) authenticate(nativeHandle, byte, byte, nfc.Key) nativeResult {
	return s.unavailable()
}

func (s stubNative) readBlock(nativeHandle, byte) ([nfc.BlockSize]byte, nativeResult) {
	return [nfc.BlockSize]byte{}, s.unavailable()
}

func (s stubNative) writeBlock(nativeHandle, byte, [nfc.BlockSize]byte) nativeResult {
	return s.unavailable()
}

func (s stubNative) writeManufacturerBlock(nativeHandle, [nfc.BlockSize]byte) nativeResult {
	return s.unavailable()
}
