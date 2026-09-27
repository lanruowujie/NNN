//go:build linux

package terminal

import (
	"os"
	"os/user"
	"strings"
)

func defaultUnixShell() (string, error) {
	current, err := user.Current()
	if err == nil {
		if contents, readErr := os.ReadFile("/etc/passwd"); readErr == nil {
			for _, line := range strings.Split(string(contents), "\n") {
				fields := strings.Split(line, ":")
				if len(fields) == 7 && (fields[0] == current.Username || fields[2] == current.Uid) && validShell(fields[6]) {
					return fields[6], nil
				}
			}
		}
	}
	return fallbackShell("/bin/bash"), nil
}
