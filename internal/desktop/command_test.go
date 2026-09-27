package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/diagnostic"
)

func TestExecuteSelfCheckDoesNotStartGUI(t *testing.T) {
	var output bytes.Buffer
	err := Execute(nil, []string{"NFCX", "--self-check", "--json"}, &output)
	if !errors.Is(err, ErrSelfCheckFailed) {
		t.Fatalf("development self-check error = %v", err)
	}
	var report diagnostic.Report
	if decodeErr := json.Unmarshal(output.Bytes(), &report); decodeErr != nil {
		t.Fatalf("decode report: %v\n%s", decodeErr, output.String())
	}
	if report.Build.Version == "" || report.Healthy {
		t.Fatalf("report = %#v", report)
	}
}

func TestExecuteRejectsUnknownArguments(t *testing.T) {
	err := Execute(nil, []string{"NFCX", "--self-check", "--unexpected"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("unknown argument was accepted")
	}
}
