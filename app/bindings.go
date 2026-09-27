package app

import "github.com/BennyThink/NFCX/internal/diagnostic"
import "github.com/BennyThink/NFCX/internal/update"

// Bindings is the only object exposed to the Wails frontend.
type Bindings struct {
	service *Service
	quit    func()
}

// GetDashboard returns the initial mock dashboard state.
func (b *Bindings) GetDashboard() DashboardDTO {
	return b.service.Dashboard()
}

func (b *Bindings) GetDiagnostics() diagnostic.Report { return b.service.Diagnostics() }

func (b *Bindings) OpenCommandTerminal() error { return b.service.OpenCommandTerminal() }

func (b *Bindings) GetTelemetrySettings() TelemetrySettingsDTO { return b.service.TelemetrySettings() }
func (b *Bindings) SetTelemetryEnabled(enabled bool) (TelemetrySettingsDTO, error) {
	return b.service.SetTelemetryEnabled(enabled)
}
func (b *Bindings) TrackTelemetry(event string) { b.service.TrackTelemetry(event) }
func (b *Bindings) CheckForUpdates() (update.Snapshot, error) {
	return b.service.CheckForUpdates()
}
func (b *Bindings) GetUpdateStatus() update.Snapshot         { return b.service.UpdateStatus() }
func (b *Bindings) DownloadUpdate() (update.Snapshot, error) { return b.service.DownloadUpdate() }
func (b *Bindings) SetAutomaticUpdateDownload(enabled bool) (update.Snapshot, error) {
	return b.service.SetAutomaticUpdateDownload(enabled)
}
func (b *Bindings) RestartAndApplyUpdate() error {
	if err := b.service.RestartAndApplyUpdate(); err != nil {
		return err
	}
	if b.quit != nil {
		b.quit()
	}
	return nil
}

func (b *Bindings) RefreshDevices() ([]DeviceDTO, error) {
	return b.service.RefreshDevices()
}

func (b *Bindings) ConnectDevice(connString string) error {
	return b.service.ConnectDevice(connString)
}

func (b *Bindings) DisconnectDevice() error {
	return b.service.DisconnectDevice()
}

func (b *Bindings) GetKeyCatalog() []KeyDTO { return b.service.KeyCatalog() }

func (b *Bindings) AddKey(value string) ([]KeyDTO, error) { return b.service.AddKey(value) }

func (b *Bindings) DeleteKey(id string) ([]KeyDTO, error) { return b.service.DeleteKey(id) }

func (b *Bindings) ImportKeyDictionary() (ImportReportDTO, error) {
	return b.service.ImportKeyDictionary()
}

func (b *Bindings) ExportKeyDictionary() (ExportResultDTO, error) {
	return b.service.ExportKeyDictionary()
}

func (b *Bindings) GetWorkbench() WorkbenchDTO { return b.service.Workbench() }

func (b *Bindings) LoadDump(discardDirty bool) (WorkbenchDTO, error) {
	return b.service.LoadDump(discardDirty)
}

func (b *Bindings) SaveDump() (SaveResultDTO, error) { return b.service.SaveDump() }

func (b *Bindings) EditBlockHex(block int, value string) (WorkbenchDTO, error) {
	return b.service.EditBlockHex(block, value)
}

func (b *Bindings) EditBlockASCII(block int, value string) (WorkbenchDTO, error) {
	return b.service.EditBlockASCII(block, value)
}

func (b *Bindings) GetEditableBlock(block int) (EditableBlockDTO, error) {
	return b.service.EditableBlock(block)
}

func (b *Bindings) UndoWorkbench() (WorkbenchDTO, error) { return b.service.UndoWorkbench() }

func (b *Bindings) RevertWorkbench() (WorkbenchDTO, error) { return b.service.RevertWorkbench() }

func (b *Bindings) ClearWorkbench() WorkbenchDTO { return b.service.ClearWorkbench() }

func (b *Bindings) StartKeyScan(discardDirty bool) (TaskDTO, error) {
	return b.service.StartKeyScan(discardDirty)
}

func (b *Bindings) StartKeyRecovery(request KeyRecoveryStartRequestDTO) (TaskDTO, error) {
	return b.service.StartKeyRecovery(request)
}

func (b *Bindings) StartReadCard(discardDirty bool) (TaskDTO, error) {
	return b.service.StartReadCard(discardDirty)
}

func (b *Bindings) CancelTask(taskID string) error { return b.service.CancelTask(taskID) }

func (b *Bindings) PreflightWrite(request WritePreflightRequestDTO) (WritePreflightDTO, error) {
	return b.service.PreflightWrite(request)
}

func (b *Bindings) StartWrite(request WriteStartRequestDTO) (TaskDTO, error) {
	return b.service.StartWrite(request)
}

func (b *Bindings) PreflightUIDWrite(uid string) (UIDWritePreflightDTO, error) {
	return b.service.PreflightUIDWrite(uid)
}

func (b *Bindings) StartUIDWrite(request UIDWriteStartRequestDTO) (TaskDTO, error) {
	return b.service.StartUIDWrite(request)
}

func (b *Bindings) GetExternalEngineFixtureStatus() ExternalEngineStatusDTO {
	return b.service.ExternalEngineFixtureStatus()
}

func (b *Bindings) StartExternalEngineFixture() (TaskDTO, error) {
	return b.service.StartExternalEngineFixture()
}

func (b *Bindings) GetMFoCStatus() ExternalEngineStatusDTO { return b.service.MFoCStatus() }

func (b *Bindings) StartMFoC(request MFoCStartRequestDTO) (TaskDTO, error) {
	return b.service.StartMFoC(request)
}

func (b *Bindings) GetMFCUKStatus() ExternalEngineStatusDTO { return b.service.MFCUKStatus() }

func (b *Bindings) StartMFCUKPipeline(request MFCUKStartRequestDTO) (TaskDTO, error) {
	return b.service.StartMFCUKPipeline(request)
}

// StartMockTask starts a non-blocking simulated operation.
func (b *Bindings) StartMockTask() TaskDTO {
	return b.service.StartMockTask()
}

// CancelMockTask cancels the matching simulated operation.
func (b *Bindings) CancelMockTask(taskID string) error {
	return b.service.CancelMockTask(taskID)
}
