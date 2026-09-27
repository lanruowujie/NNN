// Package terminal opens a user-controlled terminal with NFCX's private
// command-line runtime available for that terminal session only.
package terminal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrUnsupported = errors.New("opening a command terminal is not supported on this platform")

// Environment contains only application-controlled runtime data. Command is
// deliberately absent: NFCX opens a shell for the user, rather than executing
// arbitrary GUI-supplied commands.
type Environment struct {
	RuntimeDir string
	Device     string
}

func (e Environment) validate() error {
	if strings.TrimSpace(e.RuntimeDir) == "" {
		return errors.New("NFCX runtime directory is required")
	}
	absolute, err := filepath.Abs(e.RuntimeDir)
	if err != nil {
		return fmt.Errorf("resolve NFCX runtime directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return fmt.Errorf("inspect NFCX runtime directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("NFCX runtime path is not a directory")
	}
	return nil
}
