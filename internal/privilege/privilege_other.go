//go:build !windows && !linux

package privilege

func Ensure() (bool, error) { return false, nil }
