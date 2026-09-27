package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/BennyThink/NFCX/internal/update"
)

func main() {
	transactionPath := flag.String("transaction", "", "private NFCX update transaction")
	flag.Parse()
	if *transactionPath == "" {
		fmt.Fprintln(os.Stderr, "transaction is required")
		os.Exit(2)
	}
	transaction, err := update.ReadTransaction(*transactionPath)
	if err == nil {
		err = update.Apply(transaction)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "NFCX update:", err)
		os.Exit(1)
	}
}
