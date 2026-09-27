package mifare

import "fmt"

// AccessBits is the decoded form of bytes 6..8 in a sector trailer. Groups
// zero through two describe data blocks; group three describes the trailer.
// In large Classic 4K sectors, each data group covers five consecutive blocks.
type AccessBits struct {
	Groups [4]AccessCondition
}

// AccessCondition contains the three access-control bits for one block group.
type AccessCondition struct {
	C1 bool
	C2 bool
	C3 bool
}

// KeyMask describes which authentication key permits an operation.
type KeyMask uint8

const (
	KeyMaskNone KeyMask = 0
	KeyMaskA    KeyMask = 1 << (iota - 1)
	KeyMaskB
	KeyMaskAny = KeyMaskA | KeyMaskB
)

func (m KeyMask) AllowsA() bool { return m&KeyMaskA != 0 }
func (m KeyMask) AllowsB() bool { return m&KeyMaskB != 0 }

// DecodeAccessBits validates the inverted copies and returns all four access
// conditions. It never repairs malformed input.
func DecodeAccessBits(bytes678 [3]byte) (AccessBits, error) {
	b6, b7, b8 := bytes678[0], bytes678[1], bytes678[2]
	if (b6&0x0f)^((b7>>4)&0x0f) != 0x0f ||
		((b6>>4)&0x0f)^(b8&0x0f) != 0x0f ||
		(b7&0x0f)^((b8>>4)&0x0f) != 0x0f {
		return AccessBits{}, fmt.Errorf("invalid MIFARE Classic access-bit complements: %02X %02X %02X", b6, b7, b8)
	}

	var decoded AccessBits
	for group := 0; group < len(decoded.Groups); group++ {
		mask := byte(1 << group)
		decoded.Groups[group] = AccessCondition{
			C1: b7&(mask<<4) != 0,
			C2: b8&mask != 0,
			C3: b8&(mask<<4) != 0,
		}
	}
	return decoded, nil
}

// EncodeAccessBits produces the redundant on-card representation.
func EncodeAccessBits(bits AccessBits) [3]byte {
	var c1, c2, c3 byte
	for group, condition := range bits.Groups {
		mask := byte(1 << group)
		if condition.C1 {
			c1 |= mask
		}
		if condition.C2 {
			c2 |= mask
		}
		if condition.C3 {
			c3 |= mask
		}
	}
	return [3]byte{(^c2&0x0f)<<4 | (^c1 & 0x0f), c1<<4 | (^c3 & 0x0f), c3<<4 | c2}
}

// Code returns C1C2C3 as a three-bit value.
func (c AccessCondition) Code() uint8 {
	var result uint8
	if c.C1 {
		result |= 4
	}
	if c.C2 {
		result |= 2
	}
	if c.C3 {
		result |= 1
	}
	return result
}

// DataWriteKeys reports which keys may write a data block under this access
// condition. Manufacturer block 0 remains read-only regardless of this value.
func (c AccessCondition) DataWriteKeys() KeyMask {
	switch c.Code() {
	case 0:
		return KeyMaskAny
	case 4, 6, 3:
		return KeyMaskB
	default:
		return KeyMaskNone
	}
}

// KeyBReadable reports whether trailer bytes 10..15 are returned as data.
// In every other trailer condition the card masks both key fields with zeros.
func (c AccessCondition) KeyBReadable() bool {
	switch c.Code() {
	case 0, 2, 1:
		return true
	default:
		return false
	}
}

// FullTrailerWriteKeys returns the key capable of changing Key A, the access
// bytes, and Key B together. Conditions that allow only a subset are rejected
// because NFCX writes a complete 16-byte trailer.
func (c AccessCondition) FullTrailerWriteKeys() KeyMask {
	switch c.Code() {
	case 1:
		return KeyMaskA
	case 3:
		return KeyMaskB
	default:
		return KeyMaskNone
	}
}

// AccessGroupForBlock maps a block to C1x/C2x/C3x group zero through three.
func (l Layout) AccessGroupForBlock(block int) (int, error) {
	sector, err := l.SectorForBlock(block)
	if err != nil {
		return 0, err
	}
	first, _ := l.FirstBlock(sector)
	count, _ := l.BlocksInSector(sector)
	local := block - first
	if local == count-1 {
		return 3, nil
	}
	if count == 16 {
		return local / 5, nil
	}
	return local, nil
}
