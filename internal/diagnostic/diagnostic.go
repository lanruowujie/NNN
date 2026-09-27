// Package diagnostic implements the shared GUI and headless release self-check.
package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/BennyThink/NFCX/internal/attack"
	"github.com/BennyThink/NFCX/internal/buildinfo"
	"github.com/BennyThink/NFCX/internal/nfc/libnfc"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

type Check struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type ManifestStatus struct {
	Found    bool   `json:"found"`
	Verified bool   `json:"verified"`
	Files    int    `json:"files"`
	Detail   string `json:"detail,omitempty"`
}

type Report struct {
	Healthy          bool               `json:"healthy"`
	GeneratedAt      string             `json:"generatedAt"`
	Build            buildinfo.Info     `json:"build"`
	LibNFC           libnfc.RuntimeInfo `json:"libnfc"`
	Engines          []Check            `json:"engines"`
	Manifest         ManifestStatus     `json:"manifest"`
	CurrentDevice    string             `json:"currentDevice,omitempty"`
	SupportedReaders []string           `json:"supportedReaders"`
}

func Run(ctx context.Context, locator *runtimebundle.Locator, currentDevice string) Report {
	if ctx == nil {
		ctx = context.Background()
	}
	if locator == nil {
		locator = runtimebundle.DefaultLocator()
	}
	report := Report{
		Healthy:          true,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		Build:            buildinfo.Current(),
		LibNFC:           libnfc.Info(),
		CurrentDevice:    redactUserPath(currentDevice),
		SupportedReaders: []string{"PN532 UART (including PN532 + FT232RL)"},
	}
	if !report.LibNFC.Available {
		report.Healthy = false
	}
	report.Engines = engineChecks(ctx, locator)
	for _, check := range report.Engines {
		if !check.OK {
			report.Healthy = false
		}
	}
	report.Manifest = verifyFirstManifest(locator)
	if !report.Manifest.Verified {
		report.Healthy = false
	}
	return report
}

func engineChecks(ctx context.Context, locator *runtimebundle.Locator) []Check {
	checks := []struct {
		id      string
		name    string
		version string
		check   func(context.Context) error
	}{
		{id: "mfoc", name: "MFOC", version: attack.MFoCVersion, check: func(ctx context.Context) error {
			return attack.NewMFoCEngine(locator, attack.MFoCInvocation{}).Available(ctx)
		}},
		{id: "mfcuk", name: "MFCUK", version: attack.MFCUKVersion, check: func(ctx context.Context) error {
			return attack.NewMFCUKEngine(locator, attack.MFCUKInvocation{}).Available(ctx)
		}},
		{id: "mfoc-hardnested", name: "MFOC-Hardnested", version: attack.HardnestedVersion, check: func(ctx context.Context) error {
			return attack.NewHardnestedEngine(locator, attack.MFoCInvocation{}).Available(ctx)
		}},
		{id: "nfc-mfsetuid", name: "nfc-mfsetuid", version: attack.MFSetUIDVersion, check: func(ctx context.Context) error {
			return attack.NewMFSetUIDEngine(locator, attack.MFSetUIDInvocation{}).Available(ctx)
		}},
	}
	result := make([]Check, 0, len(checks))
	for _, candidate := range checks {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := candidate.check(checkCtx)
		cancel()
		item := Check{ID: candidate.id, Name: candidate.name, OK: err == nil, Version: candidate.version}
		if err != nil {
			item.Detail = redactUserPath(err.Error())
		}
		result = append(result, item)
	}
	return result
}

func verifyFirstManifest(locator *runtimebundle.Locator) ManifestStatus {
	status := ManifestStatus{}
	for _, root := range locator.Roots() {
		path := filepath.Join(root, "manifest.json")
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			status.Detail = redactUserPath(err.Error())
			return status
		}
		status.Found = true
		verified, err := runtimebundle.VerifyManifest(root, path)
		if err != nil {
			status.Detail = redactUserPath(err.Error())
			return status
		}
		if verified.Manifest.Platform != runtime.GOOS+"-"+runtime.GOARCH {
			status.Detail = fmt.Sprintf("manifest platform %s does not match %s-%s", verified.Manifest.Platform, runtime.GOOS, runtime.GOARCH)
			return status
		}
		status.Verified = true
		status.Files = verified.Verified
		status.Detail = "runtime manifest and file hashes verified"
		return status
	}
	status.Detail = "runtime manifest not found"
	return status
}

func redactUserPath(value string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		value = strings.ReplaceAll(value, home, "<user-home>")
	}
	return value
}
