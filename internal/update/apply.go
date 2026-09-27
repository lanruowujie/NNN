package update

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// StartApply launches the packaged helper with a private transaction. The
// helper is deliberately adjacent to the application binary, not searched on PATH.
func (m *Manager) StartApply() error {
	m.mu.Lock()
	snapshot, staging := m.snapshot, m.config.StagingDir
	m.mu.Unlock()
	if snapshot.State != StateReady || !snapshot.CanApply {
		return errors.New("no automatically applicable update is ready")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	helper := filepath.Join(filepath.Dir(executable), "NFCX Updater")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	if _, err := os.Stat(helper); err != nil {
		return fmt.Errorf("NFCX Updater is unavailable: %w", err)
	}
	transactionPath := filepath.Join(staging, "apply.json")
	if err := WriteTransaction(transactionPath, Transaction{Version: 1, PID: os.Getpid(), Format: snapshot.Asset.Format, Source: snapshot.PreparedPath, Destination: executable, Restart: executable}); err != nil {
		return err
	}
	return exec.Command(helper, "-transaction", transactionPath).Start()
}

// Transaction is written by NFCX after an asset has passed SHA-256 validation.
// The helper receives only this path, never arbitrary source/destination flags.
type Transaction struct {
	Version     int    `json:"version"`
	PID         int    `json:"pid"`
	Format      string `json:"format"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Restart     string `json:"restart"`
}

func WriteTransaction(path string, transaction Transaction) error {
	if transaction.Version != 1 || transaction.PID < 1 || !filepath.IsAbs(transaction.Source) || !filepath.IsAbs(transaction.Destination) || !filepath.IsAbs(transaction.Restart) {
		return errors.New("invalid update transaction")
	}
	data, err := json.Marshal(transaction)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func ReadTransaction(path string) (Transaction, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Transaction{}, err
	}
	var transaction Transaction
	if err := json.Unmarshal(data, &transaction); err != nil {
		return Transaction{}, err
	}
	if transaction.Version != 1 || transaction.PID < 1 || !filepath.IsAbs(transaction.Source) || !filepath.IsAbs(transaction.Destination) || !filepath.IsAbs(transaction.Restart) {
		return Transaction{}, errors.New("invalid update transaction")
	}
	return transaction, nil
}

// Apply waits for the parent to exit, replaces only the declared installation,
// and restarts it. It intentionally supports only release formats NFCX emits.
func Apply(transaction Transaction) error {
	// The parent launches us immediately before its orderly Wails shutdown. A
	// short grace period works across supported OSes without sending signals.
	time.Sleep(3 * time.Second)
	if _, err := os.Stat(transaction.Source); err != nil {
		return fmt.Errorf("verified update asset is unavailable: %w", err)
	}
	switch transaction.Format {
	case "appimage":
		if err := replaceFile(transaction.Source, transaction.Destination); err != nil {
			return err
		}
	case "zip":
		if err := replaceZipDirectory(transaction.Source, filepath.Dir(transaction.Destination)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported automatic update format %q", transaction.Format)
	}
	return exec.Command(transaction.Restart).Start()
}

func replaceFile(source, destination string) error {
	backup := destination + ".nfcx-backup"
	_ = os.Remove(backup)
	if err := os.Rename(destination, backup); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err != nil {
		_ = os.Rename(backup, destination)
		return err
	}
	if err := os.Chmod(destination, 0755); err != nil {
		return err
	}
	return nil
}

func replaceZipDirectory(source, destination string) error {
	staging, err := os.MkdirTemp(filepath.Dir(destination), ".nfcx-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := unzip(source, staging); err != nil {
		return err
	}
	backup := destination + ".nfcx-backup"
	_ = os.RemoveAll(backup)
	if err := os.Rename(destination, backup); err != nil {
		return err
	}
	if err := os.Rename(staging, destination); err != nil {
		_ = os.Rename(backup, destination)
		return err
	}
	return nil
}

func unzip(source, destination string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer archive.Close()
	for _, entry := range archive.File {
		target := filepath.Join(destination, entry.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), filepath.Clean(destination)+string(os.PathSeparator)) {
			return errors.New("update archive contains an unsafe path")
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, entry.Mode())
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		_ = input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
