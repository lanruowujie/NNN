package app

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/workflow"
)

type uidPreflightRecord struct {
	request     workflow.UIDWriteRequest
	plan        workflow.UIDWritePlan
	fingerprint string
	expires     time.Time
}

func (s *Service) PreflightUIDWrite(value string) (UIDWritePreflightDTO, error) {
	s.mu.Lock()
	operationRunning := len(s.operations) != 0
	s.mu.Unlock()
	if operationRunning {
		return UIDWritePreflightDTO{}, ErrOperationRunning
	}
	newUID, err := parseFourByteUID(value)
	if err != nil {
		return UIDWritePreflightDTO{Error: err.Error()}, nil
	}
	snapshot := s.deviceManager.Snapshot()
	if snapshot.Card == nil {
		return UIDWritePreflightDTO{}, ErrCardRequired
	}
	card := snapshot.Card.Clone()
	if nfc.InferCardType(card) != nfc.CardTypeMIFAREClassic1K {
		return UIDWritePreflightDTO{Error: "当前只支持 4-byte UID 的 MIFARE Classic 1K 特殊卡"}, nil
	}
	sectorKeys := verifiedSectorKeys(s.keyStore, card, mifare.Classic1K)
	request := workflow.UIDWriteRequest{Card: card, Device: snapshot.Device, NewUID: newUID, Sector0Keys: sectorKeys[0]}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	plan, err := s.uidService.Preflight(ctx, request)
	if err != nil {
		return UIDWritePreflightDTO{Error: err.Error()}, nil
	}
	expected := plan.OldBlock0
	request.ExpectedBlock0 = &expected
	fingerprint := uidWriteFingerprint(request, plan)
	token := fmt.Sprintf("uid-%d-%x", s.nextID.Add(1), fingerprint[:8])
	record := uidPreflightRecord{request: request, plan: plan, fingerprint: fingerprint, expires: time.Now().Add(5 * time.Minute)}
	s.mu.Lock()
	if s.uidPreflights == nil {
		s.uidPreflights = make(map[string]uidPreflightRecord)
	}
	s.uidPreflights[token] = record
	s.mu.Unlock()
	preview := uidWritePreviewDTO(plan)
	return UIDWritePreflightDTO{
		Token: token, Valid: true, Preview: &preview,
		Warning: "支持 CUID/Gen2 普通写与 PN532 UART 上的 Gen1A 特殊卡；普通不可改 UID 卡会安全失败。",
	}, nil
}

func (s *Service) StartUIDWrite(input UIDWriteStartRequestDTO) (TaskDTO, error) {
	s.mu.Lock()
	record, exists := s.uidPreflights[input.Token]
	if exists {
		delete(s.uidPreflights, input.Token)
	}
	s.mu.Unlock()
	if !exists || time.Now().After(record.expires) {
		return TaskDTO{}, ErrPreflightRequired
	}
	confirmation, err := parseFourByteUID(input.Confirmation)
	if err != nil || !bytes.Equal(confirmation, record.request.NewUID) {
		return TaskDTO{}, ErrUIDConfirm
	}
	snapshot := s.deviceManager.Snapshot()
	if snapshot.Card == nil || !nfc.SameCard(record.request.Card, *snapshot.Card) {
		return TaskDTO{}, ErrPreflightRequired
	}
	if uidWriteFingerprint(record.request, record.plan) != record.fingerprint {
		return TaskDTO{}, ErrPreflightRequired
	}
	return s.startOperation("uid_write", "修改 UID", func(ctx context.Context, taskID string) error {
		request := record.request
		request.TaskID = taskID
		result, writeErr := s.uidService.Write(ctx, request, func(event workflow.UIDWriteEvent) {
			s.emitOperation(TaskEventDetailDTO{TaskID: taskID, Kind: "uid_write", Type: TaskEventProgress, Phase: string(event.Phase), Message: event.Message})
		})
		dto := uidWriteResultDTO(result)
		s.emitOperation(TaskEventDetailDTO{
			TaskID: taskID, Kind: "uid_write", Type: TaskEventProgress, Phase: string(workflow.UIDWriteVerify),
			Message: uidWriteResultMessage(result, writeErr), UID: &dto,
		})
		if writeErr == nil {
			s.registerUIDKey(result)
		}
		return writeErr
	})
}

func parseFourByteUID(value string) ([]byte, error) {
	compact := strings.Map(func(char rune) rune {
		if unicode.IsSpace(char) || char == ':' || char == '-' {
			return -1
		}
		return char
	}, value)
	if len(compact) != 8 {
		return nil, errorsNewUID()
	}
	decoded, err := hex.DecodeString(compact)
	if err != nil || len(decoded) != 4 {
		return nil, errorsNewUID()
	}
	return decoded, nil
}

func errorsNewUID() error {
	return nfc.NewError("parse UID", nfc.CodeInvalidArgument, "UID 必须是 8 位十六进制（4 字节）", nil)
}

func uidWriteFingerprint(request workflow.UIDWriteRequest, plan workflow.UIDWritePlan) string {
	return fmt.Sprintf("%s|%X|%X|%X", keys.CardID(request.Card), request.NewUID, plan.OldBlock0, plan.NewBlock0)
}

func uidWritePreviewDTO(plan workflow.UIDWritePlan) UIDWritePreviewDTO {
	return UIDWritePreviewDTO{
		CurrentUID: formatHex(plan.Card.UID), NewUID: formatHex(plan.NewBlock0[0:4]), BCC: fmt.Sprintf("%02X", plan.BCC),
		OldBlock0: formatHex(plan.OldBlock0[:]), NewBlock0: formatHex(plan.NewBlock0[:]),
	}
}

func uidWriteResultDTO(result workflow.UIDWriteResult) UIDWriteResultDTO {
	dto := UIDWriteResultDTO{
		OldUID: formatHex(result.OldCard.UID), OldBlock0: formatHex(result.OldBlock0[:]),
		NewBlock0: formatHex(result.NewBlock0[:]), BackupPath: result.BackupPath, Verified: result.Verified,
	}
	if len(result.NewCard.UID) != 0 {
		dto.NewUID = formatHex(result.NewCard.UID)
	}
	if result.ActualRead {
		dto.ActualBlock0 = formatHex(result.ActualBlock0[:])
	}
	return dto
}

func uidWriteResultMessage(result workflow.UIDWriteResult, err error) string {
	if err == nil {
		return fmt.Sprintf("UID 已修改为 %s；原 block 0 备份：%s", formatHex(result.NewCard.UID), result.BackupPath)
	}
	if result.BackupPath != "" {
		return fmt.Sprintf("UID 写入或验证失败；原 block 0 已备份到 %s", result.BackupPath)
	}
	return "UID 写入前检查失败，卡片未被修改"
}

func (s *Service) registerUIDKey(result workflow.UIDWriteResult) {
	if !result.Verified || s.keyStore == nil {
		return
	}
	oldCardID, newCardID := keys.CardID(result.OldCard), keys.CardID(result.NewCard)
	for _, verification := range s.keyStore.Verified(oldCardID) {
		record, exists := s.keyStore.Record(verification.KeyID)
		if !exists {
			continue
		}
		_, _ = s.keyStore.MergeVerified(newCardID, verification.Sector, verification.KeyType, record.Value, keys.SourceReadContext)
	}
	_, _ = s.keyStore.MergeVerified(newCardID, 0, result.KeyType, result.Key, keys.SourceReadContext)
	_ = s.persistKeys()
	s.emitter.Emit(KeyEventName, sectorKeyDTOs(s.keyStore, &result.NewCard))
}
