//go:build !darwin && !linux && !windows

package terminal

func Open(Environment) error { return ErrUnsupported }
