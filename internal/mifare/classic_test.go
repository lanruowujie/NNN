package mifare_test

import (
	"testing"

	"github.com/BennyThink/NFCX/internal/mifare"
)

func TestClassicLayoutSizes(t *testing.T) {
	for _, test := range []struct {
		layout  mifare.Layout
		sectors int
		blocks  int
		dump    int
	}{
		{mifare.Classic1K, 16, 64, 1024},
		{mifare.Classic4K, 40, 256, 4096},
	} {
		sectors, err := test.layout.SectorCount()
		if err != nil || sectors != test.sectors {
			t.Errorf("SectorCount() = %d, %v; want %d", sectors, err, test.sectors)
		}
		blocks, err := test.layout.BlockCount()
		if err != nil || blocks != test.blocks {
			t.Errorf("BlockCount() = %d, %v; want %d", blocks, err, test.blocks)
		}
		dump, err := test.layout.DumpSize()
		if err != nil || dump != test.dump {
			t.Errorf("DumpSize() = %d, %v; want %d", dump, err, test.dump)
		}
	}
}

func TestEveryClassicBlockMapsBackToItsSector(t *testing.T) {
	for _, layout := range []mifare.Layout{mifare.Classic1K, mifare.Classic4K} {
		sectorCount, _ := layout.SectorCount()
		visited := 0
		for sector := 0; sector < sectorCount; sector++ {
			first, err := layout.FirstBlock(sector)
			if err != nil {
				t.Fatal(err)
			}
			count, _ := layout.BlocksInSector(sector)
			trailer, _ := layout.TrailerBlock(sector)
			if trailer != first+count-1 {
				t.Fatalf("layout %d sector %d trailer = %d; want %d", layout, sector, trailer, first+count-1)
			}
			for block := first; block <= trailer; block++ {
				got, err := layout.SectorForBlock(block)
				if err != nil || got != sector {
					t.Fatalf("layout %d block %d sector = %d, %v; want %d", layout, block, got, err, sector)
				}
				isTrailer, err := layout.IsTrailer(block)
				if err != nil || isTrailer != (block == trailer) {
					t.Fatalf("layout %d block %d IsTrailer = %v, %v", layout, block, isTrailer, err)
				}
				visited++
			}
		}
		blockCount, _ := layout.BlockCount()
		if visited != blockCount {
			t.Fatalf("layout %d visited %d blocks; want %d", layout, visited, blockCount)
		}
	}
}

func TestClassic4KLargeSectorBoundaries(t *testing.T) {
	for _, test := range []struct {
		sector, first, count, trailer int
	}{
		{31, 124, 4, 127},
		{32, 128, 16, 143},
		{39, 240, 16, 255},
	} {
		first, _ := mifare.Classic4K.FirstBlock(test.sector)
		count, _ := mifare.Classic4K.BlocksInSector(test.sector)
		trailer, _ := mifare.Classic4K.TrailerBlock(test.sector)
		if first != test.first || count != test.count || trailer != test.trailer {
			t.Errorf("sector %d = first %d count %d trailer %d; want %d %d %d", test.sector, first, count, trailer, test.first, test.count, test.trailer)
		}
	}
}

func TestClassicGeometryRejectsInvalidInputs(t *testing.T) {
	for _, layout := range []mifare.Layout{mifare.Classic1K, mifare.Classic4K} {
		blocks, _ := layout.BlockCount()
		sectors, _ := layout.SectorCount()
		for _, block := range []int{-1, blocks} {
			if _, err := layout.SectorForBlock(block); err == nil {
				t.Errorf("layout %d accepted block %d", layout, block)
			}
		}
		for _, sector := range []int{-1, sectors} {
			if _, err := layout.FirstBlock(sector); err == nil {
				t.Errorf("layout %d accepted sector %d", layout, sector)
			}
		}
	}
	if _, err := mifare.Layout(99).BlockCount(); err == nil {
		t.Fatal("unknown layout was accepted")
	}
}
