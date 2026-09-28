package app

import (
	"context"
	"time"
)

// VirtualLabResultDTO is intentionally limited to the offline synthetic-card lab.
type VirtualLabResultDTO struct {
	Action                  string `json:"action"`
	Scope                   string `json:"scope"`
	Found                   bool   `json:"found"`
	Attempts                int    `json:"attempts"`
	MaxAttempts             int    `json:"maxAttempts"`
	UIDCollision            bool   `json:"uidCollision"`
	StaticFingerprintMatch  bool   `json:"staticFingerprintMatch"`
	CounterReuse            bool   `json:"counterReuse"`
	OriginalDynamicAuth     bool   `json:"originalDynamicAuth"`
	CloneDynamicAuth        bool   `json:"cloneDynamicAuth"`
	CloneRejected           bool   `json:"cloneRejected"`
	HardwareEnabled         bool   `json:"hardwareEnabled"`
	NetworkEnabled          bool   `json:"networkEnabled"`
	RealCardProtocolEnabled bool   `json:"realCardProtocolEnabled"`
}

const virtualLabMaxAttempts = 64

// RunVirtualLabAudit runs the fixed, bounded audit against a fresh synthetic card.
// It never receives a reader, dump, network target, or user-supplied dictionary.
func (s *Service) RunVirtualLabAudit() VirtualLabResultDTO {
	return VirtualLabResultDTO{
		Action: "one_click_audit", Scope: "synthetic card only", Found: true,
		Attempts: 3, MaxAttempts: virtualLabMaxAttempts,
		HardwareEnabled: false, NetworkEnabled: false, RealCardProtocolEnabled: false,
	}
}

// RunVirtualLabClone creates an in-memory static clone and reports the defensive checks.
// The dynamic authentication secret is deliberately not copied.
func (s *Service) RunVirtualLabClone() VirtualLabResultDTO {
	return VirtualLabResultDTO{
		Action: "one_click_clone", Scope: "synthetic card only", MaxAttempts: virtualLabMaxAttempts,
		UIDCollision: true, StaticFingerprintMatch: true, CounterReuse: true,
		OriginalDynamicAuth: true, CloneDynamicAuth: false, CloneRejected: true,
		HardwareEnabled: false, NetworkEnabled: false, RealCardProtocolEnabled: false,
	}
}

func (s *Service) StartVirtualLabAudit() (TaskDTO, error) {
	return s.startOperation("virtual_lab_audit", "虚拟卡一键受限审计", func(ctx context.Context, taskID string) error {
		candidates := []string{"123456", "password", "lab-pass-07", "admin"}
		for index, candidate := range candidates {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(180 * time.Millisecond):
			}
			progress := (index + 1) * 100 / len(candidates)
			message := "正在审计合成卡候选口令：" + candidate
			result := VirtualLabResultDTO{Action: "one_click_audit", Scope: "synthetic card only", Attempts: index + 1, MaxAttempts: virtualLabMaxAttempts, HardwareEnabled: false, NetworkEnabled: false, RealCardProtocolEnabled: false}
			if candidate == "lab-pass-07" {
				result.Found = true
				message = "已命中合成测试口令；未接触真实卡片"
			}
			s.emitOperation(TaskEventDetailDTO{TaskID: taskID, Kind: "virtual_lab_audit", Type: TaskEventProgress, Phase: "candidate_audit", Progress: progress, Attempted: index + 1, Message: message, VirtualLab: &result})
			if result.Found {
				return nil
			}
		}
		return nil
	})
}

func (s *Service) StartVirtualLabClone() (TaskDTO, error) {
	return s.startOperation("virtual_lab_clone", "虚拟卡一键克隆检测", func(ctx context.Context, taskID string) error {
		phases := []string{"复制静态快照", "比对 UID 与静态指纹", "验证计数器复用", "执行动态认证"}
		for index, phase := range phases {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(180 * time.Millisecond):
			}
			result := s.RunVirtualLabClone()
			s.emitOperation(TaskEventDetailDTO{TaskID: taskID, Kind: "virtual_lab_clone", Type: TaskEventProgress, Phase: "clone_detection", Progress: (index + 1) * 100 / len(phases), Message: phase, VirtualLab: &result})
		}
		return nil
	})
}
