package app

// DashboardDTO is the initial, serializable state rendered by the GUI.
type DashboardDTO struct {
	Devices        []DeviceDTO    `json:"devices"`
	SelectedDevice string         `json:"selectedDevice"`
	Connection     ConnectionDTO  `json:"connection"`
	Card           CardDTO        `json:"card"`
	Blocks         []BlockDTO     `json:"blocks"`
	Keys           []SectorKeyDTO `json:"keys"`
	Actions        []ActionDTO    `json:"actions"`
}

// DeviceDTO describes a reader without exposing a future Reader implementation.
type DeviceDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ConnString string `json:"connString"`
	Transport  string `json:"transport"`
	Available  bool   `json:"available"`
	Nested     bool   `json:"nested"`
	Darkside   bool   `json:"darkside"`
	Hardnested bool   `json:"hardnested"`
}

// ConnectionDTO contains the reader connection summary shown in the shell.
type ConnectionDTO struct {
	Status    string `json:"status"`
	Label     string `json:"label"`
	Detail    string `json:"detail"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// CardDTO contains only card facts safe to send to the GUI.
type CardDTO struct {
	Present   bool   `json:"present"`
	UID       string `json:"uid"`
	UIDLength int    `json:"uidLength"`
	ATQA      string `json:"atqa"`
	SAK       string `json:"sak"`
	Type      string `json:"type"`
	TypeCode  string `json:"typeCode"`
	Inferred  bool   `json:"inferred"`
}

type DeviceStateEventDTO struct {
	Connection ConnectionDTO `json:"connection"`
	Device     DeviceDTO     `json:"device"`
	Card       CardDTO       `json:"card"`
	Time       string        `json:"time"`
}

type CardEventDTO struct {
	Type string  `json:"type"`
	Card CardDTO `json:"card"`
	Time string  `json:"time"`
}

// BlockDTO is a read-only mock row used to establish the workbench layout.
type BlockDTO struct {
	Block  int    `json:"block"`
	Sector int    `json:"sector"`
	Kind   string `json:"kind"`
	Hex    string `json:"hex"`
	Status string `json:"status"`
}

// SectorKeyDTO exposes verified key material in the dedicated key-status UI.
// Task logs omit key values instead of serializing a masked representation.
type SectorKeyDTO struct {
	Sector     int    `json:"sector"`
	KeyAStatus string `json:"keyAStatus"`
	KeyBStatus string `json:"keyBStatus"`
	KeyAID     string `json:"keyAId,omitempty"`
	KeyBID     string `json:"keyBId,omitempty"`
	KeyA       string `json:"keyA,omitempty"`
	KeyB       string `json:"keyB,omitempty"`
}

// ActionDTO reserves the main operations without implementing NFC behaviour.
type ActionDTO struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
	Risk    string `json:"risk"`
}

// TaskDTO is returned immediately when an asynchronous task starts.
type TaskDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ExternalEngineStatusDTO controls the development-only framework self-test.
// Release builds omit the fixture executable, so Available remains false.
type ExternalEngineStatusDTO struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
}

// TaskEventDTO is the single event envelope consumed by the GUI.
type TaskEventDTO struct {
	TaskID   string `json:"taskId"`
	Type     string `json:"type"`
	Progress int    `json:"progress"`
	Message  string `json:"message"`
	Time     string `json:"time"`
}
