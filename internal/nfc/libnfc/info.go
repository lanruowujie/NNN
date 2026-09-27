package libnfc

// RuntimeInfo is safe to expose through diagnostics; it contains no native
// handles or user-specific paths.
type RuntimeInfo struct {
	Available bool     `json:"available"`
	Version   string   `json:"version,omitempty"`
	Drivers   []string `json:"drivers"`
	Error     string   `json:"error,omitempty"`
}

// Info reports the linked native runtime without opening or enumerating a
// reader. The first release deliberately enables only PN532 UART.
func Info() RuntimeInfo {
	version, result := platformRuntimeVersion()
	info := RuntimeInfo{Available: result.code == nativeOK, Version: version, Drivers: []string{"pn532_uart"}}
	if result.code != nativeOK {
		info.Error = result.detail
	}
	return info
}
