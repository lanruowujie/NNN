//go:build !windows

package terminal

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writeUnixScript(env Environment) (string, error) {
	shell, err := defaultUnixShell()
	if err != nil {
		return "", err
	}
	return writeUnixScriptWithShell(env, shell)
}

func writeUnixScriptWithShell(env Environment, shell string) (string, error) {
	file, err := os.CreateTemp("", "nfcx-terminal-*.sh")
	if err != nil {
		return "", err
	}
	path := file.Name()
	cleanup := func(cause error) (string, error) { _ = file.Close(); _ = os.Remove(path); return "", cause }
	if err := file.Chmod(0o700); err != nil {
		return cleanup(fmt.Errorf("make terminal setup script executable: %w", err))
	}
	runtimeDir, err := filepath.Abs(env.RuntimeDir)
	if err != nil {
		return cleanup(fmt.Errorf("resolve runtime directory: %w", err))
	}
	device := base64.StdEncoding.EncodeToString([]byte(env.Device))
	content := fmt.Sprintf(`#!/bin/sh
export PATH=%s:"$PATH"
export LIBNFC_DEVICE="$(printf %%s %s | base64 -d 2>/dev/null || printf %%s %s | base64 -D)"
cd %s || exit 1
rm -f -- "$0"
printf '\nNFCX command-line environment\n'
printf 'Runtime: %%s\n' %s
if [ -n "$LIBNFC_DEVICE" ]; then printf 'Reader: %%s\n' "$LIBNFC_DEVICE"; else printf 'Reader: not selected (choose one in NFCX, then reopen this terminal)\n'; fi
cat <<'NFCX_HELP'

Common commands (run only against cards/systems you own or are authorized to test):
  # Read/recover with a known Classic key and save a dump:
  mfoc -k FFFFFFFFFFFF -O card.mfd
  # Supply more than one known key when available:
  mfoc -k FFFFFFFFFFFF -k A0A1A2A3A4A5 -O card.mfd

  # Darkside attempt for sector 0, Key A:
  mfcuk -C -R 0:A -s 250 -S 250 -v 2

  # Hardnested recovery when a seed key is known:
  mfoc-hardnested -C -F -k FFFFFFFFFFFF -O hardnested.mfd

  # Inspect command options:
  mfoc -h
  mfcuk -h
  mfoc-hardnested -h
  nfc-mfsetuid -h

  # DANGEROUS: nfc-mfsetuid writes a full 16-byte block 0 to compatible cards.
  # Inspect its help and make a backup before considering it.

The PATH change applies only to this terminal. Close it, then reconnect the reader in NFCX.

NFCX_HELP
exec %s -i
`, shellQuote(runtimeDir), shellQuote(device), shellQuote(device), shellQuote(runtimeDir), shellQuote(runtimeDir), shellQuote(shell))
	if _, err := file.WriteString(content); err != nil {
		return cleanup(fmt.Errorf("write terminal setup script: %w", err))
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close terminal setup script: %w", err)
	}
	return path, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
