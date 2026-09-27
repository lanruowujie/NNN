package attack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

var fixtureExecutable = runtimebundle.Executable{
	ID:              "nfcx-fake-engine",
	FileName:        "nfcx-fake-engine",
	WindowsFileName: "nfcx-fake-engine.exe",
}

// FixtureEngine is a development and acceptance-test adapter. It is available
// only when its separately built executable exists in the controlled runtime.
type FixtureEngine struct {
	locator  *runtimebundle.Locator
	runner   *processRunner
	mode     string
	echo     string
	steps    int
	interval time.Duration
}

func NewFixtureEngine(locator *runtimebundle.Locator) *FixtureEngine {
	return &FixtureEngine{
		locator: locator, runner: newProcessRunner(), mode: "success", echo: "argument with spaces · 路径测试",
		steps: 20, interval: 150 * time.Millisecond,
	}
}

func (e *FixtureEngine) Name() string { return fixtureExecutable.ID }

func (e *FixtureEngine) Available(context.Context) error {
	if e == nil || e.locator == nil {
		return errors.New("fixture engine locator is unavailable")
	}
	_, err := e.locator.Resolve(fixtureExecutable)
	return err
}

func (e *FixtureEngine) Run(ctx context.Context, request AttackRequest, emit func(AttackEvent)) (AttackResult, error) {
	result := AttackResult{Engine: e.Name(), ExitCode: -1, TemporaryDir: request.WorkingDir}
	path, err := e.locator.Resolve(fixtureExecutable)
	if err != nil {
		return result, &Error{Code: CodeUnavailable, Engine: e.Name(), ExitCode: -1, Detail: err.Error(), Cause: err}
	}
	versionCtx, cancelVersion := context.WithTimeout(ctx, 5*time.Second)
	versionResult, versionErr := e.runner.run(versionCtx, e.Name(), command{Path: path, Args: []string{"--version"}, Dir: request.WorkingDir}, nil)
	cancelVersion()
	if versionErr == nil {
		result.Version = strings.TrimSpace(string(versionResult.Stdout))
	}
	if result.Version == "" {
		result.Version = "unknown"
	}
	if emit != nil {
		emit(AttackEvent{Engine: e.Name(), State: StateStarting, Message: "starting bundled fixture engine"})
	}
	outputPath := filepath.Join(request.WorkingDir, "fixture-result.json")
	mode := e.mode
	if mode == "" {
		mode = "success"
	}
	processResult, runErr := e.runner.run(ctx, e.Name(), command{
		Path: path,
		Args: []string{
			"--mode", mode, "--output", outputPath, "--echo", e.echo,
			"--steps", fmt.Sprintf("%d", e.steps), "--interval", e.interval.String(),
		},
		Dir: request.WorkingDir,
		Env: map[string]string{
			"LIBNFC_DEVICE":         request.Device.ConnString,
			"LIBNFC_AUTO_SCAN":      "false",
			"LIBNFC_INTRUSIVE_SCAN": "false",
		},
	}, emit)
	result.ExitCode = processResult.ExitCode
	result.StartedAt = processResult.StartedAt
	result.FinishedAt = processResult.EndedAt
	result.Stdout = processResult.Stdout
	result.Stderr = processResult.Stderr
	if runErr != nil {
		return result, enrichEngineError(runErr, result)
	}
	if emit != nil {
		emit(AttackEvent{Engine: e.Name(), State: StateValidatingResult, Message: "validating fixture result"})
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: result.ExitCode, Version: result.Version, Detail: "expected result file is missing", Cause: err}
	}
	var payload struct {
		OK     bool   `json:"ok"`
		Device string `json:"device"`
		Echo   string `json:"echo"`
	}
	if err := json.Unmarshal(contents, &payload); err != nil || !payload.OK {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: result.ExitCode, Version: result.Version, Detail: "fixture result is invalid", Cause: err}
	}
	if payload.Device != request.Device.ConnString || payload.Echo != e.echo {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: result.ExitCode, Version: result.Version, Detail: "fixture result did not preserve arguments or connstring"}
	}
	result.Validated = true
	return result, nil
}

func enrichEngineError(err error, result AttackResult) error {
	var engineErr *Error
	if errors.As(err, &engineErr) {
		clone := *engineErr
		clone.Engine = result.Engine
		clone.ExitCode = result.ExitCode
		clone.Version = result.Version
		return &clone
	}
	return &Error{Code: CodeExit, Engine: result.Engine, ExitCode: result.ExitCode, Version: result.Version, Detail: fmt.Sprintf("process failed: %v", err), Cause: err}
}
