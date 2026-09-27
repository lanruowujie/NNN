package attack

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BennyThink/NFCX/internal/device"
	"github.com/BennyThink/NFCX/internal/nfc"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

const MFCUKVersion = "0.3.8"

const mfcukCandidateMediaType = "application/vnd.nfcx.mfcuk-candidates+json"

var (
	mfcukExecutable       = runtimebundle.Executable{ID: "mfcuk", FileName: "mfcuk", WindowsFileName: "mfcuk.exe"}
	mfcukMachineVersionRE = regexp.MustCompile(`(?mi)^NFCX_VERSION[ \t]+v?([0-9]+(?:\.[0-9]+)+)[ \t]*\r?$`)
	mfcukVersionRE        = regexp.MustCompile(`(?mi)^(?:.*[/\\])?mfcuk(?:\.exe)?[ \t]+(?:-[ \t]*|version[ \t]+)?v?([0-9]+(?:\.[0-9]+)+)[ \t]*\r?$`)
	mfcukProgressRE       = regexp.MustCompile(`^NFCX_PROGRESS key=([AB]) sector=([0-9]{1,2}) attempts=([0-9]+)\r?$`)
	mfcukResultRE         = regexp.MustCompile(`(?m)^NFCX_RESULT key=([AB]) sector=([0-9]{1,2}) value=([0-9A-Fa-f]{12})\r?$`)
)

type MFCUKInvocation struct {
	Card nfc.CardInfo
}

type MFCUKCandidate struct {
	Sector int         `json:"sector"`
	Type   nfc.KeyType `json:"keyType"`
	Value  nfc.Key     `json:"value"`
}

type MFCUKEngine struct {
	locator    *runtimebundle.Locator
	runner     *processRunner
	invocation MFCUKInvocation
	extraEnv   map[string]string // tests only
}

func NewMFCUKEngine(locator *runtimebundle.Locator, invocation MFCUKInvocation) *MFCUKEngine {
	invocation.Card = invocation.Card.Clone()
	return &MFCUKEngine{locator: locator, runner: newProcessRunner(), invocation: invocation}
}

func (e *MFCUKEngine) Name() string { return mfcukExecutable.ID }

func (e *MFCUKEngine) Available(ctx context.Context) error {
	if ctx == nil || e == nil || e.locator == nil || e.runner == nil {
		return errors.New("mfcuk adapter is unavailable")
	}
	path, err := e.locator.Resolve(mfcukExecutable)
	if err != nil {
		return err
	}
	versionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, runErr := e.runner.run(versionCtx, e.Name(), command{Path: path, Args: []string{"-h"}}, nil)
	if runErr != nil && !errors.Is(runErr, ErrExit) {
		return fmt.Errorf("run mfcuk version check: %w", runErr)
	}
	version := parseMFCUKVersion(result.Stdout, result.Stderr)
	if version == "" {
		return errors.New("mfcuk version output is not recognized")
	}
	if version != MFCUKVersion {
		return fmt.Errorf("mfcuk version %s is unsupported; expected %s", version, MFCUKVersion)
	}
	combined := bytes.Join([][]byte{result.Stdout, result.Stderr}, []byte{'\n'})
	if !bytes.Contains(combined, []byte("NFCX machine result: 1")) {
		return errors.New("mfcuk does not advertise the required NFCX machine result format")
	}
	if !bytes.Contains(combined, []byte("-R sector")) {
		return errors.New("mfcuk help does not advertise the required recovery option")
	}
	return nil
}

func (e *MFCUKEngine) Run(ctx context.Context, request AttackRequest, emit func(AttackEvent)) (AttackResult, error) {
	result := AttackResult{Engine: e.Name(), Version: MFCUKVersion, ExitCode: -1, TemporaryDir: request.WorkingDir}
	if ctx == nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: MFCUKVersion, Detail: "mfcuk context is required"}
	}
	if request.WorkingDir == "" || !filepath.IsAbs(request.WorkingDir) {
		return result, &Error{Code: CodeTemporary, Engine: e.Name(), ExitCode: -1, Version: MFCUKVersion, Detail: "mfcuk working directory must be an absolute task temporary directory"}
	}
	if nfc.InferCardType(e.invocation.Card) != nfc.CardTypeMIFAREClassic1K {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: MFCUKVersion, Detail: "mfcuk pipeline supports only MIFARE Classic 1K"}
	}
	if !device.CapabilitiesFor(request.Device).Darkside {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: -1, Version: MFCUKVersion, Detail: "released reader is not enabled for Darkside attacks"}
	}
	path, err := e.locator.Resolve(mfcukExecutable)
	if err != nil {
		return result, &Error{Code: CodeUnavailable, Engine: e.Name(), ExitCode: -1, Detail: err.Error(), Cause: err}
	}
	if emit != nil {
		emit(AttackEvent{Engine: e.Name(), State: StateStarting, Message: "starting bundled mfcuk Darkside recovery"})
	}
	environment := map[string]string{
		"LIBNFC_DEVICE": request.Device.ConnString, "LIBNFC_AUTO_SCAN": "false", "LIBNFC_INTRUSIVE_SCAN": "false",
	}
	for key, value := range e.extraEnv {
		environment[key] = value
	}
	stream := newMFCUKStreamEmitter(emit)
	process, runErr := e.runner.run(ctx, e.Name(), command{
		Path: path, Args: mfcukArguments(), Dir: request.WorkingDir, Env: environment,
	}, stream.emit)
	stream.flush()
	result.ExitCode, result.StartedAt, result.FinishedAt = process.ExitCode, process.StartedAt, process.EndedAt
	result.Stdout, result.Stderr = process.Stdout, process.Stderr
	if parsed := parseMFCUKVersion(process.Stdout, process.Stderr); parsed != "" {
		result.Version = parsed
	}
	if runErr != nil {
		return result, enrichEngineError(runErr, result)
	}
	if emit != nil {
		emit(AttackEvent{Engine: e.Name(), State: StateValidatingResult, Message: "validating mfcuk candidate records"})
	}
	candidates, err := parseMFCUKCandidates(process.Stdout, process.Stderr)
	if err != nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: result.ExitCode, Version: result.Version, Detail: err.Error(), Cause: err}
	}
	payload, err := json.Marshal(candidates)
	if err != nil {
		return result, &Error{Code: CodeValidation, Engine: e.Name(), ExitCode: result.ExitCode, Version: result.Version, Detail: "encode mfcuk candidates", Cause: err}
	}
	result.Artifacts = []AttackArtifact{{Name: "mfcuk-candidates.json", MediaType: mfcukCandidateMediaType, Data: payload}}
	result.Validated = true // the candidate envelope is valid; key authenticity is verified by the workflow
	return result, nil
}

func mfcukArguments() []string {
	return []string{"-C", "-R", "0:A", "-s", "250", "-S", "250", "-v", "2"}
}

func parseMFCUKVersion(streams ...[]byte) string {
	for _, stream := range streams {
		if match := mfcukMachineVersionRE.FindSubmatch(stream); len(match) == 2 {
			return string(match[1])
		}
		if match := mfcukVersionRE.FindSubmatch(stream); len(match) == 2 {
			return string(match[1])
		}
	}
	return ""
}

func parseMFCUKCandidates(streams ...[]byte) ([]MFCUKCandidate, error) {
	combined := bytes.Join(streams, []byte{'\n'})
	matches := mfcukResultRE.FindAllSubmatch(combined, -1)
	if len(matches) == 0 {
		return nil, errors.New("mfcuk produced no machine-readable key candidates")
	}
	unique := make(map[string]MFCUKCandidate, len(matches))
	for _, match := range matches {
		sector, err := strconv.Atoi(string(match[2]))
		if err != nil || sector < 0 || sector >= 16 {
			return nil, fmt.Errorf("mfcuk candidate sector %q is outside Classic 1K", match[2])
		}
		decoded, err := hex.DecodeString(string(match[3]))
		if err != nil || len(decoded) != len(nfc.Key{}) {
			return nil, fmt.Errorf("mfcuk candidate key for sector %d is invalid", sector)
		}
		keyType := nfc.KeyTypeA
		if string(match[1]) == "B" {
			keyType = nfc.KeyTypeB
		}
		var value nfc.Key
		copy(value[:], decoded)
		candidate := MFCUKCandidate{Sector: sector, Type: keyType, Value: value}
		identity := fmt.Sprintf("%02d:%d:%012X", sector, keyType, value)
		unique[identity] = candidate
	}
	result := make([]MFCUKCandidate, 0, len(unique))
	for _, candidate := range unique {
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Sector != result[j].Sector {
			return result[i].Sector < result[j].Sector
		}
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		return strings.Compare(fmt.Sprintf("%X", result[i].Value), fmt.Sprintf("%X", result[j].Value)) < 0
	})
	return result, nil
}

func MFCUKCandidatesFromArtifacts(artifacts []AttackArtifact) ([]MFCUKCandidate, error) {
	for _, artifact := range artifacts {
		if artifact.MediaType != mfcukCandidateMediaType {
			continue
		}
		var candidates []MFCUKCandidate
		if err := json.Unmarshal(artifact.Data, &candidates); err != nil {
			return nil, fmt.Errorf("decode mfcuk candidate artifact: %w", err)
		}
		if len(candidates) == 0 {
			return nil, errors.New("mfcuk candidate artifact is empty")
		}
		return candidates, nil
	}
	return nil, errors.New("mfcuk candidate artifact is missing")
}

type mfcukStreamEmitter struct {
	mu      sync.Mutex
	emitFn  func(AttackEvent)
	buffers map[Stream]string
}

func newMFCUKStreamEmitter(emit func(AttackEvent)) *mfcukStreamEmitter {
	return &mfcukStreamEmitter{emitFn: emit, buffers: make(map[Stream]string)}
}

func (s *mfcukStreamEmitter) emit(event AttackEvent) {
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
	if len(buffer) > 8192 {
		cut := len(buffer) - 64
		s.emitLine(event, buffer[:cut])
		buffer = buffer[cut:]
	}
	s.buffers[event.Stream] = buffer
}

func (s *mfcukStreamEmitter) emitLine(base AttackEvent, line string) {
	base.Data = line
	s.emitFn(base)
	match := mfcukProgressRE.FindStringSubmatch(strings.TrimSuffix(line, "\n"))
	if len(match) != 4 {
		return
	}
	s.emitFn(AttackEvent{
		Engine:  base.Engine,
		State:   StateRunning,
		Message: fmt.Sprintf("MFCUK 正在恢复 Sector %s Key %s：已尝试 %s 次认证", match[2], match[1], match[3]),
	})
}

func (s *mfcukStreamEmitter) flush() {
	if s.emitFn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for stream, buffered := range s.buffers {
		if buffered != "" {
			s.emitLine(AttackEvent{Engine: "mfcuk", State: StateRunning, Stream: stream}, buffered)
		}
	}
	s.buffers = make(map[Stream]string)
}
