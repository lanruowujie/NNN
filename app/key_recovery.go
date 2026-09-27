package app

import (
	"context"
	"errors"
	"time"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func (s *Service) StartKeyRecovery(input KeyRecoveryStartRequestDTO) (TaskDTO, error) {
	if s.keyRecovery == nil {
		return TaskDTO{}, errors.New("密钥恢复流程未配置")
	}
	if !input.Authorized {
		return TaskDTO{}, errors.New("必须确认卡片已获授权")
	}
	if s.bench.Snapshot().Dirty && !input.DiscardDirty {
		return TaskDTO{}, ErrDirtyWorkbench
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
	card, deviceInfo := snapshot.Card.Clone(), snapshot.Device
	if _, err := appLayoutForCard(card); err != nil {
		return TaskDTO{}, err
	}

	return s.startPreparedOperation("key_recovery", "自动恢复密钥", s.clearWorkbench, func(ctx context.Context, taskID string) error {
		result, runErr := s.keyRecovery.Run(ctx, workflow.KeyRecoveryRequest{
			TaskID: taskID, Card: card, Device: deviceInfo, Authorized: true,
			DarksideTimeout: time.Hour, NestedTimeout: 30 * time.Minute, HardnestedTimeout: 6 * time.Hour,
			TemporaryPolicy: attack.CleanupAlways,
		}, func(event workflow.KeyRecoveryEvent) {
			s.emitOperation(keyRecoveryEventDTO(taskID, event))
		})

		s.emitter.Emit(KeyEventName, sectorKeyDTOs(s.keyStore, &card))
		s.emitter.Emit(KeyCatalogEventName, keyDTOs(s.keyStore))
		if persistErr := s.persistKeys(); persistErr != nil && runErr == nil {
			runErr = persistErr
		}
		if result.Read.Dump.Layout.Valid() && len(result.Read.Dump.Blocks) != 0 {
			s.registerDumpKeys(result.Read)
			if _, mergeErr := s.bench.MergeReadResult(result.Read); mergeErr != nil && runErr == nil {
				runErr = mergeErr
			} else if mergeErr == nil {
				workbench := s.emitWorkbench()
				s.emitOperation(TaskEventDetailDTO{
					TaskID: taskID, Kind: "key_recovery", Type: TaskEventProgress, Phase: string(workflow.KeyRecoveryReadiness),
					Step: 5, TotalSteps: workflow.KeyRecoveryTotalSteps, StepStatus: string(workflow.KeyRecoveryCompleted), Progress: 100,
					Message: "读取验证结果已载入工作台", Workbench: &workbench,
				})
			}
		}
		if !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, nfc.ErrCanceled) && !errors.Is(runErr, attack.ErrCancelled) {
			report := keyRecoveryResultDTO(result, s.deviceManager.Snapshot())
			s.emitOperation(TaskEventDetailDTO{
				TaskID: taskID, Kind: "key_recovery", Type: TaskEventProgress, Phase: string(result.FailedStage),
				Progress: recoveryResultProgress(result), Message: keyRecoveryResultMessage(report), Recovery: &report,
			})
		}
		return runErr
	})
}

func keyRecoveryEventDTO(taskID string, event workflow.KeyRecoveryEvent) TaskEventDetailDTO {
	dto := TaskEventDetailDTO{
		TaskID: taskID, Kind: "key_recovery", Type: TaskEventProgress,
		Phase: string(event.Stage), Step: event.Step, TotalSteps: workflow.KeyRecoveryTotalSteps,
		StepStatus: string(event.Status), Progress: event.Progress, Indeterminate: event.Indeterminate,
		Sector: event.Sector, Attempted: event.Attempted, Found: event.Found,
		Message: event.Message,
	}
	if event.KeyTypeSet {
		dto.KeyType = keyTypeName(event.KeyType)
	}
	if event.Attack != nil {
		attackDTO := attackEventDTO(taskID, "key_recovery", *event.Attack)
		dto.Engine, dto.Version, dto.Stream, dto.Log = attackDTO.Engine, attackDTO.Version, attackDTO.Stream, attackDTO.Log
		dto.Sequence, dto.ExitCode, dto.RecoveryError = attackDTO.Sequence, attackDTO.ExitCode, attackDTO.RecoveryError
	}
	return dto
}

func keyRecoveryResultDTO(result workflow.KeyRecoveryResult, snapshot device.Snapshot) KeyRecoveryResultDTO {
	steps := make([]KeyRecoveryStepDTO, len(result.Steps))
	for index, step := range result.Steps {
		steps[index] = KeyRecoveryStepDTO{Number: step.Number, Stage: string(step.Stage), Label: step.Label, Status: string(step.Status), Message: step.Message}
	}
	readerRecovered := snapshot.Status == device.StatusReady || snapshot.Status == device.StatusPolling
	return KeyRecoveryResultDTO{
		Outcome: string(result.Outcome), Steps: steps, VerifiedKeySlots: result.VerifiedKeySlots,
		CoveredSectors: result.CoveredSectors, ReadableSectors: result.ReadableSectors, TotalSectors: result.TotalSectors,
		DumpComplete: result.DumpComplete, FailedStep: result.FailedStep, FailedStage: string(result.FailedStage),
		Reason: result.Reason, HardnestedAvailable: result.HardnestedAvailable, ReaderRecovered: readerRecovered,
	}
}

func keyRecoveryResultMessage(result KeyRecoveryResultDTO) string {
	switch result.Outcome {
	case string(workflow.KeyRecoveryComplete):
		return "密钥恢复完成：所有扇区均已通过实际读取验证"
	case string(workflow.KeyRecoveryPartial):
		return "密钥恢复得到部分结果：有效密钥和可读数据已保留"
	default:
		return "密钥恢复失败：" + result.Reason
	}
}

func recoveryResultProgress(result workflow.KeyRecoveryResult) int {
	if result.Outcome == workflow.KeyRecoveryComplete || result.Outcome == workflow.KeyRecoveryPartial {
		return 100
	}
	if result.FailedStep > 0 {
		return (result.FailedStep - 1) * 20
	}
	return 0
}
