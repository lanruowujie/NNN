package diagnostic_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/BennyThink/NFCX/internal/diagnostic"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

func TestRunWithoutBundledRuntimeReportsActionableFailures(t *testing.T) {
	report := diagnostic.Run(context.Background(), runtimebundle.NewLocator(t.TempDir()), "pn532_uart:/dev/example")
	if report.Healthy {
		t.Fatal("development runtime unexpectedly reported healthy")
	}
	if report.Manifest.Found || report.Manifest.Detail == "" {
		t.Fatalf("manifest status = %#v", report.Manifest)
	}
	if len(report.Engines) != 4 || len(report.LibNFC.Drivers) != 1 {
		t.Fatalf("report omitted runtime checks: %#v", report)
	}
	if report.Build.OS != runtime.GOOS || report.Build.Architecture != runtime.GOARCH {
		t.Fatalf("build platform = %#v", report.Build)
	}
}
