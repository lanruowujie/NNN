// nfcx-fake-engine is a development-only subprocess fixture. It is built into
// runtime/<platform> explicitly and is not part of release bundles.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	mode := flag.String("mode", "success", "fixture mode")
	output := flag.String("output", "", "result path")
	echo := flag.String("echo", "", "argument round trip")
	steps := flag.Int("steps", 0, "number of progress steps")
	interval := flag.Duration("interval", 0, "delay between progress steps")
	version := flag.Bool("version", false, "print version")
	flag.Parse()
	if *version {
		fmt.Println("nfcx-fake-engine 1.0")
		return
	}
	if *mode == "child" {
		for {
			time.Sleep(time.Second)
		}
	}
	fmt.Printf("fixture started: %s\n", *echo)
	fmt.Fprintln(os.Stderr, "fixture stderr is live")
	for step := 1; step <= *steps; step++ {
		fmt.Printf("fixture progress %d/%d\n", step, *steps)
		if *interval > 0 {
			time.Sleep(*interval)
		}
	}
	switch *mode {
	case "hang":
		child := exec.CommandContext(context.Background(), os.Args[0], "--mode", "child")
		if err := child.Start(); err == nil {
			fmt.Printf("child_pid=%d\n", child.Process.Pid)
		}
		for {
			time.Sleep(time.Second)
		}
	case "fail":
		fmt.Fprintln(os.Stderr, "fixture requested failure")
		os.Exit(17)
	case "invalid":
		if *output != "" {
			_ = os.WriteFile(*output, []byte("not-json"), 0o600)
		}
		return
	case "missing":
		return
	case "spam":
		for index := 0; index < 4096; index++ {
			fmt.Printf("stdout-%04d-%s", index, *echo)
			fmt.Fprintf(os.Stderr, "stderr-%04d-%s", index, *echo)
		}
	}
	payload, _ := json.Marshal(map[string]any{"ok": true, "device": os.Getenv("LIBNFC_DEVICE"), "echo": *echo})
	if *output == "" || os.WriteFile(*output, payload, 0o600) != nil {
		fmt.Fprintln(os.Stderr, "unable to write fixture result")
		os.Exit(18)
	}
	fmt.Println("fixture completed")
}
