//go:build !darwin && !linux

package libnfc

func connStringPermissionError(string) error { return nil }
