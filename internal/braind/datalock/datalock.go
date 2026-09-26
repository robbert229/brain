// Package datalock owns the process-wide advisory lock for a braind data root.
package datalock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const filename = "braind.lock"

var (
	// ErrAlreadyLocked means another process holds the data-root lock.
	ErrAlreadyLocked = errors.New("data root is already locked")
	// ErrUnsupported means advisory locking is unavailable on this platform.
	ErrUnsupported = errors.New("data-root locking is unsupported on this platform")
)

// Lock holds an exclusive advisory lock until Release is called or the process
// exits. The lock file remains in place; ownership is represented by the open,
// locked file descriptor rather than by file existence.
type Lock struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// Acquire creates the data root when necessary and attempts to take its lock
// without waiting. Contention returns ErrAlreadyLocked.
func Acquire(dataRoot string) (*Lock, error) {
	if !filepath.IsAbs(dataRoot) {
		return nil, errors.New("data root must be an absolute path")
	}
	if !lockingSupported() {
		return nil, ErrUnsupported
	}
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create data root: %w", err)
	}

	path := filepath.Join(dataRoot, filename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open data lock: %w", err)
	}
	if err := tryLock(file); err != nil {
		_ = file.Close()
		if isLockContention(err) {
			return nil, fmt.Errorf("%w: %s", ErrAlreadyLocked, path)
		}
		return nil, fmt.Errorf("acquire data lock: %w", err)
	}

	lock := &Lock{file: file, path: path}
	if err := lock.recordOwner(); err != nil {
		_ = lock.Release()
		return nil, err
	}
	return lock, nil
}

// Path returns the absolute lock-file path.
func (l *Lock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Release unlocks and closes the descriptor. It is safe to call repeatedly.
func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}

	file := l.file
	l.file = nil
	return errors.Join(unlock(file), file.Close())
}

func (l *Lock) recordOwner() error {
	if err := l.file.Chmod(0o600); err != nil {
		return fmt.Errorf("secure data lock: %w", err)
	}
	if err := l.file.Truncate(0); err != nil {
		return fmt.Errorf("truncate data lock: %w", err)
	}
	if _, err := l.file.Seek(0, 0); err != nil {
		return fmt.Errorf("seek data lock: %w", err)
	}
	if _, err := fmt.Fprintf(l.file, "%d\n", os.Getpid()); err != nil {
		return fmt.Errorf("record data-lock owner: %w", err)
	}
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("sync data lock: %w", err)
	}
	return nil
}
