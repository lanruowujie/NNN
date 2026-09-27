package workflow

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/nfc"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

type UIDFallbackRequest struct {
	TaskID string
	Card   nfc.CardInfo
	Block0 [nfc.BlockSize]byte
}

// UIDFallbackWriter owns the out-of-process Gen1A path. Implementations must
// release the in-process reader before invoking a raw-frame helper.
type UIDFallbackWriter interface {
	Write(context.Context, UIDFallbackRequest, func(UIDWriteEvent)) error
}

type Gen1AUIDWriter struct {
	released attack.ReleasedDeviceController
	locator  *runtimebundle.Locator
}

func NewGen1AUIDWriter(released attack.ReleasedDeviceController, locator *runtimebundle.Locator) *Gen1AUIDWriter {
	return &Gen1AUIDWriter{released: released, locator: locator}
}

func (w *Gen1AUIDWriter) Write(ctx context.Context, request UIDFallbackRequest, emit func(UIDWriteEvent)) error {
	if w == nil || w.released == nil || w.locator == nil {
		return attack.ErrUnavailable
	}
	engine := attack.NewMFSetUIDEngine(w.locator, attack.MFSetUIDInvocation{Card: request.Card, Block0: request.Block0})
	coordinator := attack.NewCoordinator(w.released, engine)
	expected := request.Card.Clone()
	result, err := coordinator.Run(ctx, attack.AttackRequest{
		TaskID: request.TaskID, Timeout: 15 * time.Second, ExpectedCard: &expected,
	}, func(event attack.AttackEvent) {
		if emit == nil {
			return
		}
		if raw := strings.TrimSpace(event.Data); raw != "" {
			emit(UIDWriteEvent{Phase: UIDWriteFallback, Message: "nfc-mfsetuid: " + raw})
		}
		if event.Message == "" {
			return
		}
		message := "正在执行 Gen1A 写入"
		switch event.State {
		case attack.StateReleasingDevice:
			message = "正在释放读卡器并切换到 Gen1A 写入"
		case attack.StateStarting, attack.StateRunning:
			message = "正在发送 Gen1A 解锁与 block 0 写入序列"
		case attack.StateReopeningDevice:
			message = "正在重新打开读卡器"
		}
		emit(UIDWriteEvent{Phase: UIDWriteFallback, Message: message})
	})
	if err != nil {
		return err
	}
	if result.RecoveryError != nil {
		return errors.Join(errors.New("Gen1A 写入后无法重新打开读卡器"), result.RecoveryError)
	}
	if emit != nil {
		emit(UIDWriteEvent{Phase: UIDWriteFallback, Message: describeGen1AResult(result.Stdout, result.Stderr)})
	}
	return nil
}

func describeGen1AResult(stdout, stderr []byte) string {
	output := bytes.Join([][]byte{stdout, stderr}, []byte{'\n'})
	switch {
	case bytes.Contains(output, []byte("Unlock command [1/2]: failed")):
		return "Gen1A 诊断：卡片未响应 7-bit 0x40 解锁命令，不支持该后门或已锁定"
	case bytes.Contains(output, []byte("Unlock command [2/2]: failed")):
		return "Gen1A 诊断：卡片响应了第一步，但拒绝 0x43 解锁命令"
	case bytes.Contains(output, []byte("Card unlocked")):
		return "Gen1A 诊断：卡片已接受后门解锁，写入结果将以重新寻卡和完整 block 0 回读为准"
	default:
		return "Gen1A 诊断：工具未报告明确的解锁结果，写入结果将以重新寻卡和完整 block 0 回读为准"
	}
}
