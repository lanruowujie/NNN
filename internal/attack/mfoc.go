package attack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/mifare"
	"github.com/BennyThink/NFCX/internal/nfc"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

const MFoCVersion = "0.10.7"

var (
	mfocExecutable = runtimebundle.Executable{ID: "mfoc", FileName: "mfoc", WindowsFileName: "mfoc.exe"}
	mfocVersionRE  = regexp.MustCompile(`(?i)mfoc version\s+([0-9]+(?:\.[0-9]+)+)`)
	mfocSectorRE   = regexp.MustCompile(`(?i)sector(?:\s*:)?\s*([0-9]{1,2})`)
)

type MFoCKnownKey struct {
	Sector int
	Type   nfc.KeyType
	Value  nfc.Key
}

// MFoCInvocation is immutable per task. Keeping algorithm-specific input on
// the adapter preserves the generic AttackRequest boundary used by other
// external engines.
type MFoCInvocation struct {
	Card      nfc.CardInfo
	KnownKeys []MFoCKnownKey
}

type MFoCEngine struct {
	locator    *runtimebundle.Locator
	runner     *processRunner
	invocation MFoCInvocation
	extraEnv   map[string]string // tests only; production constructors leave it empty
}

func NewMFoCEngine(locator *runtimebundle.Locator, invocation MFoCInvocation) *MFoCEngine {
	invocation.Card = invocation.Card.Clone()
	invocation.KnownKeys = append([]MFoCKnownKey(nil), invocation.KnownKeys...)
	return &MFoCEngine{locator: locator, runner: newProcessRunner(), invocation: invocation}
}

func (e *MFoCEngine) Name() string { return mfocExecutable.ID }

func (e *MFoCEngine) Available(ctx context.Context) error {
	if ctx == nil || e == nil || e.locator == nil || e.runner == nil {
		return errors.New("mfoc adapter is unavailable")
	}
	path, err := e.locator.Resolve(mfocExecutable)
	if err != nil {
		return err
	}
	versionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := e.runner.run(versionCtx, e.Name(), command{Path: path, Args: []string{"-h"}}, nil)
	if err != nil {
		return fmt.Errorf("run mfoc version check: %w", err)
	}
	version := parseMFoCVersion(result.Stdout, result.Stderr)
	if version == "" {
		return errors.New("mfoc version output is not recognized")
	}
	if version != MFoCVersion {
		return fmt.Errorf("mfoc version %s is unsupported; expected %s", version, MFoCVersion)
	}
	if !mfocHelpSupportsKnownKey(result.Stdout, result.Stderr) {
		return errors.New("mfoc help does not advertise the required repeatable -k key option")
	}
	return nil
}

func (e *MFoCEngine) Run(ctx context.Context, request AttackRequest, emit func(AttackEvent)) (AttackResult, error) {
	result := AttackResult{Engine: e.Name(), ExitCode: -1, TemporaryDir: request.WorkingDir, Version: MFoCVersion}
	if ctx == nil {
		err := errors.New("mfoc context is required")
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: MFoCVersion, Detail: err.Error(), Cause: err}
	}
	if request.WorkingDir == "" || !filepath.IsAbs(request.WorkingDir) {
		err := errors.New("mfoc working directory must be an absolute task temporary directory")
		return result, &Error{Code: CodeTemporary, Engine: e.Name(), ExitCode: -1, Version: MFoCVersion, Detail: err.Error(), Cause: err}
	}
	if err := e.validateInvocation(); err != nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: MFoCVersion, Detail: err.Error(), Cause: err}
	}
	if !device.CapabilitiesFor(request.Device).Nested {
		err := errors.New("released reader is not enabled for Nested attacks")
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: MFoCVersion, Detail: err.Error(), Cause: err}
	}
	path, err := e.locator.Resolve(mfocExecutable)
	if err != nil {
		return result, &Error{Code: CodeUnavailable, Engine: e.Name(), ExitCode: -1, Detail: err.Error(), Cause: err}
	}
	outputPath := filepath.Join(request.WorkingDir, "mfoc-result.mfd")
	if emit != nil {
		emit(AttackEvent{Engine: e.Name(), State: StateStarting, Message: "starting bundled mfoc"})
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
		Args: mfocArguments(e.invocation.KnownKeys, outputPath),
		Dir:  request.WorkingDir,
		Env:  environment,
	}, stream.emit)
	stream.flush()
	result.ExitCode = process.ExitCode
	result.StartedAt = process.StartedAt
	result.FinishedAt = process.EndedAt
	result.Stdout = process.Stdout
	result.Stderr = process.Stderr
	if parsed := parseMFoCVersion(process.Stdout, process.Stderr); parsed != "" {
		result.Version = parsed
	}
	if runErr != nil {
		return result, enrichEngineError(runErr, result)
	}
	if emit != nil {
		emit(AttackEvent{Engine: e.Name(), State: StateValidatingResult, Message: "validating mfoc dump structure"})
	}
	raw, err := readMFoCOutput(e.invocation.Card, outputPath)
	if err != nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: result.ExitCode, Version: result.Version, Detail: "mfoc output dump cannot be accepted: " + err.Error(), Cause: err}
	}
	image, err := validateMFoCDump(e.invocation.Card, raw)
	if err != nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: result.ExitCode, Version: result.Version, Detail: "mfoc output dump is invalid: " + err.Error(), Cause: err}
	}
	validatedRaw, _ := image.Raw()
	result.Artifacts = []AttackArtifact{{Name: "mfoc-result.mfd", MediaType: "application/vnd.nfcx.mifare-dump", Data: validatedRaw}}
	result.Validated = true
	return result, nil
}

func readMFoCOutput(card nfc.CardInfo, path string) ([]byte, error) {
	layout, err := mfocLayout(card)
	if err != nil {
		return nil, err
	}
	wanted, _ := layout.DumpSize()
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("mfoc output is not a regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) == wanted {
		return raw, nil
	}
	// MFOC 0.10.7 writes sizeof(mifare_classic_tag), a 256-block/4096-byte
	// static structure, even for a 1K card. For a 1K target accept only the
	// documented first 1024 bytes and require the unused 3072-byte tail to be
	// entirely zero, so a genuine 4K dump can never be silently truncated.
	if wanted == mifare.Classic1KDumpSize && len(raw) == mifare.Classic4KDumpSize {
		for _, value := range raw[wanted:] {
			if value != 0 {
				return nil, errors.New("1K mfoc output has non-zero data in its 4K compatibility padding")
			}
		}
		return append([]byte(nil), raw[:wanted]...), nil
	}
	return nil, fmt.Errorf("expected %d bytes for target card, got %d", wanted, info.Size())
}

func (e *MFoCEngine) validateInvocation() error {
	if e == nil {
		return errors.New("mfoc invocation is unavailable")
	}
	return validateMFoCInvocation(e.invocation)
}

func validateMFoCInvocation(invocation MFoCInvocation) error {
	if len(invocation.KnownKeys) == 0 {
		return errors.New("at least one NFCX-verified key is required")
	}
	layout, err := mfocLayout(invocation.Card)
	if err != nil {
		return err
	}
	sectors, _ := layout.SectorCount()
	for _, known := range invocation.KnownKeys {
		if known.Sector < 0 || known.Sector >= sectors || !known.Type.Valid() {
			return fmt.Errorf("invalid verified key slot sector=%d type=%d", known.Sector, known.Type)
		}
	}
	return nil
}

func mfocArguments(known []MFoCKnownKey, outputPath string) []string {
	unique := make(map[nfc.Key]struct{}, len(known))
	values := make([]string, 0, len(known))
	for _, entry := range known {
		if _, exists := unique[entry.Value]; exists {
			continue
		}
		unique[entry.Value] = struct{}{}
		values = append(values, fmt.Sprintf("%012X", entry.Value))
	}
	sort.Strings(values)
	arguments := make([]string, 0, len(values)*2+2)
	for _, value := range values {
		arguments = append(arguments, "-k", value)
	}
	return append(arguments, "-O", outputPath)
}

func parseMFoCVersion(streams ...[]byte) string {
	for _, stream := range streams {
		match := mfocVersionRE.FindSubmatch(stream)
		if len(match) == 2 {
			return string(match[1])
		}
	}
	return ""
}

func mfocHelpSupportsKnownKey(streams ...[]byte) bool {
	return bytes.Contains(bytes.Join(streams, []byte{'\n'}), []byte("[-k key]..."))
}

func validateMFoCDump(card nfc.CardInfo, raw []byte) (mifare.Dump, error) {
	layout, err := mfocLayout(card)
	if err != nil {
		return mifare.Dump{}, err
	}
	wanted, _ := layout.DumpSize()
	if len(raw) != wanted {
		return mifare.Dump{}, fmt.Errorf("expected %d bytes for target card, got %d", wanted, len(raw))
	}
	image, err := mifare.ParseRaw(raw)
	if err != nil {
		return mifare.Dump{}, err
	}
	if image.Layout != layout {
		return mifare.Dump{}, errors.New("dump layout does not match target card")
	}
	if err := image.ValidateTrailers(); err != nil {
		return mifare.Dump{}, err
	}
	if len(card.UID) == 4 {
		if !bytes.Equal(image.Blocks[0].Data[:4], card.UID) {
			return mifare.Dump{}, errors.New("manufacturer block UID does not match target card")
		}
		if _, err := image.ValidateBCC(4); err != nil {
			return mifare.Dump{}, err
		}
	}
	return image, nil
}

func mfocLayout(card nfc.CardInfo) (mifare.Layout, error) {
	switch nfc.InferCardType(card) {
	case nfc.CardTypeMIFAREClassic1K:
		return mifare.Classic1K, nil
	case nfc.CardTypeMIFAREClassic4K:
		return mifare.Classic4K, nil
	default:
		return 0, errors.New("mfoc supports only MIFARE Classic 1K/4K in NFCX")
	}
}

func mfocSectorCount(card nfc.CardInfo) int {
	layout, err := mfocLayout(card)
	if err != nil {
		return 0
	}
	count, _ := layout.SectorCount()
	return count
}

type mfocStreamEmitter struct {
	mu      sync.Mutex
	emitFn  func(AttackEvent)
	buffers map[Stream]string
	sectors int
	engine  string
}

func newMFoCStreamEmitter(emit func(AttackEvent), sectors int, engine string) *mfocStreamEmitter {
	return &mfocStreamEmitter{emitFn: emit, buffers: make(map[Stream]string), sectors: sectors, engine: engine}
}

func (s *mfocStreamEmitter) emit(event AttackEvent) {
	if s.emitFn == nil || event.Data == "" {
		if s.emitFn != nil {
			s.emitFn(event)
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	buffer := s.buffers[event.Stream] + event.Data
	for {
		index := strings.IndexByte(buffer, '\n')
		if index < 0 {
			break
		}
		line := buffer[:index+1]
		buffer = buffer[index+1:]
		s.emitLine(event, line)
	}
	// Bound pathological no-newline output while retaining enough overlap to
	// retain enough overlap for a progress token that straddles visible chunks.
	if len(buffer) > 8192 {
		cut := len(buffer) - 64
		s.emitLine(event, buffer[:cut])
		buffer = buffer[cut:]
	}
	s.buffers[event.Stream] = buffer
}

func (s *mfocStreamEmitter) emitLine(base AttackEvent, line string) {
	base.Data = line
	s.emitFn(base)
	match := mfocSectorRE.FindStringSubmatch(line)
	if len(match) != 2 || s.sectors == 0 {
		return
	}
	sector, err := strconv.Atoi(match[1])
	if err != nil || sector < 0 || sector >= s.sectors {
		return
	}
	s.emitFn(AttackEvent{Engine: base.Engine, State: StateRunning, Message: fmt.Sprintf("mfoc is processing sector %d", sector)})
}

func (s *mfocStreamEmitter) flush() {
	if s.emitFn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for stream, buffered := range s.buffers {
		if buffered != "" {
			s.emitLine(AttackEvent{Engine: s.engine, State: StateRunning, Stream: stream}, buffered)
		}
	}
	s.buffers = make(map[Stream]string)
}
