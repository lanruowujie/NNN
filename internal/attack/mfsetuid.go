package attack

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/BennyThink/NFCX/internal/nfc"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

const MFSetUIDVersion = "libnfc-1.8.0"

var mfsetuidExecutable = runtimebundle.Executable{
	ID: "nfc-mfsetuid", FileName: "nfc-mfsetuid", WindowsFileName: "nfc-mfsetuid.exe",
}

// MFSetUIDInvocation is the complete block 0 that the Gen1A helper will write.
// Passing all 16 bytes preserves manufacturer bytes 5..15; the upstream tool
// recalculates byte 4 before transmitting the block.
type MFSetUIDInvocation struct {
	Card   nfc.CardInfo
	Block0 [nfc.BlockSize]byte
}

type MFSetUIDEngine struct {
	locator    *runtimebundle.Locator
	runner     *processRunner
	invocation MFSetUIDInvocation
}

func NewMFSetUIDEngine(locator *runtimebundle.Locator, invocation MFSetUIDInvocation) *MFSetUIDEngine {
	invocation.Card = invocation.Card.Clone()
	return &MFSetUIDEngine{locator: locator, runner: newProcessRunner(), invocation: invocation}
}

func (e *MFSetUIDEngine) Name() string { return mfsetuidExecutable.ID }

func (e *MFSetUIDEngine) Available(ctx context.Context) error {
	if ctx == nil || e == nil || e.locator == nil || e.runner == nil {
		return errors.New("nfc-mfsetuid adapter is unavailable")
	}
	path, err := e.locator.Resolve(mfsetuidExecutable)
	if err != nil {
		return err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, runErr := e.runner.run(checkCtx, e.Name(), command{Path: path, Args: []string{"-h"}}, nil)
	if runErr != nil {
		return fmt.Errorf("run nfc-mfsetuid capability check: %w", runErr)
	}
	help := bytes.Join([][]byte{result.Stdout, result.Stderr}, []byte{'\n'})
	if !bytes.Contains(help, []byte("[UID|BLOCK0]")) || !bytes.Contains(help, []byte("16 HEX bytes")) {
		return errors.New("nfc-mfsetuid help does not advertise full block 0 input")
	}
	return nil
}

func (e *MFSetUIDEngine) Run(ctx context.Context, request AttackRequest, emit func(AttackEvent)) (AttackResult, error) {
	result := AttackResult{Engine: e.Name(), Version: MFSetUIDVersion, ExitCode: -1, TemporaryDir: request.WorkingDir}
	if ctx == nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: MFSetUIDVersion, Detail: "nfc-mfsetuid context is required"}
	}
	if request.WorkingDir == "" || !filepath.IsAbs(request.WorkingDir) {
		return result, &Error{Code: CodeTemporary, Engine: e.Name(), ExitCode: -1, Version: MFSetUIDVersion, Detail: "nfc-mfsetuid working directory must be an absolute task temporary directory"}
	}
	if nfc.InferCardType(e.invocation.Card) != nfc.CardTypeMIFAREClassic1K || len(e.invocation.Card.UID) != 4 {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: MFSetUIDVersion, Detail: "nfc-mfsetuid supports only 4-byte MIFARE Classic 1K cards"}
	}
	if !strings.HasPrefix(strings.ToLower(request.Device.ConnString), "pn532_uart:") {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: MFSetUIDVersion, Detail: "nfc-mfsetuid is enabled only for PN532 UART readers"}
	}
	path, err := e.locator.Resolve(mfsetuidExecutable)
	if err != nil {
		return result, &Error{Code: CodeUnavailable, Engine: e.Name(), ExitCode: -1, Version: MFSetUIDVersion, Detail: err.Error(), Cause: err}
	}
	if emit != nil {
		emit(AttackEvent{Engine: e.Name(), State: StateStarting, Message: "正在使用 Gen1A 后门序列写入 block 0"})
	}
	process, runErr := e.runner.run(ctx, e.Name(), command{
		Path: path,
		Args: mfsetuidArguments(e.invocation.Block0),
		Dir:  request.WorkingDir,
		Env: map[string]string{
			"LIBNFC_DEVICE": request.Device.ConnString, "LIBNFC_AUTO_SCAN": "false", "LIBNFC_INTRUSIVE_SCAN": "false",
		},
	}, emit)
	result.ExitCode, result.StartedAt, result.FinishedAt = process.ExitCode, process.StartedAt, process.EndedAt
	result.Stdout, result.Stderr = process.Stdout, process.Stderr
	if runErr != nil {
		return result, enrichEngineError(runErr, result)
	}
	// Upstream nfc-mfsetuid exits zero even when a card rejects an individual
	// raw frame. The caller must re-open the reader and verify the full block.
	result.Validated = true
	return result, nil
}

func mfsetuidArguments(block0 [nfc.BlockSize]byte) []string {
	// Keep verbose output enabled. The pinned upstream utility exits zero even
	// when a Gen1A unlock or write frame is rejected, so these frame-level
	// diagnostics are required to explain the later authoritative readback.
	return []string{strings.ToUpper(hex.EncodeToString(block0[:]))}
}
