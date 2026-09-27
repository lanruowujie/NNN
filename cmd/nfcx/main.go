package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/BennyThink/NFCX/internal/desktop"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := desktop.Execute(assets, os.Args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
