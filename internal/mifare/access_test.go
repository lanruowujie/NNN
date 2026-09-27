package mifare_test

import (
	"testing"

	"github.com/BennyThink/NFCX/internal/mifare"
)

func TestAccessBitsTransportConfigurationRoundTrip(t *testing.T) {
	decoded, err := mifare.DecodeAccessBits([3]byte{0xff, 0x07, 0x80})
	if err != nil {
		t.Fatal(err)
	}
	for group := 0; group < 3; group++ {
		if decoded.Groups[group].Code() != 0 {
			t.Errorf("data group %d code = %03b; want 000", group, decoded.Groups[group].Code())
		}
	}
	if decoded.Groups[3].Code() != 1 {
		t.Fatalf("trailer code = %03b; want 001", decoded.Groups[3].Code())
	}
	if encoded := mifare.EncodeAccessBits(decoded); encoded != ([3]byte{0xff, 0x07, 0x80}) {
		t.Fatalf("EncodeAccessBits() = %x", encoded)
	}
}

func TestAccessBitsAllConditionsRoundTrip(t *testing.T) {
	for packed := 0; packed < 1<<12; packed++ {
		var value mifare.AccessBits
		for group := range value.Groups {
			code := (packed >> (group * 3)) & 7
			value.Groups[group] = mifare.AccessCondition{C1: code&4 != 0, C2: code&2 != 0, C3: code&1 != 0}
		}
		decoded, err := mifare.DecodeAccessBits(mifare.EncodeAccessBits(value))
		if err != nil || decoded != value {
			t.Fatalf("access bits round trip %03x = %+v, %v", packed, decoded, err)
		}
	}
}

func TestAccessBitsRejectBrokenComplements(t *testing.T) {
	for index := 0; index < 3; index++ {
		value := [3]byte{0xff, 0x07, 0x80}
		value[index] ^= 1
		if _, err := mifare.DecodeAccessBits(value); err == nil {
			t.Errorf("DecodeAccessBits(%x) accepted malformed complements", value)
		}
	}
}

func TestAccessConditionsDescribeSafeWrites(t *testing.T) {
	if got := (mifare.AccessCondition{}).DataWriteKeys(); got != mifare.KeyMaskAny {
		t.Fatalf("transport data write keys = %d", got)
	}
	if got := (mifare.AccessCondition{C3: true}).FullTrailerWriteKeys(); got != mifare.KeyMaskA {
		t.Fatalf("transport trailer write keys = %d", got)
	}
	if got := (mifare.AccessCondition{C2: true, C3: true}).FullTrailerWriteKeys(); got != mifare.KeyMaskB {
		t.Fatalf("key-B trailer write keys = %d", got)
	}
	if !(mifare.AccessCondition{}).KeyBReadable() {
		t.Fatal("000 trailer should expose Key B as data")
	}
}

func TestAccessGroupForLargeSector(t *testing.T) {
	wants := map[int]int{128: 0, 132: 0, 133: 1, 137: 1, 138: 2, 142: 2, 143: 3}
	for block, want := range wants {
		got, err := mifare.Classic4K.AccessGroupForBlock(block)
		if err != nil || got != want {
			t.Errorf("AccessGroupForBlock(%d) = %d, %v; want %d", block, got, err, want)
		}
	}
}
