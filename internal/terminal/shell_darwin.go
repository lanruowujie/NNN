//go:build darwin

package terminal

import (
	"os/exec"
	"os/user"
	"strings"
)

// defaultUnixShell returns the account login shell rather than trusting the
// GUI process environment, which often lacks SHELL when launched from Finder.
func defaultUnixShell() (string, error) {
	current, err := user.Current()
	if err == nil && current.Username != "" {
		if output, err := exec.Command("dscl", ".", "-read", "/Users/"+current.Username, "UserShell").Output(); err == nil {
			if fields := strings.Fields(string(output)); len(fields) >= 2 && validShell(fields[len(fields)-1]) {
				return fields[len(fields)-1], nil
			}
		}
	}
	return fallbackShell("/bin/zsh"), nil
}
