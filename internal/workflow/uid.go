package workflow

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
)

type UIDWritePhase string

const (
	UIDWritePreflight UIDWritePhase = "preflight"
	UIDWriteBackup    UIDWritePhase = "backup"
	UIDWriteExecute   UIDWritePhase = "execute"
	UIDWriteReselect  UIDWritePhase = "reselect"
	UIDWriteFallback  UIDWritePhase = "gen1a_fallback"
	UIDWriteVerify    UIDWritePhase = "verify"
	UIDWriteComplete  UIDWritePhase = "complete"
)

type UIDWriteEvent struct {
	Phase   UIDWritePhase
	Message string
}

// UIDWriteRequest carries the shared safety facts for the ordinary CUID/Gen2
// attempt and the optional external Gen1A fallback. Irreversible magic-card
// configuration commands are deliberately absent.
type UIDWriteRequest struct {
	TaskID         string
	Card           nfc.CardInfo
	Device         nfc.DeviceInfo
	NewUID         []byte
	Sector0Keys    SectorKeys
	ExpectedBlock0 *[nfc.BlockSize]byte
}

type UIDWritePlan struct {
	Card      nfc.CardInfo
	OldBlock0 [nfc.BlockSize]byte
	NewBlock0 [nfc.BlockSize]byte
	BCC       byte
	KeyType   nfc.KeyType
	Key       nfc.Key
}

type UIDWriteResult struct {
	OldCard      nfc.CardInfo
	NewCard      nfc.CardInfo
	OldBlock0    [nfc.BlockSize]byte
	NewBlock0    [nfc.BlockSize]byte
	ActualBlock0 [nfc.BlockSize]byte
	ActualRead   bool
	BackupPath   string
	KeyType      nfc.KeyType
	Key          nfc.Key
	Verified     bool
}

type UIDBackup struct {
	TaskID    string
	CreatedAt time.Time
	Card      nfc.CardInfo
	Device    nfc.DeviceInfo
	OldBlock0 [nfc.BlockSize]byte
	NewBlock0 [nfc.BlockSize]byte
	Method    string
}

type UIDBackupStore interface {
	Save(context.Context, UIDBackup) (string, error)
}

type UIDService struct {
	devices       ReaderExecutor
	backups       UIDBackupStore
	fallback      UIDFallbackWriter
	reselectLimit time.Duration
	retryDelay    time.Duration
	now           func() time.Time
}

func NewUIDService(devices ReaderExecutor, backups UIDBackupStore, fallback ...UIDFallbackWriter) *UIDService {
	service := &UIDService{
		devices: devices, backups: backups, reselectLimit: 8 * time.Second,
		retryDelay: 200 * time.Millisecond, now: func() time.Time { return time.Now().UTC() },
	}
	if len(fallback) != 0 {
		service.fallback = fallback[0]
	}
	return service
}

// Preflight is read-only. It proves that the current card is a 4-byte Classic
// 1K target, that block 0 can be authenticated and read, and that a verified
// key is permitted by sector 0's current access conditions.
func (s *UIDService) Preflight(ctx context.Context, request UIDWriteRequest) (UIDWritePlan, error) {
	request.Card = request.Card.Clone()
	request.NewUID = append([]byte(nil), request.NewUID...)
	if err := validateUIDWriteRequest(request); err != nil {
		return UIDWritePlan{}, err
	}
	if s == nil || s.devices == nil {
		return UIDWritePlan{}, nfc.NewError("UID write preflight", nfc.CodeNotOpen, "device manager is unavailable", nil)
	}
	var plan UIDWritePlan
	err := s.devices.WithReader(ctx, func(operationCtx context.Context, reader nfc.Reader) error {
		var err error
		plan, err = buildUIDWritePlan(operationCtx, reader, request)
		return err
	})
	return plan, err
}

// Write repeats preflight under one exclusive lease, persists the original
// block 0, performs the special write, then reselects and verifies the card.
func (s *UIDService) Write(ctx context.Context, request UIDWriteRequest, emit func(UIDWriteEvent)) (UIDWriteResult, error) {
	request.Card = request.Card.Clone()
	request.NewUID = append([]byte(nil), request.NewUID...)
	result := UIDWriteResult{OldCard: request.Card.Clone()}
	if request.ExpectedBlock0 != nil && len(request.NewUID) == 4 {
		result.OldBlock0 = *request.ExpectedBlock0
		result.NewBlock0 = *request.ExpectedBlock0
		copy(result.NewBlock0[0:4], request.NewUID)
		result.NewBlock0[4] = request.NewUID[0] ^ request.NewUID[1] ^ request.NewUID[2] ^ request.NewUID[3]
	}
	if err := validateUIDWriteRequest(request); err != nil {
		return result, err
	}
	if s == nil || s.devices == nil || s.backups == nil {
		return result, nfc.NewError("write UID", nfc.CodeNotOpen, "UID writer or backup storage is unavailable", nil)
	}
	if emit == nil {
		emit = func(UIDWriteEvent) {}
	}
	emit(UIDWriteEvent{Phase: UIDWritePreflight, Message: "正在重新确认卡片与 block 0"})

	var ordinaryErr error
	err := s.devices.WithReader(ctx, func(operationCtx context.Context, reader nfc.Reader) error {
		plan, err := buildUIDWritePlan(operationCtx, reader, request)
		if err != nil {
			return err
		}
		result.OldBlock0, result.NewBlock0 = plan.OldBlock0, plan.NewBlock0
		result.KeyType, result.Key = plan.KeyType, plan.Key

		emit(UIDWriteEvent{Phase: UIDWriteBackup, Message: "正在保存原始 block 0"})
		now := time.Now().UTC()
		if s.now != nil {
			now = s.now()
		}
		backupPath, err := s.backups.Save(operationCtx, UIDBackup{
			TaskID: request.TaskID, CreatedAt: now, Card: request.Card.Clone(), Device: request.Device,
			OldBlock0: plan.OldBlock0, NewBlock0: plan.NewBlock0, Method: "special_uid_auto",
		})
		if err != nil {
			return nfc.NewError("backup block 0", nfc.CodeIO, "无法保存原始 block 0，已禁止写入 UID", err)
		}
		result.BackupPath = backupPath

		writer, ok := reader.(nfc.ClassicManufacturerWriter)
		if !ok {
			return nfc.NewError("write UID", nfc.CodeUnsupported, "reader does not expose the protected manufacturer-block capability", nil)
		}
		if err := selectExpectedCard(operationCtx, reader, plan.Card); err != nil {
			return err
		}
		if err := reader.Authenticate(operationCtx, 0, plan.KeyType, plan.Key); err != nil {
			return err
		}
		emit(UIDWriteEvent{Phase: UIDWriteExecute, Message: "正在写入 UID 与 BCC"})
		writeErr := writer.WriteManufacturerBlock(operationCtx, plan.NewBlock0)
		if writeErr != nil && !uidWriteMayHaveCommitted(writeErr) {
			return writeErr
		}
		if writeErr != nil {
			emit(UIDWriteEvent{Phase: UIDWriteReselect, Message: "写入响应不确定，正在重新寻卡验证实际结果"})
		} else {
			emit(UIDWriteEvent{Phase: UIDWriteReselect, Message: "正在重新寻卡并确认新 UID"})
		}

		newCard, err := s.waitForNewUID(operationCtx, plan.Card, request.NewUID, reader, s.fallback != nil)
		if err != nil {
			if s.fallback != nil && errors.Is(err, errUIDUnchanged) {
				ordinaryErr = errors.Join(writeErr, err)
				return nil
			}
			if writeErr != nil {
				return nfc.NewError("verify UID write", nfc.CodeVerificationFailed, "写入响应异常，重新寻卡后也未检测到目标 UID", errors.Join(writeErr, err))
			}
			return err
		}
		return verifyUIDBlock(operationCtx, reader, newCard, plan, &result, emit)
	})
	if err != nil {
		return result, err
	}
	if ordinaryErr != nil {
		emit(UIDWriteEvent{Phase: UIDWriteFallback, Message: "普通 CUID/Gen2 写入未改变 UID（" + ordinaryErr.Error() + "），正在尝试 Gen1A 写入"})
		fallbackErr := s.fallback.Write(ctx, UIDFallbackRequest{TaskID: request.TaskID, Card: request.Card, Block0: result.NewBlock0}, emit)
		if fallbackErr != nil {
			return result, nfc.NewError("write UID", nfc.CodeIO, "普通写入与 Gen1A 写入均未完成", errors.Join(ordinaryErr, fallbackErr))
		}
		err = s.devices.WithReader(ctx, func(operationCtx context.Context, reader nfc.Reader) error {
			newCard, waitErr := s.waitForNewUID(operationCtx, request.Card, request.NewUID, reader, false)
			if waitErr != nil {
				return nfc.NewError("verify Gen1A UID write", nfc.CodeVerificationFailed, "Gen1A 写入后未检测到目标 UID", errors.Join(ordinaryErr, waitErr))
			}
			plan := UIDWritePlan{Card: request.Card.Clone(), OldBlock0: result.OldBlock0, NewBlock0: result.NewBlock0, KeyType: result.KeyType, Key: result.Key}
			return verifyUIDBlock(operationCtx, reader, newCard, plan, &result, emit)
		})
		if err != nil {
			return result, err
		}
	}
	emit(UIDWriteEvent{Phase: UIDWriteComplete, Message: "UID 与完整 block 0 已验证"})
	return result, nil
}

var errUIDUnchanged = errors.New("UID remained unchanged")

func verifyUIDBlock(ctx context.Context, reader nfc.Reader, newCard nfc.CardInfo, plan UIDWritePlan, result *UIDWriteResult, emit func(UIDWriteEvent)) error {
	result.NewCard = newCard.Clone()
	if err := reader.Authenticate(ctx, 0, plan.KeyType, plan.Key); err != nil {
		return nfc.NewError("verify UID write", nfc.CodeVerificationFailed, "新 UID 已出现，但 sector 0 无法使用原密钥认证", err)
	}
	emit(UIDWriteEvent{Phase: UIDWriteVerify, Message: "正在读取并核对完整 block 0"})
	actual, err := reader.ReadBlock(ctx, 0)
	result.ActualBlock0 = actual
	if err != nil {
		return nfc.NewError("verify UID write", nfc.CodeVerificationFailed, "新 UID 已出现，但无法读取 block 0", err)
	}
	result.ActualRead = true
	if actual != plan.NewBlock0 {
		return nfc.NewError("verify UID write", nfc.CodeVerificationFailed, "重新读取的 block 0 与预期不一致", nil)
	}
	bcc, _ := mifare.BCCFor4ByteUID(newCard.UID)
	if actual[4] != bcc {
		return nfc.NewError("verify UID write", nfc.CodeVerificationFailed, "重新读取的 block 0 BCC 无效", nil)
	}
	result.Verified = true
	return nil
}

func uidWriteMayHaveCommitted(err error) bool {
	return errors.Is(err, nfc.ErrIO) || errors.Is(err, nfc.ErrTimeout) ||
		errors.Is(err, nfc.ErrNoCard) || errors.Is(err, nfc.ErrCardChanged) ||
		errors.Is(err, nfc.ErrNotAuthenticated)
}

func validateUIDWriteRequest(request UIDWriteRequest) error {
	if nfc.InferCardType(request.Card) != nfc.CardTypeMIFAREClassic1K {
		return nfc.NewError("UID write preflight", nfc.CodeUnsupported, "当前只支持 4-byte UID 的 MIFARE Classic 1K 特殊卡", nil)
	}
	if len(request.Card.UID) != 4 || len(request.NewUID) != 4 {
		return nfc.NewError("UID write preflight", nfc.CodeInvalidArgument, "当前写 UID 功能只接受 4-byte UID", nil)
	}
	if bytes.Equal(request.Card.UID, request.NewUID) {
		return nfc.NewError("UID write preflight", nfc.CodeInvalidArgument, "新 UID 与当前 UID 相同", nil)
	}
	if request.Sector0Keys.Sector != 0 {
		return nfc.NewError("UID write preflight", nfc.CodeInvalidArgument, "必须提供 sector 0 的已验证密钥", nil)
	}
	return nil
}

func buildUIDWritePlan(ctx context.Context, reader nfc.Reader, request UIDWriteRequest) (UIDWritePlan, error) {
	if _, ok := reader.(nfc.ClassicManufacturerWriter); !ok {
		return UIDWritePlan{}, nfc.NewError("UID write preflight", nfc.CodeUnsupported, "reader does not expose the protected manufacturer-block capability", nil)
	}
	verified, err := verifyProvidedKeys(ctx, reader, request.Card, 0, mifare.Classic1K, request.Sector0Keys)
	if err != nil {
		return UIDWritePlan{}, err
	}
	readType, readKey, ok := preferredVerifiedKey(verified)
	if !ok {
		return UIDWritePlan{}, nfc.NewError("UID write preflight", nfc.CodeAuthenticationFailed, "没有已验证密钥可读取 sector 0", nil)
	}
	if err := selectExpectedCard(ctx, reader, request.Card); err != nil {
		return UIDWritePlan{}, err
	}
	if err := reader.Authenticate(ctx, 3, readType, readKey); err != nil {
		return UIDWritePlan{}, err
	}
	trailer, err := reader.ReadBlock(ctx, 3)
	if err != nil {
		return UIDWritePlan{}, err
	}
	access, err := mifare.DecodeAccessBits([3]byte{trailer[6], trailer[7], trailer[8]})
	if err != nil {
		return UIDWritePlan{}, nfc.NewError("UID write preflight", nfc.CodeInvalidArgument, "sector 0 access bits are invalid", err)
	}
	keyType, key, ok := choosePermittedKey(access.Groups[0].DataWriteKeys(), verified)
	if !ok {
		return UIDWritePlan{}, nfc.NewError("UID write preflight", nfc.CodePermission, "sector 0 access conditions do not permit block 0 writing with a verified key", nil)
	}
	if err := selectExpectedCard(ctx, reader, request.Card); err != nil {
		return UIDWritePlan{}, err
	}
	if err := reader.Authenticate(ctx, 0, keyType, key); err != nil {
		return UIDWritePlan{}, err
	}
	oldBlock, err := reader.ReadBlock(ctx, 0)
	if err != nil {
		return UIDWritePlan{}, err
	}
	if !bytes.Equal(oldBlock[0:4], request.Card.UID) {
		return UIDWritePlan{}, nfc.NewError("UID write preflight", nfc.CodeVerificationFailed, "block 0 UID does not match the selected card", nil)
	}
	oldBCC, _ := mifare.BCCFor4ByteUID(request.Card.UID)
	if oldBlock[4] != oldBCC {
		return UIDWritePlan{}, nfc.NewError("UID write preflight", nfc.CodeVerificationFailed, "current block 0 BCC is invalid", nil)
	}
	if request.ExpectedBlock0 != nil && oldBlock != *request.ExpectedBlock0 {
		return UIDWritePlan{}, nfc.NewError("UID write preflight", nfc.CodeCardChanged, "block 0 changed after the write preview", nil)
	}
	newBlock := oldBlock
	copy(newBlock[0:4], request.NewUID)
	bcc, _ := mifare.BCCFor4ByteUID(request.NewUID)
	newBlock[4] = bcc
	return UIDWritePlan{Card: request.Card.Clone(), OldBlock0: oldBlock, NewBlock0: newBlock, BCC: bcc, KeyType: keyType, Key: key}, nil
}

func (s *UIDService) waitForNewUID(ctx context.Context, oldCard nfc.CardInfo, newUID []byte, reader nfc.Reader, oldUIDIsFinal bool) (nfc.CardInfo, error) {
	limit := s.reselectLimit
	if limit <= 0 {
		limit = 8 * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	delay := s.retryDelay
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	sawOldUID := false
	for {
		card, err := reader.CardInfo(waitCtx)
		if err == nil {
			if card.ATQA != oldCard.ATQA || card.SAK != oldCard.SAK {
				return nfc.CardInfo{}, nfc.NewError("verify UID write", nfc.CodeCardChanged, "a different card type appeared after the write", nil)
			}
			if bytes.Equal(card.UID, newUID) {
				return card, nil
			}
			if !bytes.Equal(card.UID, oldCard.UID) {
				return nfc.CardInfo{}, nfc.NewError("verify UID write", nfc.CodeCardChanged, "an unexpected UID appeared after the write", nil)
			}
			sawOldUID = true
			if oldUIDIsFinal {
				return nfc.CardInfo{}, nfc.NewError("verify UID write", nfc.CodeVerificationFailed, "写命令结束后旧 UID 未改变", errUIDUnchanged)
			}
		} else if !errors.Is(err, nfc.ErrNoCard) && !errors.Is(err, nfc.ErrTimeout) {
			return nfc.CardInfo{}, err
		}

		timer := time.NewTimer(delay)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			if parentErr := ctx.Err(); parentErr != nil {
				code := nfc.CodeCanceled
				message := "UID 写入验证已取消"
				if errors.Is(parentErr, context.DeadlineExceeded) {
					code = nfc.CodeTimeout
					message = "UID 写入验证已超时"
				}
				return nfc.CardInfo{}, nfc.NewError("verify UID write", code, message, parentErr)
			}
			if sawOldUID {
				return nfc.CardInfo{}, nfc.NewError("verify UID write", nfc.CodeVerificationFailed, "写命令结束后旧 UID 未改变", errors.Join(errUIDUnchanged, waitCtx.Err()))
			}
			return nfc.CardInfo{}, nfc.NewError("verify UID write", nfc.CodeVerificationFailed, "写命令结束后没有重新检测到目标 UID", waitCtx.Err())
		case <-timer.C:
		}
	}
}
