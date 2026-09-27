//go:build libnfc && cgo

package libnfc

/*
#cgo pkg-config: libnfc
#cgo darwin LDFLAGS: -framework UniformTypeIdentifiers
#include "shim.h"
*/
import "C"

import (
	"unsafe"

	"github.com/BennyThink/NFCX/internal/nfc"
)

const (
	maxEnumeratedDevices = 32
	detailBufferSize     = 512
)

type cgoHandle struct {
	ptr *C.nfcx_handle
}

func (*cgoHandle) isNativeHandle() {}

type cgoNative struct{}

func platformNative() nativeDriver { return cgoNative{} }

func platformRuntimeVersion() (string, nativeResult) {
	var output [64]C.char
	var outputLength C.size_t
	code := C.nfcx_runtime_version(&output[0], C.size_t(len(output)), &outputLength)
	if code != C.NFCX_OK {
		return "", nativeResult{code: int(code), detail: "libnfc version is unavailable"}
	}
	if outputLength >= C.size_t(len(output)) {
		return "", nativeResult{code: nativeInternal, detail: "libnfc returned an invalid version length"}
	}
	return C.GoStringN(&output[0], C.int(outputLength)), nativeResult{code: nativeOK}
}

func (cgoNative) listDevices() ([]nfc.DeviceInfo, nativeResult) {
	records := make([]C.nfcx_device_info, maxEnumeratedDevices)
	var count C.size_t
	var detail [detailBufferSize]C.char
	var detailLength C.size_t
	code := C.nfcx_list_devices(
		&records[0],
		C.size_t(len(records)),
		&count,
		&detail[0],
		C.size_t(len(detail)),
		&detailLength,
	)
	result := cgoResult(code, &detail[0], detailLength, len(detail))
	if result.code != nativeOK {
		return nil, result
	}
	if count > C.size_t(len(records)) {
		return nil, nativeResult{code: nativeInternal, detail: "libnfc returned too many devices"}
	}
	devices := make([]nfc.DeviceInfo, 0, int(count))
	for index := 0; index < int(count); index++ {
		record := &records[index]
		if record.connstring_length >= C.size_t(C.NFCX_CONNSTRING_MAX) {
			return nil, nativeResult{code: nativeInternal, detail: "libnfc returned an invalid connstring length"}
		}
		connString := C.GoStringN(
			(*C.char)(unsafe.Pointer(&record.connstring[0])),
			C.int(record.connstring_length),
		)
		devices = append(devices, nfc.DeviceInfo{Name: connString, ConnString: connString})
	}
	return devices, result
}

func (cgoNative) open(connString string) (nativeHandle, nativeResult) {
	bytes := []byte(connString)
	var output *C.nfcx_handle
	var detail [detailBufferSize]C.char
	var detailLength C.size_t
	code := C.nfcx_reader_open(
		(*C.char)(unsafe.Pointer(&bytes[0])),
		C.size_t(len(bytes)),
		&output,
		&detail[0],
		C.size_t(len(detail)),
		&detailLength,
	)
	result := cgoResult(code, &detail[0], detailLength, len(detail))
	if result.code != nativeOK {
		return nil, result
	}
	return &cgoHandle{ptr: output}, result
}

func (cgoNative) deviceName(handle nativeHandle) (string, nativeResult) {
	native, result := cgoHandleFrom(handle)
	if result.code != nativeOK {
		return "", result
	}
	var output [C.NFCX_DEVICE_NAME_MAX]C.char
	var outputLength C.size_t
	result = callWithDetail(func(detail *C.char, capacity C.size_t, length *C.size_t) C.int {
		return C.nfcx_reader_name(
			native.ptr,
			&output[0],
			C.size_t(len(output)),
			&outputLength,
			detail,
			capacity,
			length,
		)
	})
	if result.code != nativeOK {
		return "", result
	}
	if outputLength >= C.size_t(len(output)) {
		return "", nativeResult{code: nativeInternal, detail: "C shim returned an invalid device name length"}
	}
	return C.GoStringN(&output[0], C.int(outputLength)), result
}

func (cgoNative) close(handle nativeHandle) nativeResult {
	native, result := cgoHandleFrom(handle)
	if result.code != nativeOK {
		return result
	}
	var detail [detailBufferSize]C.char
	var detailLength C.size_t
	code := C.nfcx_reader_close(
		&native.ptr,
		&detail[0],
		C.size_t(len(detail)),
		&detailLength,
	)
	return cgoResult(code, &detail[0], detailLength, len(detail))
}

func (cgoNative) abort(handle nativeHandle) nativeResult {
	native, result := cgoHandleFrom(handle)
	if result.code != nativeOK {
		return result
	}
	return callWithDetail(func(detail *C.char, capacity C.size_t, length *C.size_t) C.int {
		return C.nfcx_reader_abort(native.ptr, detail, capacity, length)
	})
}

func (cgoNative) cardInfo(handle nativeHandle) (nfc.CardInfo, nativeResult) {
	native, result := cgoHandleFrom(handle)
	if result.code != nativeOK {
		return nfc.CardInfo{}, result
	}
	var output C.nfcx_card_info
	result = callWithDetail(func(detail *C.char, capacity C.size_t, length *C.size_t) C.int {
		return C.nfcx_reader_card_info(native.ptr, &output, detail, capacity, length)
	})
	if result.code != nativeOK {
		return nfc.CardInfo{}, result
	}
	if output.uid_length > C.size_t(C.NFCX_UID_MAX) || output.atqa_length != C.NFCX_ATQA_LENGTH {
		return nfc.CardInfo{}, nativeResult{code: nativeInternal, detail: "C shim returned invalid card buffer lengths"}
	}
	uid := C.GoBytes(unsafe.Pointer(&output.uid[0]), C.int(output.uid_length))
	return nfc.CardInfo{
		UID:  uid,
		ATQA: [nfc.ATQALength]byte{byte(output.atqa[0]), byte(output.atqa[1])},
		SAK:  byte(output.sak),
	}, result
}

func (cgoNative) authenticate(handle nativeHandle, block byte, command byte, key nfc.Key) nativeResult {
	native, result := cgoHandleFrom(handle)
	if result.code != nativeOK {
		return result
	}
	return callWithDetail(func(detail *C.char, capacity C.size_t, length *C.size_t) C.int {
		return C.nfcx_reader_authenticate(
			native.ptr,
			C.uint8_t(block),
			C.uint8_t(command),
			(*C.uint8_t)(unsafe.Pointer(&key[0])),
			C.size_t(len(key)),
			detail,
			capacity,
			length,
		)
	})
}

func (cgoNative) readBlock(handle nativeHandle, block byte) ([nfc.BlockSize]byte, nativeResult) {
	native, result := cgoHandleFrom(handle)
	if result.code != nativeOK {
		return [nfc.BlockSize]byte{}, result
	}
	var output [nfc.BlockSize]byte
	var outputLength C.size_t
	result = callWithDetail(func(detail *C.char, capacity C.size_t, length *C.size_t) C.int {
		return C.nfcx_reader_read_block(
			native.ptr,
			C.uint8_t(block),
			(*C.uint8_t)(unsafe.Pointer(&output[0])),
			C.size_t(len(output)),
			&outputLength,
			detail,
			capacity,
			length,
		)
	})
	if result.code == nativeOK && outputLength != C.NFCX_BLOCK_LENGTH {
		return [nfc.BlockSize]byte{}, nativeResult{code: nativeInternal, detail: "C shim returned an invalid block length"}
	}
	return output, result
}

func (cgoNative) writeBlock(handle nativeHandle, block byte, data [nfc.BlockSize]byte) nativeResult {
	native, result := cgoHandleFrom(handle)
	if result.code != nativeOK {
		return result
	}
	return callWithDetail(func(detail *C.char, capacity C.size_t, length *C.size_t) C.int {
		return C.nfcx_reader_write_block(
			native.ptr,
			C.uint8_t(block),
			(*C.uint8_t)(unsafe.Pointer(&data[0])),
			C.size_t(len(data)),
			detail,
			capacity,
			length,
		)
	})
}

func (cgoNative) writeManufacturerBlock(handle nativeHandle, data [nfc.BlockSize]byte) nativeResult {
	native, result := cgoHandleFrom(handle)
	if result.code != nativeOK {
		return result
	}
	return callWithDetail(func(detail *C.char, capacity C.size_t, length *C.size_t) C.int {
		return C.nfcx_reader_write_manufacturer_block(
			native.ptr,
			(*C.uint8_t)(unsafe.Pointer(&data[0])),
			C.size_t(len(data)),
			detail,
			capacity,
			length,
		)
	})
}

func cgoHandleFrom(handle nativeHandle) (*cgoHandle, nativeResult) {
	native, ok := handle.(*cgoHandle)
	if !ok || native == nil || native.ptr == nil {
		return nil, nativeResult{code: nativeNotOpen, detail: "reader is not open"}
	}
	return native, nativeResult{code: nativeOK}
}

func callWithDetail(call func(*C.char, C.size_t, *C.size_t) C.int) nativeResult {
	var detail [detailBufferSize]C.char
	var detailLength C.size_t
	code := call(&detail[0], C.size_t(len(detail)), &detailLength)
	return cgoResult(code, &detail[0], detailLength, len(detail))
}

func cgoResult(code C.int, detail *C.char, detailLength C.size_t, capacity int) nativeResult {
	length := int(detailLength)
	if length >= capacity {
		length = capacity - 1
	}
	message := ""
	if length > 0 {
		message = C.GoStringN(detail, C.int(length))
	}
	return nativeResult{code: int(code), detail: message}
}

func nativeSmoke() nativeResult {
	return callWithDetail(func(detail *C.char, capacity C.size_t, length *C.size_t) C.int {
		return C.nfcx_smoke(detail, capacity, length)
	})
}

func nativeConfigureDiscoveryDefaults() nativeResult {
	return callWithDetail(func(detail *C.char, capacity C.size_t, length *C.size_t) C.int {
		return C.nfcx_configure_discovery_defaults(detail, capacity, length)
	})
}

type resourceCount struct {
	contexts int
	devices  int
	handles  int
}

func nativeResources() resourceCount {
	var count C.nfcx_resource_count
	C.nfcx_get_resource_count(&count)
	return resourceCount{
		contexts: int(count.contexts),
		devices:  int(count.devices),
		handles:  int(count.handles),
	}
}
