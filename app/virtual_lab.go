package app

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
