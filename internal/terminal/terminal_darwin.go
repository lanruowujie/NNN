//go:build darwin

package terminal

import (
	"fmt"
	"os/exec"
	"strings"
)

// Open starts macOS Terminal with a fresh shell configured for NFCX's bundled
// tools. macOS may ask the user to authorize NFCX controlling Terminal.
func Open(env Environment) error {
	if err := env.validate(); err != nil {
		return err
	}
	script, err := writeUnixScript(env)
	if err != nil {
		return err
	}
	command := "/bin/sh " + shellQuote(script)
	appleScript := `tell application "Terminal" to do script "` + appleScriptQuote(command) + `"`
	if output, err := exec.Command("osascript", "-e", appleScript).CombinedOutput(); err != nil {
		return fmt.Errorf("open Terminal: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func appleScriptQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}
