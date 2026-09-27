// Package buildinfo exposes release metadata injected by the build pipeline.
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// These values are overridden with -ldflags for release builds. Keeping safe
// development defaults makes ordinary go test and wails dev builds useful.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
	Dirty     = "false"
)

const WailsVersion = "2.15.0"

type Info struct {
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	BuildDate    string `json:"buildDate"`
	Dirty        bool   `json:"dirty"`
	GoVersion    string `json:"goVersion"`
	WailsVersion string `json:"wailsVersion"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

func Current() Info {
	version, commit, buildDate := Version, Commit, BuildDate
	if details, ok := debug.ReadBuildInfo(); ok {
		if version == "dev" && details.Main.Version != "" && details.Main.Version != "(devel)" {
			version = details.Main.Version
		}
		for _, setting := range details.Settings {
			switch setting.Key {
			case "vcs.revision":
				if commit == "unknown" && setting.Value != "" {
					commit = setting.Value
				}
			case "vcs.time":
				if buildDate == "unknown" && setting.Value != "" {
					buildDate = setting.Value
				}
			}
		}
	}
	return Info{
		Version:      clean(version, "dev"),
		Commit:       clean(commit, "unknown"),
		BuildDate:    clean(buildDate, "unknown"),
		Dirty:        strings.EqualFold(strings.TrimSpace(Dirty), "true"),
		GoVersion:    runtime.Version(),
		WailsVersion: WailsVersion,
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	}
}

func clean(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
