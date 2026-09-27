//go:build windows

package terminal

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const createNewConsole = 0x00000010

// Open starts a separate PowerShell console. The generated script is limited
// to NFCX-controlled environment values and removes itself before handing the
// interactive prompt to the user.
func Open(env Environment) error {
	if err := env.validate(); err != nil {
		return err
	}
	runtimeDir, err := filepath.Abs(env.RuntimeDir)
	if err != nil {
		return fmt.Errorf("resolve runtime directory: %w", err)
	}
	file, err := os.CreateTemp("", "nfcx-terminal-*.ps1")
	if err != nil {
		return fmt.Errorf("create terminal setup script: %w", err)
	}
	path := file.Name()
	cleanup := func(cause error) error { _ = file.Close(); _ = os.Remove(path); return cause }
	content := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$env:PATH = %s + [IO.Path]::PathSeparator + $env:PATH
$env:LIBNFC_DEVICE = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String(%s))
Set-Location -LiteralPath %s
Remove-Item -LiteralPath $PSCommandPath -Force
Write-Host ''
Write-Host 'NFCX command-line environment'
Write-Host ('Runtime: ' + (Get-Location))
if ($env:LIBNFC_DEVICE) { Write-Host ('Reader: ' + $env:LIBNFC_DEVICE) } else { Write-Host 'Reader: not selected (choose one in NFCX, then reopen this terminal)' }
Write-Host ''
Write-Host 'Common commands (run only against cards/systems you own or are authorized to test):'
Write-Host '  # Read/recover with a known Classic key and save a dump:'
Write-Host '  mfoc -k FFFFFFFFFFFF -O card.mfd'
Write-Host '  # Supply more than one known key when available:'
Write-Host '  mfoc -k FFFFFFFFFFFF -k A0A1A2A3A4A5 -O card.mfd'
Write-Host ''
Write-Host '  # Darkside attempt for sector 0, Key A:'
Write-Host '  mfcuk -C -R 0:A -s 250 -S 250 -v 2'
Write-Host ''
Write-Host '  # Hardnested recovery when a seed key is known:'
Write-Host '  mfoc-hardnested -C -F -k FFFFFFFFFFFF -O hardnested.mfd'
Write-Host ''
Write-Host '  # Inspect command options:'
Write-Host '  mfoc -h'
Write-Host '  mfcuk -h'
Write-Host '  mfoc-hardnested -h'
Write-Host '  nfc-mfsetuid -h'
Write-Host ''
Write-Host '  # DANGEROUS: nfc-mfsetuid writes a full 16-byte block 0 to compatible cards.'
Write-Host '  # Inspect its help and make a backup before considering it.'
Write-Host ''
Write-Host 'The PATH change applies only to this terminal. Close it, then reconnect the reader in NFCX.'
`, powershellQuote(runtimeDir), powershellQuote(base64.StdEncoding.EncodeToString([]byte(env.Device))), powershellQuote(runtimeDir))
	if _, err := file.WriteString(content); err != nil {
		return cleanup(fmt.Errorf("write terminal setup script: %w", err))
	}
	if err := file.Close(); err != nil {
		return cleanup(fmt.Errorf("close terminal setup script: %w", err))
	}
	command := exec.Command("powershell.exe", "-NoLogo", "-NoExit", "-ExecutionPolicy", "Bypass", "-File", path)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	if err := command.Start(); err != nil {
		return cleanup(fmt.Errorf("open PowerShell: %w", err))
	}
	return nil
}

func powershellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
