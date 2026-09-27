package attack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type command struct {
	Path string
	Args []string
	Dir  string
	Env  map[string]string
}

type processResult struct {
	ExitCode  int
	StartedAt time.Time
	EndedAt   time.Time
	Stdout    []byte
	Stderr    []byte
}

type processRunner struct {
	WaitDelay         time.Duration
	HeartbeatInterval time.Duration
}

func newProcessRunner() *processRunner {
	return &processRunner{WaitDelay: 2 * time.Second, HeartbeatInterval: 5 * time.Second}
}

type processStreamWriter struct {
	destination *bytes.Buffer
	stream      Stream
	engine      string
	emit        func(AttackEvent)
}

func (w *processStreamWriter) Write(chunk []byte) (int, error) {
	count, err := w.destination.Write(chunk)
	if count > 0 && w.emit != nil {
		w.emit(AttackEvent{
			Engine: w.engine,
			State:  StateRunning,
			Stream: w.stream,
			Data:   strings.ToValidUTF8(string(chunk[:count]), "�"),
		})
	}
	return count, err
}

func (r *processRunner) run(ctx context.Context, engine string, spec command, emit func(AttackEvent)) (processResult, error) {
	result := processResult{ExitCode: -1}
	if ctx == nil || spec.Path == "" {
		return result, &Error{Code: CodeStart, Engine: engine, ExitCode: -1, Detail: "invalid process request"}
	}
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = mergeEnvironment(os.Environ(), spec.Env)
	waitDelay := r.WaitDelay
	if waitDelay <= 0 {
		waitDelay = 2 * time.Second
	}
	cmd.WaitDelay = waitDelay

	var stdoutBuffer, stderrBuffer bytes.Buffer
	cmd.Stdout = &processStreamWriter{destination: &stdoutBuffer, stream: StreamStdout, engine: engine, emit: emit}
	cmd.Stderr = &processStreamWriter{destination: &stderrBuffer, stream: StreamStderr, engine: engine, emit: emit}
	tree, err := prepareProcessTree(cmd)
	if err != nil {
		return result, &Error{Code: CodeStart, Engine: engine, ExitCode: -1, Detail: "prepare process isolation", Cause: err}
	}
	defer tree.Close()

	result.StartedAt = time.Now().UTC()
	if err := cmd.Start(); err != nil {
		result.EndedAt = time.Now().UTC()
		return result, &Error{Code: CodeStart, Engine: engine, ExitCode: -1, Detail: "start process", Cause: err}
	}
	if err := tree.Started(cmd); err != nil {
		_ = tree.Kill()
		_ = cmd.Wait()
		result.EndedAt = time.Now().UTC()
		return result, &Error{Code: CodeStart, Engine: engine, ExitCode: -1, Detail: "attach process tree", Cause: err}
	}
	if emit != nil {
		emit(AttackEvent{Engine: engine, State: StateRunning, Message: "external process is running"})
	}
	stopHeartbeat := r.startHeartbeat(ctx, engine, result.StartedAt, emit)

	waitErr := cmd.Wait()
	stopHeartbeat()
	result.EndedAt = time.Now().UTC()
	result.Stdout = append([]byte(nil), stdoutBuffer.Bytes()...)
	result.Stderr = append([]byte(nil), stderrBuffer.Bytes()...)
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		code := CodeCancelled
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			code = CodeTimeout
		}
		return result, &Error{Code: code, Engine: engine, ExitCode: result.ExitCode, Detail: ctxErr.Error(), Cause: ctxErr}
	}
	if waitErr != nil {
		return result, &Error{Code: CodeExit, Engine: engine, ExitCode: result.ExitCode, Detail: "process returned a non-zero exit status", Cause: waitErr}
	}
	return result, nil
}

func (r *processRunner) startHeartbeat(ctx context.Context, engine string, startedAt time.Time, emit func(AttackEvent)) func() {
	if emit == nil {
		return func() {}
	}
	interval := r.HeartbeatInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				elapsed := now.Sub(startedAt).Round(time.Second)
				message := fmt.Sprintf("%s 进程仍在运行：已用时 %s", engine, elapsed)
				if deadline, ok := ctx.Deadline(); ok {
					remaining := deadline.Sub(now).Round(time.Second)
					if remaining < 0 {
						remaining = 0
					}
					message += fmt.Sprintf("，本阶段超时剩余 %s", remaining)
				}
				emit(AttackEvent{Engine: engine, State: StateRunning, Message: message})
			case <-ctx.Done():
				return
			case <-stop:
				return
			}
		}
	}()
	return func() {
		once.Do(func() { close(stop) })
		<-done
	}
}

func mergeEnvironment(base []string, additions map[string]string) []string {
	if len(additions) == 0 {
		return append([]string(nil), base...)
	}
	keys := make(map[string]struct{}, len(additions))
	for key := range additions {
		keys[environmentKey(key)] = struct{}{}
	}
	result := make([]string, 0, len(base)+len(additions))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := keys[environmentKey(key)]; replaced {
				continue
			}
		}
		result = append(result, entry)
	}
	for key, value := range additions {
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}
	return result
}

func environmentKey(key string) string {
	if os.PathSeparator == '\\' {
		return strings.ToUpper(key)
	}
	return key
}
