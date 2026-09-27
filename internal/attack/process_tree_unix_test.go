//go:build darwin || linux

package attack

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/BennyThink/NFCX/internal/nfc"
)

func TestCancellationTerminatesFixtureChildProcess(t *testing.T) {
	devices := &fakeReleasedDevices{device: nfc.DeviceInfo{ConnString: "pn532_uart:/dev/process-tree"}}
	coordinator, _ := fixtureCoordinator("hang", devices)
	recorder := newEventRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan AttackResult, 1)
	go func() {
		result, _ := coordinator.Run(ctx, AttackRequest{TaskID: "process-tree", Timeout: 10 * time.Second}, recorder.emit)
		done <- result
	}()
	waitForLog(t, recorder, "child_pid=")
	cancel()
	var result AttackResult
	select {
	case result = <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("process tree did not terminate")
	}
	pid := childPID(t, string(result.Stdout))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child process %d still exists after cancellation", pid)
}

func waitForLog(t *testing.T, recorder *eventRecorder, wanted string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		for _, event := range recorder.snapshot() {
			if strings.Contains(event.Data, wanted) {
				return
			}
		}
		select {
		case <-recorder.notify:
		case <-timer.C:
			t.Fatalf("timed out waiting for log %q", wanted)
		}
	}
}

func childPID(t *testing.T, output string) int {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if value, found := strings.CutPrefix(line, "child_pid="); found {
			pid, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("parse child PID %q: %v", value, err)
			}
			return pid
		}
	}
	t.Fatalf("child PID missing from %q", output)
	return 0
}
