package attack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"github.com/BennyThink/NFCX/internal/device"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

const HardnestedVersion = "0.10.9"

var (
	hardnestedExecutable = runtimebundle.Executable{ID: "mfoc-hardnested", FileName: "mfoc-hardnested", WindowsFileName: "mfoc-hardnested.exe"}
	hardnestedVersionRE  = regexp.MustCompile(`(?i)mfoc-hardnested version\s+([0-9]+(?:\.[0-9]+)+)`)
)

// HardnestedEngine runs the pinned mfoc-hardnested fork with only NFCX-verified
// seed keys. Its result is accepted only through the same validated dump path
// used by the MFOC adapter.
type HardnestedEngine struct {
	locator    *runtimebundle.Locator
	runner     *processRunner
	invocation MFoCInvocation
	extraEnv   map[string]string // tests only; production constructors leave it empty
}

func NewHardnestedEngine(locator *runtimebundle.Locator, invocation MFoCInvocation) *HardnestedEngine {
	invocation.Card = invocation.Card.Clone()
	invocation.KnownKeys = append([]MFoCKnownKey(nil), invocation.KnownKeys...)
	return &HardnestedEngine{locator: locator, runner: newProcessRunner(), invocation: invocation}
}

func (e *HardnestedEngine) Name() string { return hardnestedExecutable.ID }

func (e *HardnestedEngine) Available(ctx context.Context) error {
	if ctx == nil || e == nil || e.locator == nil || e.runner == nil {
		return errors.New("mfoc-hardnested adapter is unavailable")
	}
	path, err := e.locator.Resolve(hardnestedExecutable)
	if err != nil {
		return err
	}
	versionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := e.runner.run(versionCtx, e.Name(), command{Path: path, Args: []string{"-h"}}, nil)
	if err != nil {
		return fmt.Errorf("run mfoc-hardnested version check: %w", err)
	}
	version := parseHardnestedVersion(result.Stdout, result.Stderr)
	if version == "" {
		return errors.New("mfoc-hardnested version output is not recognized")
	}
	if version != HardnestedVersion {
		return fmt.Errorf("mfoc-hardnested version %s is unsupported; expected %s", version, HardnestedVersion)
	}
	if !hardnestedHelpSupportsRequiredOptions(result.Stdout, result.Stderr) {
		return errors.New("mfoc-hardnested help does not advertise required -C, -F, -k, and -O options")
	}
	return nil
}

func (e *HardnestedEngine) Run(ctx context.Context, request AttackRequest, emit func(AttackEvent)) (AttackResult, error) {
	result := AttackResult{Engine: e.Name(), ExitCode: -1, TemporaryDir: request.WorkingDir, Version: HardnestedVersion}
	if ctx == nil {
		err := errors.New("mfoc-hardnested context is required")
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: HardnestedVersion, Detail: err.Error(), Cause: err}
	}
	if request.WorkingDir == "" || !filepath.IsAbs(request.WorkingDir) {
		err := errors.New("mfoc-hardnested working directory must be an absolute task temporary directory")
		return result, &Error{Code: CodeTemporary, Engine: e.Name(), ExitCode: -1, Version: HardnestedVersion, Detail: err.Error(), Cause: err}
	}
	if err := validateMFoCInvocation(e.invocation); err != nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: HardnestedVersion, Detail: err.Error(), Cause: err}
	}
	if !device.CapabilitiesFor(request.Device).Hardnested {
		err := errors.New("released reader is not enabled for Hardnested attacks")
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: HardnestedVersion, Detail: err.Error(), Cause: err}
	}
	path, err := e.locator.Resolve(hardnestedExecutable)
	if err != nil {
		return result, &Error{Code: CodeUnavailable, Engine: e.Name(), ExitCode: -1, Detail: err.Error(), Cause: err}
	}
	outputPath := filepath.Join(request.WorkingDir, "hardnested-result.mfd")
	if emit != nil {
		emit(AttackEvent{Engine: e.Name(), State: StateStarting, Message: "starting bundled mfoc-hardnested"})
	}
	stream := newMFoCStreamEmitter(emit, mfocSectorCount(e.invocation.Card), e.Name())
	environment := map[string]string{
		"LIBNFC_DEVICE":         request.Device.ConnString,
		"LIBNFC_AUTO_SCAN":      "false",
		"LIBNFC_INTRUSIVE_SCAN": "false",
	}
	for key, value := range e.extraEnv {
		environment[key] = value
	}
	process, runErr := e.runner.run(ctx, e.Name(), command{
		Path: path,
		Args: hardnestedArguments(e.invocation.KnownKeys, outputPath),
		Dir:  request.WorkingDir,
		Env:  environment,
	}, stream.emit)
	stream.flush()
	result.ExitCode = process.ExitCode
	result.StartedAt = process.StartedAt
	result.FinishedAt = process.EndedAt
	result.Stdout = process.Stdout
	result.Stderr = process.Stderr
	if parsed := parseHardnestedVersion(process.Stdout, process.Stderr); parsed != "" {
		result.Version = parsed
	}
	if runErr != nil {
		return result, enrichEngineError(runErr, result)
	}
	if emit != nil {
		emit(AttackEvent{Engine: e.Name(), State: StateValidatingResult, Message: "validating mfoc-hardnested dump structure"})
	}
	raw, err := readMFoCOutput(e.invocation.Card, outputPath)
	if err != nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: result.ExitCode, Version: result.Version, Detail: "mfoc-hardnested output dump cannot be accepted: " + err.Error(), Cause: err}
	}
	image, err := validateMFoCDump(e.invocation.Card, raw)
	if err != nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: result.ExitCode, Version: result.Version, Detail: "mfoc-hardnested output dump is invalid: " + err.Error(), Cause: err}
	}
	validatedRaw, _ := image.Raw()
	result.Artifacts = []AttackArtifact{{Name: "hardnested-result.mfd", MediaType: "application/vnd.nfcx.mifare-dump", Data: validatedRaw}}
	result.Validated = true
	return result, nil
}

func hardnestedArguments(known []MFoCKnownKey, outputPath string) []string {
	return append([]string{"-C", "-F"}, mfocArguments(known, outputPath)...)
}

func parseHardnestedVersion(streams ...[]byte) string {
	for _, stream := range streams {
		match := hardnestedVersionRE.FindSubmatch(stream)
		if len(match) == 2 {
			return string(match[1])
		}
	}
	return ""
}

func hardnestedHelpSupportsRequiredOptions(streams ...[]byte) bool {
	help := bytes.Join(streams, []byte{'\n'})
	return bytes.Contains(help, []byte("-C")) && bytes.Contains(help, []byte("-F")) &&
		bytes.Contains(help, []byte("-k key")) && bytes.Contains(help, []byte("-O output"))
}
