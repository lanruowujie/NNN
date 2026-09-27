package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func (s *Service) ExternalEngineFixtureStatus() ExternalEngineStatusDTO {
	status := ExternalEngineStatusDTO{ID: "nfcx-fake-engine"}
	if s.fixtureAttack == nil {
		status.Detail = "开发测试引擎未配置"
		return status
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.fixtureAttack.Available(ctx); err != nil {
		status.Detail = "未安装开发测试引擎"
		return status
	}
	status.Available = true
	status.Detail = "仅用于验证设备交接、日志、取消和恢复"
	return status
}

func (s *Service) MFoCStatus() ExternalEngineStatusDTO {
	status := ExternalEngineStatusDTO{ID: "mfoc"}
	if s.mfocLocator == nil {
		status.Detail = "MFOC 未配置"
		return status
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := attack.NewMFoCEngine(s.mfocLocator, attack.MFoCInvocation{}).Available(ctx); err != nil {
		status.Detail = err.Error()
		return status
	}
	status.Available = true
	status.Detail = "MFOC 0.10.7 · 仅使用 NFCX 已验证密钥启动 Nested"
	return status
}

func (s *Service) MFCUKStatus() ExternalEngineStatusDTO {
	status := ExternalEngineStatusDTO{ID: "mfcuk-pipeline"}
	if s.mfcukPipeline == nil || s.mfocLocator == nil {
		status.Detail = "MFCUK pipeline 未配置"
		return status
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if err := attack.NewMFCUKEngine(s.mfocLocator, attack.MFCUKInvocation{}).Available(ctx); err != nil {
		status.Detail = err.Error()
		return status
	}
	if err := attack.NewMFoCEngine(s.mfocLocator, attack.MFoCInvocation{}).Available(ctx); err != nil {
		status.Detail = "MFCUK 可用，但后续 MFOC 不可用：" + err.Error()
		return status
	}
	status.Available = true
	status.Detail = "MFCUK 0.3.8 → MFOC 0.10.7 · 仅限授权的 Classic 1K"
	return status
}

func (s *Service) StartMFoC(input MFoCStartRequestDTO) (TaskDTO, error) {
	if s.mfocAttack == nil || s.mfocLocator == nil {
		return TaskDTO{}, attack.ErrUnavailable
	}
	if !input.Authorized {
		return TaskDTO{}, workflow.ErrMFoCAuthorization
	}
	availabilityCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := attack.NewMFoCEngine(s.mfocLocator, attack.MFoCInvocation{}).Available(availabilityCtx); err != nil {
		return TaskDTO{}, err
	}
	snapshot := s.deviceManager.Snapshot()
	if snapshot.Status != device.StatusReady && snapshot.Status != device.StatusPolling {
		if snapshot.Status == device.StatusBusy || snapshot.Status == device.StatusConnecting || snapshot.Status == device.StatusRecovering {
			return TaskDTO{}, nfc.ErrBusy
		}
		return TaskDTO{}, nfc.ErrNotOpen
	}
	if snapshot.Card == nil {
		return TaskDTO{}, ErrCardRequired
	}
	if !snapshot.Capabilities.Nested {
		return TaskDTO{}, workflow.ErrMFoCNested
	}
	card := snapshot.Card.Clone()
	if len(s.keyStore.Verified(keys.CardID(card))) == 0 {
		return TaskDTO{}, workflow.ErrMFoCSeedRequired
	}
	deviceInfo := snapshot.Device
	return s.startOperation("mfoc", "MFOC Nested 密钥恢复", func(ctx context.Context, taskID string) error {
		result, runErr := s.mfocAttack.Run(ctx, workflow.MFoCRequest{
			TaskID: taskID, Card: card, Device: deviceInfo, Authorized: true,
			Timeout: 30 * time.Minute, TemporaryPolicy: attack.CleanupAlways,
		}, func(event attack.AttackEvent) {
			s.emitOperation(attackEventDTO(taskID, "mfoc", event))
		})
		report := MFoCResultDTO{
			OutputValidated: result.External.Validated, Claimed: len(result.Claims), Added: result.Added,
			Existing: result.Existing, Rejected: result.Rejected, Conflicts: result.Conflicts,
		}
		if len(result.Claims) != 0 {
			s.emitter.Emit(KeyEventName, sectorKeyDTOs(s.keyStore, &card))
			s.emitter.Emit(KeyCatalogEventName, keyDTOs(s.keyStore))
			if err := s.persistKeys(); err != nil && runErr == nil {
				runErr = err
			}
		}
		if runErr == nil {
			report.FilledBytes, report.DataConflicts, report.KeyConflicts, runErr = s.readAndMergeAttackResult(ctx, card, deviceInfo)
			if runErr == nil {
				workbench := s.emitWorkbench()
				s.emitOperation(TaskEventDetailDTO{TaskID: taskID, Kind: "mfoc", Type: TaskEventProgress, Phase: string(attack.StateMergingResult), Message: "已用 libnfc 重新读取并安全合并工作台", Workbench: &workbench})
			}
		}
		exitCode := result.External.ExitCode
		recoveryError := ""
		if result.External.RecoveryError != nil {
			recoveryError = result.External.RecoveryError.Error()
		}
		s.emitOperation(TaskEventDetailDTO{
			TaskID: taskID, Kind: "mfoc", Type: TaskEventProgress, Phase: string(result.External.State),
			Engine: result.External.Engine, Version: result.External.Version, ExitCode: &exitCode, RecoveryError: recoveryError, MFoC: &report,
			Message: fmt.Sprintf("MFOC：声称 %d，把 %d 个新密钥标为已验证，%d 个冲突，%d 个复验失败", report.Claimed, report.Added, report.Conflicts, report.Rejected),
		})
		return runErr
	})
}

func (s *Service) StartMFCUKPipeline(input MFCUKStartRequestDTO) (TaskDTO, error) {
	if s.mfcukPipeline == nil || s.mfocLocator == nil {
		return TaskDTO{}, attack.ErrUnavailable
	}
	if !input.Authorized {
		return TaskDTO{}, workflow.ErrMFCUKAuthorization
	}
	status := s.MFCUKStatus()
	if !status.Available {
		return TaskDTO{}, fmt.Errorf("%w: %s", attack.ErrUnavailable, status.Detail)
	}
	snapshot := s.deviceManager.Snapshot()
	if snapshot.Status != device.StatusReady && snapshot.Status != device.StatusPolling {
		if snapshot.Status == device.StatusBusy || snapshot.Status == device.StatusConnecting || snapshot.Status == device.StatusRecovering {
			return TaskDTO{}, nfc.ErrBusy
		}
		return TaskDTO{}, nfc.ErrNotOpen
	}
	if snapshot.Card == nil {
		return TaskDTO{}, ErrCardRequired
	}
	card := snapshot.Card.Clone()
	if nfc.InferCardType(card) != nfc.CardTypeMIFAREClassic1K {
		return TaskDTO{}, workflow.ErrMFCUKClassic1K
	}
	if !snapshot.Capabilities.Darkside {
		return TaskDTO{}, workflow.ErrMFCUKDarkside
	}
	if !snapshot.Capabilities.Nested {
		return TaskDTO{}, workflow.ErrMFoCNested
	}
	if len(s.keyStore.Verified(keys.CardID(card))) != 0 {
		return TaskDTO{}, workflow.ErrMFCUKSeedAlreadyKnown
	}
	deviceInfo := snapshot.Device
	return s.startOperation("mfcuk_pipeline", "MFCUK → MFOC 自动密钥恢复", func(ctx context.Context, taskID string) error {
		result, runErr := s.mfcukPipeline.Run(ctx, workflow.MFCUKPipelineRequest{
			TaskID: taskID, Card: card, Device: deviceInfo, Authorized: true, Timeout: 60 * time.Minute,
			MFCUKTimeout: 30 * time.Minute, MFoCTimeout: 30 * time.Minute, TemporaryPolicy: attack.CleanupAlways,
		}, func(event attack.AttackEvent) {
			s.emitOperation(attackEventDTO(taskID, "mfcuk_pipeline", event))
		})
		report := MFCUKResultDTO{
			CandidateOutputValidated: result.MFCUK.Validated, Candidates: len(result.Candidates), Verified: result.Verified,
			Existing: result.Existing, Rejected: result.Rejected, Conflicts: result.Conflicts,
			MFCUKVersion: result.MFCUK.Version, MFCUKDurationMillis: result.MFCUK.Duration().Milliseconds(),
			MFoCOutputValidated: result.MFoC.External.Validated, MFoCVersion: result.MFoC.External.Version,
			MFoCDurationMillis: result.MFoC.External.Duration().Milliseconds(), MFoCAdded: result.MFoC.Added, MFoCRejected: result.MFoC.Rejected,
		}
		if len(result.Candidates) != 0 || len(result.MFoC.Claims) != 0 {
			s.emitter.Emit(KeyEventName, sectorKeyDTOs(s.keyStore, &card))
			s.emitter.Emit(KeyCatalogEventName, keyDTOs(s.keyStore))
			if err := s.persistKeys(); err != nil && runErr == nil {
				runErr = err
			}
		}
		if runErr == nil {
			s.emitOperation(TaskEventDetailDTO{TaskID: taskID, Kind: "mfcuk_pipeline", Type: TaskEventProgress, Phase: string(attack.StateReadingDump), Message: "两阶段密钥复验完成，正在重新读取卡片"})
			report.FilledBytes, report.DataConflicts, report.KeyConflicts, runErr = s.readAndMergeAttackResult(ctx, card, deviceInfo)
			if runErr == nil {
				workbench := s.emitWorkbench()
				s.emitOperation(TaskEventDetailDTO{TaskID: taskID, Kind: "mfcuk_pipeline", Type: TaskEventProgress, Phase: string(attack.StateMergingResult), Message: "已用 libnfc 重新读取并安全合并工作台", Workbench: &workbench})
			}
		}
		s.emitOperation(TaskEventDetailDTO{
			TaskID: taskID, Kind: "mfcuk_pipeline", Type: TaskEventProgress, Phase: string(result.MFoC.External.State), MFCUK: &report,
			Message: fmt.Sprintf("MFCUK 候选 %d，验证 %d，拒绝 %d；MFOC 新增验证密钥 %d", report.Candidates, report.Verified, report.Rejected, report.MFoCAdded),
		})
		return runErr
	})
}

func (s *Service) readAndMergeAttackResult(ctx context.Context, card nfc.CardInfo, deviceInfo nfc.DeviceInfo) (int, int, int, error) {
	layout, err := appLayoutForCard(card)
	if err != nil {
		return 0, 0, 0, err
	}
	readResult, readErr := s.dumpService.ReadCard(ctx, workflow.DumpRequest{
		Card: card, Device: deviceInfo, Keys: verifiedSectorKeys(s.keyStore, card, layout),
	})
	filled, dataConflicts, keyConflicts := 0, 0, 0
	if dumpHasKnownBytes(readResult.Dump) {
		merge, mergeErr := s.bench.MergeReadResult(readResult)
		if mergeErr != nil {
			return 0, 0, 0, mergeErr
		}
		filled, dataConflicts, keyConflicts = merge.FilledBytes, len(merge.Conflicts), len(merge.KeyConflicts)
	}
	return filled, dataConflicts, keyConflicts, readErr
}

func dumpHasKnownBytes(image mifare.Dump) bool {
	if !image.Layout.Valid() {
		return false
	}
	for _, block := range image.Blocks {
		if block.KnownMask != 0 {
			return true
		}
	}
	return false
}

func (s *Service) StartExternalEngineFixture() (TaskDTO, error) {
	if s.fixtureAttack == nil {
		return TaskDTO{}, attack.ErrUnavailable
	}
	availabilityCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.fixtureAttack.Available(availabilityCtx); err != nil {
		return TaskDTO{}, err
	}
	snapshot := s.deviceManager.Snapshot()
	if snapshot.Status != device.StatusReady && snapshot.Status != device.StatusPolling {
		if snapshot.Status == device.StatusBusy || snapshot.Status == device.StatusConnecting || snapshot.Status == device.StatusRecovering {
			return TaskDTO{}, nfc.ErrBusy
		}
		return TaskDTO{}, nfc.ErrNotOpen
	}
	return s.startOperation("external_fixture", "外部引擎框架自检", func(ctx context.Context, taskID string) error {
		result, err := s.fixtureAttack.Run(ctx, attack.AttackRequest{
			TaskID:          taskID,
			Timeout:         30 * time.Second,
			TemporaryPolicy: attack.CleanupAlways,
		}, func(event attack.AttackEvent) {
			s.emitOperation(externalAttackEventDTO(taskID, event))
		})
		exitCode := result.ExitCode
		s.emitOperation(TaskEventDetailDTO{
			TaskID: taskID, Kind: "external_fixture", Type: TaskEventProgress, Phase: string(result.State),
			Engine: result.Engine, Version: result.Version, ExitCode: &exitCode,
			Message: fmt.Sprintf("外部引擎诊断：版本 %s，退出码 %d，耗时 %s", result.Version, result.ExitCode, result.Duration().Round(time.Millisecond)),
		})
		if result.RecoveryError != nil {
			s.emitOperation(TaskEventDetailDTO{
				TaskID: taskID, Kind: "external_fixture", Type: TaskEventProgress, Phase: string(attack.StateReopeningDevice),
				Engine: result.Engine, RecoveryError: result.RecoveryError.Error(), Message: "外部任务结果已保留，但读卡器恢复失败",
			})
		}
		return err
	})
}

func externalAttackEventDTO(taskID string, event attack.AttackEvent) TaskEventDetailDTO {
	return attackEventDTO(taskID, "external_fixture", event)
}

func attackEventDTO(taskID, kind string, event attack.AttackEvent) TaskEventDetailDTO {
	message := event.Message
	if message == "" {
		message = attackStateMessage(event.State)
	}
	return TaskEventDetailDTO{
		TaskID: taskID, Kind: kind, Type: TaskEventProgress, Phase: string(event.State),
		Engine: event.Engine, Stream: string(event.Stream), Log: event.Data, Sequence: event.Sequence,
		Message: message, Time: event.Time.Format(time.RFC3339Nano),
	}
}

func attackStateMessage(state attack.State) string {
	switch state {
	case attack.StateQueued:
		return "外部任务已排队"
	case attack.StatePreflight:
		return "正在重新确认目标卡片"
	case attack.StateReleasingDevice:
		return "正在停止轮询并释放读卡器"
	case attack.StateStarting:
		return "正在启动受控外部程序"
	case attack.StateRunning:
		return "外部程序正在运行"
	case attack.StateCancelling:
		return "正在终止外部程序及其子进程"
	case attack.StateValidatingResult:
		return "正在校验外部程序结果"
	case attack.StateReopeningDevice:
		return "正在重新打开读卡器"
	case attack.StateVerifyingKeys:
		return "正在用 libnfc 重新验证候选密钥"
	case attack.StateMergingResult:
		return "正在安全合并已验证结果"
	case attack.StateMFCUKStage:
		return "阶段 1/2：MFCUK Darkside"
	case attack.StateVerifyingSeed:
		return "正在用 libnfc 验证 MFCUK 候选"
	case attack.StateMFoCStage:
		return "阶段 2/2：MFOC Nested"
	case attack.StateReadingDump:
		return "正在用已验证密钥读取卡片"
	case attack.StateSucceeded:
		return "外部程序结果校验成功"
	case attack.StateFailed:
		return "外部程序执行失败"
	case attack.StateCancelled:
		return "外部程序已取消"
	default:
		return strings.TrimSpace(string(state))
	}
}
