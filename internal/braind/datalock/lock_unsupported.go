//go:build !linux && !darwin

package datalock

import "os"

func lockingSupported() bool      { return false }
func tryLock(_ *os.File) error    { return ErrUnsupported }
func unlock(_ *os.File) error     { return nil }
func isLockContention(error) bool { return false }
