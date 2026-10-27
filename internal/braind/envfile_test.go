package braind

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnrowl/brain/internal/braind/config"
	"github.com/johnrowl/brain/internal/braind/datalock"
)

// TestRunLoadsConfigurationFromEnvFile proves that configuration values read
// from a .env file flow through the daemon exactly like process-environment
// values do (the dotenv overlay feeds config.Load inside run).
func TestRunLoadsConfigurationFromEnvFile(t *testing.T) {
	dataRoot := t.TempDir()
	envFile := filepath.Join(dataRoot, ".env")
	if err := os.WriteFile(envFile, []byte(envFileConfig(dataRoot)), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	lookup, err := config.LoadDotEnv(mapLookup(map[string]string{}), envFile)
	if err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(ctx, nil, "test", &stdout, &stderr, lookup)

	if strings.Contains(stderr.String(), datalock.ErrUnsupported.Error()) {
		t.Skip(datalock.ErrUnsupported)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"msg":"server listening"`) ||
		!strings.Contains(stdout.String(), `"address":"127.0.0.1:`) {
		t.Fatalf("startup output did not contain configured listener: %q", stdout.String())
	}
}

// TestRunEntryPointLoadsEnvFile verifies that the exported Run entry point
// loads the dot-env overlay (envFile): the daemon starts successfully using
// configuration that exists ONLY in the file. Without loading the file the
// startup would fail OIDC validation and exit non-zero.
func TestRunEntryPointLoadsEnvFile(t *testing.T) {
	dataRoot := t.TempDir()
	dotEnv := filepath.Join(dataRoot, ".env")
	if err := os.WriteFile(dotEnv, []byte(envFileConfig(dataRoot)), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	previous := envFile
	envFile = dotEnv
	t.Cleanup(func() { envFile = previous })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(ctx, nil, "test", &stdout, &stderr)

	if strings.Contains(stderr.String(), datalock.ErrUnsupported.Error()) {
		t.Skip(datalock.ErrUnsupported)
	}
	if exitCode != 0 {
		t.Fatalf("Run did not load config from .env (expected exit 0, got %d): %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"address":"127.0.0.1:`) {
		t.Fatalf("daemon did not use the .env-configured listener: %q", stdout.String())
	}
}

// envFileConfig returns a minimal valid braind configuration for dataRoot.
func envFileConfig(dataRoot string) string {
	return fmt.Sprintf(
		"BRAIND_LISTEN_ADDR=127.0.0.1:0\n"+
			"BRAIND_DATA_PATH=%s\n"+
			"BRAIND_VAULT_PATH=%s/vault\n"+
			"BRAIND_GIT_DIR=%s/git/vault.git\n"+
			"BRAIND_GIT_BACKUP_ENABLED=false\n"+
			"OIDC_ENABLED=false\n",
		dataRoot, dataRoot, dataRoot,
	)
}
