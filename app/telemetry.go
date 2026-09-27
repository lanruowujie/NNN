package app

// TelemetrySettingsDTO intentionally exposes no installation identifier to the GUI.
type TelemetrySettingsDTO struct {
	Configured bool `json:"configured"`
	Enabled    bool `json:"enabled"`
}

func (s *Service) TelemetrySettings() TelemetrySettingsDTO {
	settings := s.telemetryStore.Settings()
	return TelemetrySettingsDTO{Configured: settings.Configured, Enabled: settings.Enabled}
}

func (s *Service) SetTelemetryEnabled(enabled bool) (TelemetrySettingsDTO, error) {
	settings, err := s.telemetryStore.SetEnabled(enabled)
	if err != nil {
		return TelemetrySettingsDTO{}, err
	}
	if enabled {
		s.telemetry.Track("app_started")
	}
	return TelemetrySettingsDTO{Configured: settings.Configured, Enabled: settings.Enabled}, nil
}

// TrackTelemetry accepts only a server-independent allowlisted name; it never blocks a GUI action.
func (s *Service) TrackTelemetry(event string) { s.telemetry.Track(event) }
