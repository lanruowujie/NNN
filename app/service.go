package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/diagnostic"
	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/libnfc"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
	"github.com/BennyThink/NFCX/internal/telemetry"
	"github.com/BennyThink/NFCX/internal/terminal"
	"github.com/BennyThink/NFCX/internal/update"
	"github.com/BennyThink/NFCX/internal/workbench"
	"github.com/BennyThink/NFCX/internal/workflow"
)

const (
	// MockTaskEventName is the Wails event channel used by the scaffold task.
	MockTaskEventName   = "nfcx:mock-task"
	DeviceEventName     = "nfcx:device-state"
	CardEventName       = "nfcx:card-state"
	TaskEventName       = "nfcx:task"
	UpdateEventName     = "nfcx:update"
	WorkbenchEventName  = "nfcx:workbench"
	KeyEventName        = "nfcx:sector-keys"
	KeyCatalogEventName = "nfcx:key-catalog"

	TaskEventStarted   = "started"
	TaskEventProgress  = "progress"
	TaskEventCompleted = "completed"
	TaskEventCancelled = "cancelled"
)

var (
	ErrTaskIDRequired = errors.New("task ID is required")
	ErrTaskNotFound   = errors.New("task not found")
)

// EventEmitter keeps the application service independent of the Wails runtime.
type EventEmitter interface {
	Emit(name string, payload any)
}

// Diagnostics runs the same read-only, no-hardware checks used by release CI.
func (s *Service) Diagnostics() diagnostic.Report {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	currentDevice := ""
	if s.deviceManager != nil {
		currentDevice = s.deviceManager.Snapshot().Device.ConnString
	}
	return diagnostic.Run(ctx, s.mfocLocator, currentDevice)
}

// OpenCommandTerminal releases the selected reader, then opens a user-driven
// shell whose PATH includes NFCX's private runtime. NFCX cannot observe when a
// user-run command ends, so the reader intentionally remains disconnected
// until the user reconnects it in the GUI.
func (s *Service) OpenCommandTerminal() error {
	s.mu.Lock()
	busy := len(s.operations) != 0
	s.mu.Unlock()
	if busy {
		return errors.New("wait for the active NFCX task to finish before opening a command terminal")
	}
	runtimeDir, err := s.mfocLocator.RuntimeRoot()
	if err != nil {
		return fmt.Errorf("locate NFCX command-line runtime: %w", err)
	}
	snapshot := s.deviceManager.Snapshot()
	connString := snapshot.Device.ConnString
	if connString != "" {
		if err := s.deviceManager.Disconnect(); err != nil {
			return fmt.Errorf("release reader for command terminal: %w", err)
		}
	}
	if err := terminal.Open(terminal.Environment{RuntimeDir: runtimeDir, Device: connString}); err != nil {
		if connString != "" {
			ctx, cancel := context.WithTimeout(context.Background(), s.deviceTimeout)
			defer cancel()
			if reconnectErr := s.deviceManager.Connect(ctx, connString); reconnectErr != nil {
				return fmt.Errorf("open command terminal: %w; reconnect reader: %v", err, reconnectErr)
			}
		}
		return fmt.Errorf("open command terminal: %w", err)
	}
	return nil
}

type noopEmitter struct{}

func (noopEmitter) Emit(string, any) {}

type mockTask struct {
	cancel          context.CancelFunc
	cancelRequested bool
}

type operationTask struct {
	cancel          context.CancelFunc
	kind            string
	cancelRequested bool
}

type mockTaskConfig struct {
	steps        int
	stepInterval time.Duration
}

// Service provides the GUI-facing application behaviour.
type Service struct {
	mu               sync.Mutex
	emitter          EventEmitter
	tasks            map[string]*mockTask
	nextID           atomic.Uint64
	config           mockTaskConfig
	deviceManager    *device.Manager
	devices          []nfc.DeviceInfo
	deviceTimeout    time.Duration
	keyStore         *keys.Store
	keyStorePath     string
	bench            *workbench.Model
	keyScanner       *workflow.KeyScanService
	keyRecovery      *workflow.KeyRecoveryService
	dumpService      *workflow.DumpService
	restoreService   *workflow.RestoreService
	uidService       *workflow.UIDService
	fixtureAttack    *attack.Coordinator
	mfocAttack       *workflow.MFoCService
	mfcukPipeline    *workflow.MFCUKPipelineService
	hardnestedAttack *workflow.HardnestedService
	mfocLocator      *runtimebundle.Locator
	operations       map[string]*operationTask
	preflights       map[string]writePreflightRecord
	uidPreflights    map[string]uidPreflightRecord
	dialogs          fileDialogs
	telemetry        *telemetry.Client
	telemetryStore   *telemetry.Store
	updater          *update.Manager
}

// NewService wires the application service to the current libnfc backend.
func NewService(emitter EventEmitter) *Service {
	return newService(emitter, mockTaskConfig{steps: 20, stepInterval: 180 * time.Millisecond})
}

func newService(emitter EventEmitter, config mockTaskConfig) *Service {
	if emitter == nil {
		emitter = noopEmitter{}
	}
	if config.steps < 1 {
		config.steps = 1
	}
	if config.stepInterval <= 0 {
		config.stepInterval = time.Millisecond
	}
	backend := libnfc.NewBackend()
	manager := device.NewManager(backend, func() nfc.Reader { return backend.NewReader() }, deviceEventSink{emitter: emitter}, device.DefaultConfig())
	locator := runtimebundle.DefaultLocator()
	fixtureAttack := attack.NewCoordinator(manager, attack.NewFixtureEngine(locator))
	keyStore := keys.NewStore()
	mfocAttack := workflow.NewMFoCService(manager, manager, keyStore, func(invocation attack.MFoCInvocation) attack.AttackEngine {
		return attack.NewMFoCEngine(locator, invocation)
	})
	mfcukPipeline := workflow.NewMFCUKPipelineService(manager, manager, keyStore, func(invocation attack.MFCUKInvocation) attack.AttackEngine {
		return attack.NewMFCUKEngine(locator, invocation)
	}, mfocAttack)
	hardnestedAttack := workflow.NewHardnestedService(manager, manager, keyStore, func(invocation attack.MFoCInvocation) attack.AttackEngine {
		return attack.NewHardnestedEngine(locator, invocation)
	})
	keyScanner := workflow.NewKeyScanService(manager, keyStore)
	dumpService := workflow.NewDumpService(manager)
	keyRecovery := workflow.NewKeyRecoveryService(keyScanner, keyStore, mfcukPipeline, mfocAttack, hardnestedAttack, dumpService)
	configRoot, _ := os.UserConfigDir()
	keyStorePath := ""
	uidBackupRoot := ""
	telemetryPath := ""
	updatePath := ""
	updateStaging := ""
	if configRoot != "" {
		keyStorePath = filepath.Join(configRoot, "NFCX", "keys.json")
		uidBackupRoot = filepath.Join(configRoot, "NFCX", "uid-backups")
		telemetryPath = filepath.Join(configRoot, "NFCX", "telemetry.json")
		updatePath = filepath.Join(configRoot, "NFCX", "updates.json")
		updateStaging = filepath.Join(configRoot, "NFCX", "updates")
		_ = keyStore.Load(keyStorePath)
	}
	telemetryStore := telemetry.NewStore(telemetryPath)
	if telemetryPath != "" {
		_ = telemetryStore.Load()
	}
	updater, _ := update.NewManager(update.Config{ConfigPath: updatePath, StagingDir: updateStaging, AllowedHosts: []string{"github.com", "objects.githubusercontent.com"}})
	return &Service{
		emitter:          emitter,
		tasks:            make(map[string]*mockTask),
		config:           config,
		deviceManager:    manager,
		deviceTimeout:    5 * time.Second,
		keyStore:         keyStore,
		keyStorePath:     keyStorePath,
		bench:            workbench.New(),
		keyScanner:       keyScanner,
		keyRecovery:      keyRecovery,
		dumpService:      dumpService,
		restoreService:   workflow.NewRestoreService(manager),
		uidService:       workflow.NewUIDService(manager, workflow.FileUIDBackupStore{Root: uidBackupRoot}, workflow.NewGen1AUIDWriter(manager, locator)),
		fixtureAttack:    fixtureAttack,
		mfocAttack:       mfocAttack,
		mfcukPipeline:    mfcukPipeline,
		hardnestedAttack: hardnestedAttack,
		mfocLocator:      locator,
		operations:       make(map[string]*operationTask),
		preflights:       make(map[string]writePreflightRecord),
		uidPreflights:    make(map[string]uidPreflightRecord),
		dialogs:          noFileDialogs{},
		telemetry:        telemetry.NewClient(telemetryStore),
		telemetryStore:   telemetryStore,
		updater:          updater,
	}
}

// Dashboard returns the latest real device/card snapshot. Memory and key data
// remain empty until their respective specifications are implemented.
func (s *Service) Dashboard() DashboardDTO {
	s.mu.Lock()
	knownDevices := append([]nfc.DeviceInfo(nil), s.devices...)
	s.mu.Unlock()
	snapshot := s.deviceManager.Snapshot()
	devices := make([]DeviceDTO, 0, len(knownDevices)+1)
	for _, info := range knownDevices {
		devices = append(devices, deviceDTO(info))
	}
	if snapshot.Device.ConnString != "" && !containsDevice(knownDevices, snapshot.Device.ConnString) {
		devices = append(devices, deviceDTO(snapshot.Device))
	}
	var benchSnapshot workbench.Snapshot
	if s.bench != nil {
		benchSnapshot = s.bench.Snapshot()
	}
	return DashboardDTO{
		Devices:        devices,
		SelectedDevice: deviceID(snapshot.Device.ConnString),
		Connection:     connectionDTO(snapshot),
		Card:           cardPointerDTO(snapshot.Card),
		Blocks:         blockDTOs(benchSnapshot),
		Keys:           sectorKeyDTOs(s.keyStore, snapshot.Card),
		Actions: []ActionDTO{
			{ID: "read", Label: "读取整卡", Enabled: false, Risk: "normal"},
			{ID: "save", Label: "保存 Dump", Enabled: false, Risk: "normal"},
			{ID: "write", Label: "写入卡片", Enabled: false, Risk: "danger"},
			{ID: "scan", Label: "扫描默认密钥", Enabled: false, Risk: "normal"},
		},
	}
}

// RefreshDevices updates the GUI list without disturbing the open Reader.
func (s *Service) RefreshDevices() ([]DeviceDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.deviceTimeout)
	defer cancel()
	devices, err := s.deviceManager.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := s.deviceManager.Snapshot()
	if snapshot.Device.ConnString != "" && !containsDevice(devices, snapshot.Device.ConnString) {
		devices = append(devices, snapshot.Device)
	}
	s.mu.Lock()
	s.devices = append([]nfc.DeviceInfo(nil), devices...)
	s.mu.Unlock()
	result := make([]DeviceDTO, 0, len(devices))
	for _, info := range devices {
		result = append(result, deviceDTO(info))
	}
	return result, nil
}

// ConnectDevice passes the exact user-selected connstring to DeviceManager.
func (s *Service) ConnectDevice(connString string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.deviceTimeout)
	defer cancel()
	return s.deviceManager.Connect(ctx, connString)
}

func (s *Service) DisconnectDevice() error { return s.deviceManager.Disconnect() }

// StartMockTask starts a cancellable background job and returns without blocking.
func (s *Service) StartMockTask() TaskDTO {
	id := fmt.Sprintf("mock-%04d", s.nextID.Add(1))
	ctx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	s.tasks[id] = &mockTask{cancel: cancel}
	s.mu.Unlock()

	result := TaskDTO{ID: id, Name: "模拟整卡读取", Status: "running"}
	go s.runMockTask(ctx, result)
	return result
}

// CancelMockTask requests cancellation. A successful call guarantees no completed event.
func (s *Service) CancelMockTask(taskID string) error {
	if taskID == "" {
		return ErrTaskIDRequired
	}

	s.mu.Lock()
	task, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	task.cancelRequested = true
	task.cancel()
	s.mu.Unlock()
	return nil
}

// Shutdown cancels all active mock work when the GUI exits.
func (s *Service) Shutdown() {
	s.mu.Lock()
	for _, task := range s.tasks {
		task.cancelRequested = true
		task.cancel()
	}
	for _, task := range s.operations {
		task.cancelRequested = true
		task.cancel()
	}
	s.mu.Unlock()
	s.deviceManager.Shutdown()
}

func (s *Service) runMockTask(ctx context.Context, task TaskDTO) {
	s.emitTaskEvent(task.ID, TaskEventStarted, 0, "任务已开始")
	ticker := time.NewTicker(s.config.stepInterval)
	defer ticker.Stop()

	for step := 1; step <= s.config.steps; step++ {
		select {
		case <-ctx.Done():
			s.finishTask(task.ID, true)
			return
		case <-ticker.C:
			progress := step * 100 / s.config.steps
			if progress < 100 {
				s.emitTaskEvent(task.ID, TaskEventProgress, progress, fmt.Sprintf("正在读取模拟 block：%d%%", progress))
				continue
			}
			s.finishTask(task.ID, false)
			return
		}
	}
}

func (s *Service) finishTask(taskID string, contextCancelled bool) {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return
	}
	cancelled := contextCancelled || task.cancelRequested
	delete(s.tasks, taskID)
	s.mu.Unlock()

	if cancelled {
		s.emitTaskEvent(taskID, TaskEventCancelled, 0, "任务已取消")
		return
	}
	s.emitTaskEvent(taskID, TaskEventProgress, 100, "模拟数据读取完成")
	s.emitTaskEvent(taskID, TaskEventCompleted, 100, "任务已完成")
}

func (s *Service) emitTaskEvent(taskID, eventType string, progress int, message string) {
	s.emitter.Emit(MockTaskEventName, TaskEventDTO{
		TaskID:   taskID,
		Type:     eventType,
		Progress: progress,
		Message:  message,
		Time:     time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func mockBlockHex(block int) string {
	if block == 0 {
		return "04 A1 B2 C3 D4 E5 F6 88 04 00 00 00 00 00 00 00"
	}
	if block%4 == 3 {
		return "FF FF FF FF FF FF FF 07 80 69 FF FF FF FF FF FF"
	}
	return fmt.Sprintf("%02X 00 00 00 00 00 00 00 00 00 00 00 00 00 00 %02X", block, 255-block)
}

type deviceEventSink struct{ emitter EventEmitter }

func (s deviceEventSink) DeviceStateChanged(snapshot device.Snapshot) {
	s.emitter.Emit(DeviceEventName, DeviceStateEventDTO{
		Connection: connectionDTO(snapshot),
		Device:     deviceDTO(snapshot.Device),
		Card:       cardPointerDTO(snapshot.Card),
		Time:       snapshot.UpdatedAt.Format(time.RFC3339Nano),
	})
}

func (s deviceEventSink) CardChanged(event device.CardEvent) {
	s.emitter.Emit(CardEventName, CardEventDTO{
		Type: string(event.Type),
		Card: cardDTO(event.Card),
		Time: event.Time.Format(time.RFC3339Nano),
	})
}

func connectionDTO(snapshot device.Snapshot) ConnectionDTO {
	labels := map[device.Status]string{
		device.StatusDisconnected: "未连接",
		device.StatusConnecting:   "正在连接",
		device.StatusReady:        "设备已就绪",
		device.StatusPolling:      "正在检测卡片",
		device.StatusBusy:         "设备忙",
		device.StatusRecovering:   "正在恢复连接",
		device.StatusError:        "设备错误",
	}
	detail := snapshot.Detail
	if snapshot.ErrorMessage != "" {
		detail = snapshot.ErrorMessage
	}
	return ConnectionDTO{
		Status:    string(snapshot.Status),
		Label:     labels[snapshot.Status],
		Detail:    detail,
		ErrorCode: errorCodeName(snapshot.ErrorCode),
	}
}

func cardPointerDTO(card *nfc.CardInfo) CardDTO {
	if card == nil {
		return CardDTO{Present: false, UID: "—", ATQA: "—", SAK: "—", Type: "未知"}
	}
	return cardDTO(*card)
}

func cardDTO(card nfc.CardInfo) CardDTO {
	cardType, label := string(nfc.InferCardType(card)), "未知"
	switch nfc.InferCardType(card) {
	case nfc.CardTypeMIFAREMini:
		label = "MIFARE Mini"
	case nfc.CardTypeMIFAREClassic1K:
		label = "MIFARE Classic 1K"
	case nfc.CardTypeMIFAREClassic4K:
		label = "MIFARE Classic 4K"
	}
	return CardDTO{
		Present:   true,
		UID:       formatHex(card.UID),
		UIDLength: len(card.UID),
		ATQA:      formatHex(card.ATQA[:]),
		SAK:       fmt.Sprintf("%02X", card.SAK),
		Type:      label,
		TypeCode:  cardType,
		Inferred:  cardType != string(nfc.CardTypeUnknown),
	}
}

func deviceDTO(info nfc.DeviceInfo) DeviceDTO {
	name := info.Name
	if name == "" {
		name = info.ConnString
	}
	return DeviceDTO{
		ID:         deviceID(info.ConnString),
		Name:       name,
		ConnString: info.ConnString,
		Transport:  transportLabel(info.ConnString),
		Available:  info.ConnString != "",
		Nested:     device.CapabilitiesFor(info).Nested,
		Darkside:   device.CapabilitiesFor(info).Darkside,
		Hardnested: device.CapabilitiesFor(info).Hardnested,
	}
}

func deviceID(connString string) string {
	if connString == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(connString))
	return fmt.Sprintf("reader-%x", sum[:8])
}

func transportLabel(connString string) string {
	driver := connString
	if index := strings.IndexByte(driver, ':'); index >= 0 {
		driver = driver[:index]
	}
	switch driver {
	case "pn532_uart":
		return "UART / PN532"
	case "pn53x_usb":
		return "USB / PN53x"
	case "acr122_usb":
		return "USB / ACR122"
	case "acr122_pcsc", "pcsc":
		return "PC/SC"
	case "":
		return "未知连接"
	default:
		return driver
	}
}

func formatHex(data []byte) string {
	parts := make([]string, len(data))
	for index, value := range data {
		parts[index] = fmt.Sprintf("%02X", value)
	}
	return strings.Join(parts, " ")
}

func containsDevice(devices []nfc.DeviceInfo, connString string) bool {
	for _, info := range devices {
		if info.ConnString == connString {
			return true
		}
	}
	return false
}

func errorCodeName(code nfc.ErrorCode) string {
	switch code {
	case nfc.CodeNoCard:
		return "no_card"
	case nfc.CodeTimeout:
		return "timeout"
	case nfc.CodeAuthenticationFailed:
		return "authentication_failed"
	case nfc.CodeDeviceDisconnected:
		return "device_disconnected"
	case nfc.CodeIO:
		return "io"
	case nfc.CodeCanceled:
		return "canceled"
	case nfc.CodeInvalidArgument:
		return "invalid_argument"
	case nfc.CodeUnsupported:
		return "unsupported"
	case nfc.CodeBusy:
		return "busy"
	case nfc.CodePermission:
		return "permission"
	case nfc.CodeCardChanged:
		return "card_changed"
	case nfc.CodeNotAuthenticated:
		return "not_authenticated"
	case nfc.CodeVerificationFailed:
		return "verification_failed"
	case nfc.CodeInternal:
		return "internal"
	default:
		return ""
	}
}
