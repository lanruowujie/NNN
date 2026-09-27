//go:build linux

package terminal

import (
	"fmt"
	"os"
	"os/exec"
)

// Open tries the freedesktop terminal launcher first, then common terminal
// emulators. Linux does not have one terminal API available on every desktop.
func Open(env Environment) error {
	if err := env.validate(); err != nil {
		return err
	}
	script, err := writeUnixScript(env)
	if err != nil {
		return err
	}
	launchers := [][]string{
		{"xdg-terminal-exec", "/bin/sh", script},
		{"gnome-terminal", "--", "/bin/sh", script},
		{"konsole", "-e", "/bin/sh", script},
		{"x-terminal-emulator", "-e", "/bin/sh", script},
		{"xterm", "-e", "/bin/sh", script},
	}
	for _, args := range launchers {
		binary, lookErr := exec.LookPath(args[0])
		if lookErr != nil {
			continue
		}
		command := exec.Command(binary, args[1:]...)
		if err := command.Start(); err == nil {
			return nil
		}
	}
	_ = os.Remove(script)
	return fmt.Errorf("%w: install a terminal emulator supported by your desktop", ErrUnsupported)
}
