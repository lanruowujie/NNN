// nfcx-fake-mfcuk is a test fixture and must never be shipped.
package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "-h" {
			fmt.Println("mfcuk - 0.3.8")
			fmt.Println("Mifare Classic DarkSide Key Recovery Tool - 0.3")
			fmt.Println("NFCX machine result: 1")
			fmt.Println("NFCX_VERSION 0.3.8")
			fmt.Println("-R sector[:A/B/any_other_alphanum] - recover key for sector")
			os.Exit(1)
		}
	}
	fmt.Println("mfcuk - 0.3.8")
	mode := os.Getenv("NFCX_FAKE_MFCUK_MODE")
	switch mode {
	case "nonzero":
		fmt.Fprintln(os.Stderr, "requested mfcuk failure")
		os.Exit(23)
	case "hang":
		for {
			time.Sleep(time.Second)
		}
	case "missing":
		return
	case "multiple":
		fmt.Println("NFCX_RESULT key=B sector=3 value=A0A1A2A3A4A5")
		fmt.Println("NFCX_RESULT key=A sector=0 value=FFFFFFFFFFFF")
		fmt.Println("NFCX_RESULT key=A sector=0 value=FFFFFFFFFFFF")
		return
	}
	fmt.Println("NFCX_RESULT key=A sector=0 value=FFFFFFFFFFFF")
}
