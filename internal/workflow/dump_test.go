package workflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func transportTrailer(key byte) [nfc.BlockSize]byte {
	return [nfc.BlockSize]byte{key, key, key, key, key, key, 0xff, 0x07, 0x80, 0x69, key, key, key, key, key, key}
}

func TestDumpReadProducesCompleteImageAndSynthesizesMaskedKeyA(t *testing.T) {
	card := classic1KCard(1)
	keyA := nfc.Key{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	reader := openMockReader(t, mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) { return card, nil },
		Authenticate: func(_ context.Context, _ byte, kind nfc.KeyType, key nfc.Key) error {
			if kind == nfc.KeyTypeA && key == keyA {
				return nil
			}
			return nfc.ErrAuthenticationFailed
		},
		ReadBlock: func(_ context.Context, block byte) ([nfc.BlockSize]byte, error) {
			if block%4 == 3 {
				trailer := transportTrailer(0xff)
				for index := 0; index < 6; index++ {
					trailer[index] = 0
				}
				return trailer, nil
			}
			return [nfc.BlockSize]byte{block}, nil
		},
	})
	keys := make([]workflow.SectorKeys, mifare.Classic1KSectors)
	for sector := range keys {
		keys[sector] = workflow.SectorKeys{Sector: sector, KeyA: &keyA}
	}
	service := workflow.NewDumpService(directExecutor{reader: reader})
	result, err := service.ReadCard(context.Background(), workflow.DumpRequest{Card: card, Keys: keys})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete() {
		t.Fatal("known Key A plus readable Key B should produce a complete dump")
	}
	trailer := result.Dump.Blocks[3]
	if trailer.Status != mifare.BlockSynthetic || trailer.KeyASource != "authenticated_key_a" || trailer.KeyBSource != "card_read" {
		t.Fatalf("trailer provenance = status %s A %q B %q", trailer.Status, trailer.KeyASource, trailer.KeyBSource)
	}
	if trailer.Data[0] != 0xff || trailer.KnownMask != mifare.AllBytesKnown {
		t.Fatalf("trailer data/mask = %x/%04x", trailer.Data, trailer.KnownMask)
	}
}

func TestDumpWithoutSectorKeyStaysPartial(t *testing.T) {
	card := classic1KCard(1)
	reader := openMockReader(t, mock.ReaderFuncs{CardInfo: func(context.Context) (nfc.CardInfo, error) { return card, nil }})
	result, err := workflow.NewDumpService(directExecutor{reader: reader}).ReadCard(context.Background(), workflow.DumpRequest{Card: card})
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete() {
		t.Fatal("dump with no known keys was marked complete")
	}
	if result.Dump.Blocks[0].Status != mifare.BlockAuthFailed || result.Dump.Blocks[63].Status != mifare.BlockAuthFailed {
		t.Fatalf("missing-key statuses = %s, %s", result.Dump.Blocks[0].Status, result.Dump.Blocks[63].Status)
	}
	if _, err := result.Dump.Raw(); !errors.Is(err, mifare.ErrIncompleteDump) {
		t.Fatalf("partial Raw() error = %v", err)
	}
}

func TestDumpStopsImmediatelyWhenCardChanges(t *testing.T) {
	card := classic1KCard(1)
	replacement := classic1KCard(9)
	key := nfc.Key{}
	selections := 0
	reader := openMockReader(t, mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) {
			selections++
			if selections >= 5 {
				return replacement, nil
			}
			return card, nil
		},
		Authenticate: func(context.Context, byte, nfc.KeyType, nfc.Key) error { return nil },
		ReadBlock:    func(context.Context, byte) ([nfc.BlockSize]byte, error) { return [nfc.BlockSize]byte{}, nil },
	})
	result, err := workflow.NewDumpService(directExecutor{reader: reader}).ReadCard(context.Background(), workflow.DumpRequest{
		Card: card, Keys: []workflow.SectorKeys{{Sector: 0, KeyA: &key}},
	})
	if !errors.Is(err, nfc.ErrCardChanged) {
		t.Fatalf("ReadCard() error = %v; want ErrCardChanged", err)
	}
	if result.Dump.Blocks[4].Status != mifare.BlockUnread {
		t.Fatalf("unvisited block status = %s", result.Dump.Blocks[4].Status)
	}
}

func TestDumpDoesNotTreatMaskedKeysAsZeros(t *testing.T) {
	card := classic1KCard(1)
	keyB := nfc.Key{1, 2, 3, 4, 5, 6}
	bits := mifare.AccessBits{}
	bits.Groups[3] = mifare.AccessCondition{C2: true, C3: true} // 011: Key B is secret.
	encoded := mifare.EncodeAccessBits(bits)
	reader := openMockReader(t, mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) { return card, nil },
		Authenticate: func(_ context.Context, _ byte, kind nfc.KeyType, key nfc.Key) error {
			if kind == nfc.KeyTypeB && key == keyB {
				return nil
			}
			return nfc.ErrAuthenticationFailed
		},
		ReadBlock: func(_ context.Context, block byte) ([nfc.BlockSize]byte, error) {
			var data [nfc.BlockSize]byte
			if block%4 == 3 {
				data[6], data[7], data[8] = encoded[0], encoded[1], encoded[2]
			}
			return data, nil
		},
	})
	result, err := workflow.NewDumpService(directExecutor{reader: reader}).ReadCard(context.Background(), workflow.DumpRequest{
		Card: card, Keys: []workflow.SectorKeys{{Sector: 0, KeyB: &keyB}},
	})
	if err != nil {
		t.Fatal(err)
	}
	trailer := result.Dump.Blocks[3]
	if trailer.Complete() || trailer.KnownMask&0x003f != 0 || trailer.KnownMask&0xfc00 != 0xfc00 {
		t.Fatalf("masked-key certainty = %04x", trailer.KnownMask)
	}
	if trailer.KeyBSource != "authenticated_key_b" {
		t.Fatalf("Key B source = %q", trailer.KeyBSource)
	}
}
