package mifare

import "fmt"

const (
	BlockSize = 16

	Classic1KSectors  = 16
	Classic1KBlocks   = 64
	Classic1KDumpSize = Classic1KBlocks * BlockSize

	Classic4KSectors  = 40
	Classic4KBlocks   = 256
	Classic4KDumpSize = Classic4KBlocks * BlockSize
)

// Layout identifies one supported MIFARE Classic memory geometry.
type Layout uint8

const (
	Classic1K Layout = iota + 1
	Classic4K
)

// Valid reports whether the layout is supported by the first NFCX release.
func (l Layout) Valid() bool { return l == Classic1K || l == Classic4K }

// SectorCount returns the number of sectors in the layout.
func (l Layout) SectorCount() (int, error) {
	switch l {
	case Classic1K:
		return Classic1KSectors, nil
	case Classic4K:
		return Classic4KSectors, nil
	default:
		return 0, invalidLayout(l)
	}
}

// BlockCount returns the number of addressable 16-byte blocks in the layout.
func (l Layout) BlockCount() (int, error) {
	switch l {
	case Classic1K:
		return Classic1KBlocks, nil
	case Classic4K:
		return Classic4KBlocks, nil
	default:
		return 0, invalidLayout(l)
	}
}

// DumpSize returns the raw dump size in bytes.
func (l Layout) DumpSize() (int, error) {
	blocks, err := l.BlockCount()
	if err != nil {
		return 0, err
	}
	return blocks * BlockSize, nil
}

// SectorForBlock returns the sector containing block.
func (l Layout) SectorForBlock(block int) (int, error) {
	if err := l.validateBlock(block); err != nil {
		return 0, err
	}
	if block < 128 {
		return block / 4, nil
	}
	return 32 + (block-128)/16, nil
}

// FirstBlock returns the first block in sector.
func (l Layout) FirstBlock(sector int) (int, error) {
	if err := l.validateSector(sector); err != nil {
		return 0, err
	}
	if sector < 32 {
		return sector * 4, nil
	}
	return 128 + (sector-32)*16, nil
}

// BlocksInSector returns the number of blocks in sector.
func (l Layout) BlocksInSector(sector int) (int, error) {
	if err := l.validateSector(sector); err != nil {
		return 0, err
	}
	if sector < 32 {
		return 4, nil
	}
	return 16, nil
}

// TrailerBlock returns the final, access-control block in sector.
func (l Layout) TrailerBlock(sector int) (int, error) {
	first, err := l.FirstBlock(sector)
	if err != nil {
		return 0, err
	}
	count, _ := l.BlocksInSector(sector)
	return first + count - 1, nil
}

// IsTrailer reports whether block is a sector trailer.
func (l Layout) IsTrailer(block int) (bool, error) {
	sector, err := l.SectorForBlock(block)
	if err != nil {
		return false, err
	}
	trailer, _ := l.TrailerBlock(sector)
	return block == trailer, nil
}

func (l Layout) validateBlock(block int) error {
	count, err := l.BlockCount()
	if err != nil {
		return err
	}
	if block < 0 || block >= count {
		return fmt.Errorf("MIFARE Classic block %d is outside layout with %d blocks", block, count)
	}
	return nil
}

func (l Layout) validateSector(sector int) error {
	count, err := l.SectorCount()
	if err != nil {
		return err
	}
	if sector < 0 || sector >= count {
		return fmt.Errorf("MIFARE Classic sector %d is outside layout with %d sectors", sector, count)
	}
	return nil
}

func invalidLayout(layout Layout) error {
	return fmt.Errorf("unsupported MIFARE Classic layout %d", layout)
}
