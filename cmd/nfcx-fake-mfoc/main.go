// nfcx-fake-mfoc is a test fixture and must never be shipped.
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	hardnested := strings.Contains(strings.ToLower(filepath.Base(os.Args[0])), "hardnested")
	for _, arg := range os.Args[1:] {
		if arg == "-h" {
			if hardnested {
				fmt.Println("Usage: mfoc-hardnested [-h] [-C] [-F] [-k key] [-O output]")
				fmt.Println("This is mfoc-hardnested version 0.10.9.")
				return
			}
			fmt.Println("This is mfoc version 0.10.7.")
			fmt.Println("Usage: mfoc [-h] [-k key]... [-O output]")
			return
		}
	}
	knownKeys, outputPath := options("-k"), option("-O")
	if len(knownKeys) == 0 || outputPath == "" {
		fmt.Fprintln(os.Stderr, "required -k/-O option is missing")
		os.Exit(2)
	}
	for _, known := range knownKeys {
		decoded, err := hex.DecodeString(known)
		if err != nil || len(decoded) != 6 {
			fmt.Fprintln(os.Stderr, "invalid known key")
			os.Exit(3)
		}
	}
	fmt.Printf("The custom key 0x%s has been added\n", knownKeys[0])
	mode := os.Getenv("NFCX_FAKE_MFOC_MODE")
	if hardnested && os.Getenv("NFCX_FAKE_HARDNESTED_MODE") != "" {
		mode = os.Getenv("NFCX_FAKE_HARDNESTED_MODE")
	}
	switch mode {
	case "nonzero":
		fmt.Fprintln(os.Stderr, "requested mfoc failure")
		os.Exit(19)
	case "hang":
		for {
			time.Sleep(time.Second)
		}
	case "missing":
		return
	case "corrupt":
		_ = os.WriteFile(outputPath, []byte("corrupt"), 0o600)
		return
	}
	cardSize := 1024
	if value, err := strconv.Atoi(os.Getenv("NFCX_FAKE_MFOC_SIZE")); err == nil && value > 0 {
		cardSize = value
	}
	// Upstream 0.10.7 always writes its 4096-byte mifare_classic_tag struct,
	// leaving the unused tail zeroed for a 1K card.
	raw := make([]byte, 4096)
	uid, _ := hex.DecodeString(os.Getenv("NFCX_FAKE_MFOC_UID"))
	if len(uid) != 4 {
		uid = []byte{1, 2, 3, 4}
	}
	copy(raw[:4], uid)
	raw[4] = uid[0] ^ uid[1] ^ uid[2] ^ uid[3]
	sectors := 16
	if cardSize == 4096 {
		sectors = 40
	}
	for sector := 0; sector < sectors; sector++ {
		trailer := sector*4 + 3
		if sector >= 32 {
			trailer = 128 + (sector-32)*16 + 15
		}
		offset := trailer * 16
		for index := 0; index < 6; index++ {
			raw[offset+index] = byte(sector + 1)
			raw[offset+10+index] = byte(0xa0 + sector)
		}
		copy(raw[offset+6:offset+10], []byte{0xff, 0x07, 0x80, 0x69})
		fmt.Printf("Sector: %d, type A, probe 0\n", sector)
	}
	if mode == "dirty-padding" && cardSize == 1024 {
		raw[1024] = 0xff
	}
	if err := os.WriteFile(outputPath, raw, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
}

func option(name string) string {
	for index := 1; index+1 < len(os.Args); index++ {
		if os.Args[index] == name {
			return os.Args[index+1]
		}
	}
	return ""
}

func options(name string) []string {
	var result []string
	for index := 1; index+1 < len(os.Args); index++ {
		if os.Args[index] == name {
			result = append(result, os.Args[index+1])
			index++
		}
	}
	return result
}
