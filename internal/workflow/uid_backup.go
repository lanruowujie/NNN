package workflow

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileUIDBackupStore struct {
	Root string
}

type uidBackupFile struct {
	Version   int    `json:"version"`
	Method    string `json:"method"`
	TaskID    string `json:"taskId,omitempty"`
	CreatedAt string `json:"createdAt"`
	UID       string `json:"uid"`
	ATQA      string `json:"atqa"`
	SAK       string `json:"sak"`
	Device    string `json:"device"`
	OldBlock0 string `json:"oldBlock0"`
	NewBlock0 string `json:"newBlock0"`
}

func (s FileUIDBackupStore) Save(ctx context.Context, backup UIDBackup) (string, error) {
	if ctx == nil || strings.TrimSpace(s.Root) == "" {
		return "", errors.New("UID backup directory is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(s.Root, 0o700); err != nil {
		return "", err
	}
	createdAt := backup.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	record := uidBackupFile{
		Version: 1, Method: backup.Method, TaskID: backup.TaskID, CreatedAt: createdAt.Format(time.RFC3339Nano),
		UID:  strings.ToUpper(hex.EncodeToString(backup.Card.UID)),
		ATQA: strings.ToUpper(hex.EncodeToString(backup.Card.ATQA[:])), SAK: fmt.Sprintf("%02X", backup.Card.SAK),
		Device: backup.Device.ConnString, OldBlock0: strings.ToUpper(hex.EncodeToString(backup.OldBlock0[:])),
		NewBlock0: strings.ToUpper(hex.EncodeToString(backup.NewBlock0[:])),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(s.Root, ".uid-backup-*.tmp")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return "", err
	}
	if _, err := temporary.Write(data); err != nil {
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	unique := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(temporaryPath), ".uid-backup-"), ".tmp")
	name := fmt.Sprintf("%s-%s-%s.json", createdAt.Format("20060102T150405.000000000Z"), safeBackupName(backup.TaskID), unique)
	path := filepath.Join(s.Root, name)
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", err
	}
	keep = true
	return path, nil
}

func safeBackupName(value string) string {
	var output strings.Builder
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			output.WriteRune(char)
		}
		if output.Len() >= 48 {
			break
		}
	}
	if output.Len() == 0 {
		return "task"
	}
	return output.String()
}
