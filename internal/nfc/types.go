package nfc

import "context"

const (
	KeySize      = 6
	BlockSize    = 16
	MaxUIDLength = 10
	ATQALength   = 2
)

// Key is an exact MIFARE Classic key. Its fixed size prevents callers from
// accidentally passing truncated or oversized key material to a Reader.
type Key [KeySize]byte

// KeyFromBytes copies exactly six bytes into a Key. Keeping conversion behind
// this constructor prevents file, GUI, and subprocess inputs from being
// silently truncated or padded.
func KeyFromBytes(value []byte) (Key, error) {
	if len(value) != KeySize {
		return Key{}, NewError("parse key", CodeInvalidArgument, "MIFARE Classic keys must contain exactly 6 bytes", nil)
	}
	var key Key
	copy(key[:], value)
	return key, nil
}

// KeyType selects which key slot is used for authentication.
type KeyType uint8

const (
	KeyTypeA KeyType = iota
	KeyTypeB
)

// Valid reports whether the key type is one understood by NFCX.
func (k KeyType) Valid() bool {
	return k == KeyTypeA || k == KeyTypeB
}

// DeviceInfo contains only portable discovery information. Name may fall back
// to ConnString when a backend cannot determine a friendly name without
// opening the device.
type DeviceInfo struct {
	Name       string
	ConnString string
}

// CardInfo is a copy of the ISO/IEC 14443A selection data owned by Go.
type CardInfo struct {
	UID  []byte
	ATQA [ATQALength]byte
	SAK  byte
}

// Clone returns a deep copy so callers cannot mutate a Reader's retained UID.
func (c CardInfo) Clone() CardInfo {
	clone := c
	clone.UID = append([]byte(nil), c.UID...)
	return clone
}

// Reader is the only interface application services use for normal NFC
// operations. Implementations must serialize access to one underlying device.
// A successful CardInfo selection invalidates prior authentication. WriteBlock
// is the ordinary safe path: it rejects block 0 and sector trailers and returns
// success only after an immediate readback matches.
type Reader interface {
	Open(ctx context.Context, connString string) error
	Close() error
	CardInfo(ctx context.Context) (CardInfo, error)
	Authenticate(ctx context.Context, block byte, keyType KeyType, key Key) error
	ReadBlock(ctx context.Context, block byte) ([BlockSize]byte, error)
	WriteBlock(ctx context.Context, block byte, data [BlockSize]byte) error
}

// ClassicTrailerWriter is an intentionally separate capability for protected
// restore workflows. Ordinary Reader.WriteBlock continues to reject sector
// trailers, so application and GUI code cannot accidentally bypass the safety
// checks performed before this method is reached.
type ClassicTrailerWriter interface {
	WriteSectorTrailer(ctx context.Context, block byte, data [BlockSize]byte) error
}

// ClassicManufacturerWriter is an intentionally separate capability for the
// explicitly confirmed CUID/Gen2 workflow. It has no block argument so callers
// cannot use it as a general escape hatch around Reader.WriteBlock's block-0
// protection. A successful return means only that the card accepted the write;
// the workflow must reselect the card and verify its new UID and full block 0.
type ClassicManufacturerWriter interface {
	WriteManufacturerBlock(ctx context.Context, data [BlockSize]byte) error
}

// DeviceDescriber is an optional Reader capability used after Open when a
// backend can provide a friendly device name that enumeration cannot obtain.
type DeviceDescriber interface {
	DeviceInfo() DeviceInfo
}

// DeviceEnumerator discovers readers without requiring an opened Reader.
type DeviceEnumerator interface {
	ListDevices(ctx context.Context) ([]DeviceInfo, error)
}
