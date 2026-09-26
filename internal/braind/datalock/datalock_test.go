package datalock

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAcquireIsExclusiveAndReusable(t *testing.T) {
	dataRoot := t.TempDir()
	first, err := Acquire(dataRoot)
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	t.Cleanup(func() { _ = first.Release() })

	if first.Path() != filepath.Join(dataRoot, filename) {
		t.Fatalf("unexpected lock path: %q", first.Path())
	}
	owner, err := os.ReadFile(first.Path())
	if err != nil {
		t.Fatalf("read lock owner: %v", err)
	}
	if strings.TrimSpace(string(owner)) != fmt.Sprint(os.Getpid()) {
		t.Fatalf("unexpected lock owner: %q", owner)
	}
	info, err := os.Stat(first.Path())
	if err != nil {
		t.Fatalf("stat lock: %v", err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Fatalf("unexpected lock permissions: %o", permissions)
	}

	second, err := Acquire(dataRoot)
	if second != nil || !errors.Is(err, ErrAlreadyLocked) {
		t.Fatalf("expected lock contention, got lock=%v error=%v", second, err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("release first lock: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("release first lock again: %v", err)
	}

	reacquired, err := Acquire(dataRoot)
	if err != nil {
		t.Fatalf("reacquire lock: %v", err)
	}
	if err := reacquired.Release(); err != nil {
		t.Fatalf("release reacquired lock: %v", err)
	}
}

func TestOrdinaryProcessExitReleasesLock(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("advisory lock implementation is not available on this platform")
	}

	dataRoot := t.TempDir()
	readyPath := filepath.Join(t.TempDir(), "ready")
	stopPath := filepath.Join(t.TempDir(), "stop")
	command := exec.Command(os.Args[0], "-test.run=^TestDataLockHelperProcess$")
	command.Env = append(os.Environ(),
		"BRAIND_DATALOCK_HELPER=1",
		"BRAIND_DATALOCK_ROOT="+dataRoot,
		"BRAIND_DATALOCK_READY="+readyPath,
		"BRAIND_DATALOCK_STOP="+stopPath,
	)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatalf("start lock helper: %v", err)
	}
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})

	waitForFile(t, readyPath)
	contended, err := Acquire(dataRoot)
	if contended != nil || !errors.Is(err, ErrAlreadyLocked) {
		t.Fatalf("helper did not hold lock: lock=%v error=%v", contended, err)
	}
	if err := os.WriteFile(stopPath, []byte("stop\n"), 0o600); err != nil {
		t.Fatalf("signal helper to exit: %v", err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("wait for ordinary helper exit: %v: %s", err, output.String())
	}
	waited = true

	reacquired, err := Acquire(dataRoot)
	if err != nil {
		t.Fatalf("acquire after helper exit: %v", err)
	}
	if err := reacquired.Release(); err != nil {
		t.Fatalf("release lock after helper exit: %v", err)
	}
}

func TestDataLockHelperProcess(t *testing.T) {
	if os.Getenv("BRAIND_DATALOCK_HELPER") != "1" {
		return
	}
	lock, err := Acquire(os.Getenv("BRAIND_DATALOCK_ROOT"))
	if err != nil {
		t.Fatalf("helper acquire lock: %v", err)
	}
	// Deliberately do not release lock: process exit must close the descriptor.
	_ = lock
	if err := os.WriteFile(os.Getenv("BRAIND_DATALOCK_READY"), []byte("ready\n"), 0o600); err != nil {
		t.Fatalf("write helper ready marker: %v", err)
	}
	stopPath := os.Getenv("BRAIND_DATALOCK_STOP")
	for {
		if _, err := os.Stat(stopPath); err == nil {
			runtime.KeepAlive(lock)
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("check helper stop marker: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAcquireRejectsRelativeRoot(t *testing.T) {
	if _, err := Acquire("relative/data"); err == nil {
		t.Fatal("expected relative data root to fail")
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("wait for %s: %v", path, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
