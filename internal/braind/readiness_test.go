package braind

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/johnrowl/brain/internal/braind/lifecycle"
)

func TestVaultAccessible(t *testing.T) {
	t.Run("readable directory", func(t *testing.T) {
		if !vaultAccessible(t.TempDir()) {
			t.Fatal("readable directory reported inaccessible")
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "empty")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("create directory: %v", err)
		}
		if !vaultAccessible(dir) {
			t.Fatal("empty directory reported inaccessible")
		}
	})

	t.Run("missing path", func(t *testing.T) {
		if vaultAccessible(filepath.Join(t.TempDir(), "does-not-exist")) {
			t.Fatal("missing path reported accessible")
		}
	})

	t.Run("file instead of directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "vault.md")
		if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		if vaultAccessible(file) {
			t.Fatal("regular file reported accessible")
		}
	})

	t.Run("unreadable directory", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("permission checks do not apply when running as root")
		}
		dir := filepath.Join(t.TempDir(), "locked")
		if err := os.Mkdir(dir, 0o000); err != nil {
			t.Fatalf("create directory: %v", err)
		}
		defer os.Chmod(dir, 0o700)
		if vaultAccessible(dir) {
			t.Fatal("unreadable directory reported accessible")
		}
	})
}

func TestReadinessComposition(t *testing.T) {
	vault := t.TempDir()

	t.Run("nil process fails closed", func(t *testing.T) {
		if readiness(handlerState{vaultPath: vault}) {
			t.Fatal("unwired process must not report ready")
		}
	})

	t.Run("starting process is not ready", func(t *testing.T) {
		process := lifecycle.New()
		if readiness(handlerState{process: process, vaultPath: vault}) {
			t.Fatal("starting process must not report ready")
		}
	})

	t.Run("running with accessible vault is ready", func(t *testing.T) {
		process := lifecycle.New()
		if err := process.MarkRunning(); err != nil {
			t.Fatalf("mark running: %v", err)
		}
		if !readiness(handlerState{process: process, vaultPath: vault}) {
			t.Fatal("running process with accessible vault must report ready")
		}
	})

	t.Run("running with inaccessible vault is not ready", func(t *testing.T) {
		process := lifecycle.New()
		if err := process.MarkRunning(); err != nil {
			t.Fatalf("mark running: %v", err)
		}
		missing := filepath.Join(t.TempDir(), "vault")
		if readiness(handlerState{process: process, vaultPath: missing}) {
			t.Fatal("inaccessible vault must clear readiness (US2-AC2)")
		}
	})

	t.Run("degraded local mode keeps readiness decided by vault", func(t *testing.T) {
		process := lifecycle.New()
		if err := process.MarkDegraded(true); err != nil {
			t.Fatalf("mark degraded: %v", err)
		}
		if !readiness(handlerState{process: process, vaultPath: vault}) {
			t.Fatal("degraded-but-capable process must report ready")
		}
	})

	t.Run("shutting down is never ready", func(t *testing.T) {
		process := lifecycle.New()
		if err := process.MarkRunning(); err != nil {
			t.Fatalf("mark running: %v", err)
		}
		process.BeginShutdown()
		if readiness(handlerState{process: process, vaultPath: vault}) {
			t.Fatal("shutting-down process must not report ready (SC-003)")
		}
	})
}
