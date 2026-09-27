//go:build !windows

package terminal

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestWriteUnixScriptKeepsRuntimeAndDeviceDataQuoted(t *testing.T) {
	runtimeDir := t.TempDir() + "/runtime with ' quote"
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	device := "pn532_uart:/dev/tty usb; echo unsafe"
	script, err := writeUnixScriptWithShell(Environment{RuntimeDir: runtimeDir, Device: device}, "/bin/sh")
	if err != nil {
		t.Fatalf("writeUnixScript: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(script) })
	contents, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "NFCX command-line environment") || !strings.Contains(text, "mfoc-hardnested -C -F -k FFFFFFFFFFFF") {
		t.Fatalf("script does not include terminal guidance: %s", text)
	}
	if strings.Contains(text, "echo unsafe") {
		t.Fatalf("raw device value appears in shell script: %s", text)
	}
	info, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("script permissions = %o, want 700", info.Mode().Perm())
	}
	execAt := strings.LastIndex(text, "\nexec ")
	if execAt < 0 {
		t.Fatalf("generated script has no interactive shell: %s", text)
	}
	text = text[:execAt] + "\nprintf '%s' \"$LIBNFC_DEVICE\"\n"
	if err := os.WriteFile(script, []byte(text), 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("/bin/sh", script).Output()
	if err != nil {
		t.Fatalf("run generated script: %v", err)
	}
	if got := string(output); !strings.HasSuffix(got, device) {
		t.Fatalf("script device = %q, want suffix %q", got, device)
	}
}
