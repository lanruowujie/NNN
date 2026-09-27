//go:build !darwin && !linux && !windows

package terminal

func defaultUnixShell() (string, error) { return "/bin/sh", nil }
