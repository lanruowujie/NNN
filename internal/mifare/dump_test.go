package mifare_test

import (
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/mifare"
)

func validRaw(size int) []byte {
	raw := make([]byte, size)
	raw[0], raw[1], raw[2], raw[3] = 1, 2, 3, 4
	raw[4] = 1 ^ 2 ^ 3 ^ 4
	layout := mifare.Classic1K
	if size == mifare.Classic4KDumpSize {
		layout = mifare.Classic4K
	}
	sectors, _ := layout.SectorCount()
	for sector := 0; sector < sectors; sector++ {
		block, _ := layout.TrailerBlock(sector)
		offset := block * mifare.BlockSize
		raw[offset+6], raw[offset+7], raw[offset+8] = 0xff, 0x07, 0x80
	}
	return raw
}

func TestParseRawAcceptsOnlyClassicSizes(t *testing.T) {
	for _, size := range []int{mifare.Classic1KDumpSize, mifare.Classic4KDumpSize} {
		dump, err := mifare.ParseRaw(validRaw(size))
		if err != nil || !dump.Complete() {
			t.Fatalf("ParseRaw(%d) complete=%v error=%v", size, dump.Complete(), err)
		}
		encoded, err := dump.Raw()
		if err != nil || len(encoded) != size {
			t.Fatalf("Raw() length=%d error=%v", len(encoded), err)
		}
	}
	for _, size := range []int{0, 1023, 1025, 4095, 4097} {
		if _, err := mifare.ParseRaw(make([]byte, size)); err == nil {
			t.Errorf("ParseRaw(%d) unexpectedly succeeded", size)
		}
	}
}

func TestPartialDumpCannotProduceRawOrValidateTrailers(t *testing.T) {
	dump, _ := mifare.NewDump(mifare.Classic1K)
	if _, err := dump.Raw(); !errors.Is(err, mifare.ErrIncompleteDump) {
		t.Fatalf("Raw() error = %v", err)
	}
	if err := dump.ValidateTrailers(); !errors.Is(err, mifare.ErrIncompleteDump) {
		t.Fatalf("ValidateTrailers() error = %v", err)
	}
}

func TestBCCValidInvalidAndNotApplicable(t *testing.T) {
	dump, _ := mifare.ParseRaw(validRaw(mifare.Classic1KDumpSize))
	if status, err := dump.ValidateBCC(4); status != mifare.BCCValid || err != nil {
		t.Fatalf("valid BCC = %s, %v", status, err)
	}
	dump.Blocks[0].Data[4] ^= 0xff
	if status, err := dump.ValidateBCC(4); status != mifare.BCCInvalid || err == nil {
		t.Fatalf("invalid BCC = %s, %v", status, err)
	}
	if status, err := dump.ValidateBCC(7); status != mifare.BCCNotApplicable || err != nil {
		t.Fatalf("7-byte UID BCC = %s, %v", status, err)
	}
	if status, err := dump.ValidateBCC(0); status != mifare.BCCNotApplicable || err != nil {
		t.Fatalf("unknown UID BCC = %s, %v", status, err)
	}
}

func TestBCCFor4ByteUIDRejectsOtherLengths(t *testing.T) {
	if got, err := mifare.BCCFor4ByteUID([]byte{1, 2, 3, 4}); err != nil || got != 4 {
		t.Fatalf("BCCFor4ByteUID() = %02x, %v; want 04", got, err)
	}
	for _, length := range []int{0, 3, 7, 10} {
		if _, err := mifare.BCCFor4ByteUID(make([]byte, length)); err == nil {
			t.Errorf("BCCFor4ByteUID(%d bytes) unexpectedly succeeded", length)
		}
	}
}

func TestValidateTrailersIdentifiesSector(t *testing.T) {
	dump, _ := mifare.ParseRaw(validRaw(mifare.Classic1KDumpSize))
	dump.Blocks[7].Data[6] ^= 1
	if err := dump.ValidateTrailers(); err == nil || err.Error()[:8] != "sector 1" {
		t.Fatalf("ValidateTrailers() error = %v", err)
	}
}

func TestValidateTrailersCoversLargeClassic4KSectors(t *testing.T) {
	dump, _ := mifare.ParseRaw(validRaw(mifare.Classic4KDumpSize))
	if err := dump.ValidateTrailers(); err != nil {
		t.Fatal(err)
	}
	dump.Blocks[255].Data[8] ^= 1
	if err := dump.ValidateTrailers(); err == nil {
		t.Fatal("invalid sector 39 trailer was accepted")
	}
}
