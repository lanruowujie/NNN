//go:build !windows

package terminal

import "os"

func validShell(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func fallbackShell(preferred string) string {
	if validShell(preferred) {
		return preferred
	}
	return "/bin/sh"
}
