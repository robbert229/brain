package braind

import (
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/config"
)

// TestRunPrivacyAcrossLogsAndStatus exercises a full startup with representative
// secrets configured (OIDC client secret, Git remote URL) and asserts none of
// them appear in any log line or in the public status document (SC-004).
func TestRunPrivacyAcrossLogsAndStatus(t *testing.T) {
	const (
		clientSecret = "privacy-check-oidc-client-secret"
		gitRemoteURL = "git@example.test:team/private-brain.git"
	)
	environment := localEnvironment(t)
	dataRoot := environment[config.EnvDataPath]
	environment[config.EnvOIDCEnabled] = "true"
	environment[config.EnvOIDCConfigurationURL] = "https://oidc.example.test/.well-known/openid-configuration"
	environment[config.EnvOIDCClientID] = "braind"
	environment[config.EnvOIDCClientSecret] = clientSecret
	environment[config.EnvOIDCAdminGroup] = "access-brain-admin"
	environment[config.EnvOIDCUserGroup] = "access-brain-user"
	environment[config.EnvOIDCViewerGroup] = "access-brain-viewer"
	environment[config.EnvGitBackupEnabled] = "true"
	environment[config.EnvGitRemoteURL] = gitRemoteURL
	environment[config.EnvGitDir] = filepath.Join(dataRoot, "git", "vault.git")
	environment[config.EnvGitSSHKeyPath] = filepath.Join(dataRoot, "keys", "ssh-privatekey")
	environment[config.EnvGitKnownHostsPath] = filepath.Join(dataRoot, "keys", "known_hosts")
	environment[config.EnvLogLevel] = "debug"

	var stdout syncWriter
	var stderr syncWriter
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan int, 1)
	go func() { result <- run(ctx, nil, "test", "test-commit", &stdout, &stderr, mapLookup(environment)) }()
	cleanupReturned := false
	t.Cleanup(func() {
		cancel()
		if !cleanupReturned {
			select {
			case <-result:
			case <-time.After(3 * time.Second):
				t.Error("run did not exit after cleanup cancellation")
			}
		}
	})

	// Wait for the listener address, then generate request logs and probe status.
	address := ""
	deadline := time.Now().Add(5 * time.Second)
	for address == "" {
		if time.Now().After(deadline) {
			t.Fatalf("daemon never started listening: %s", stdout.text())
		}
		if _, after, ok := strings.Cut(stdout.text(), `"address":"`); ok {
			host, _, _ := strings.Cut(after, `"`)
			if _, port, err := net.SplitHostPort(host); err == nil {
				address = "http://127.0.0.1:" + port
			}
		}
		if address == "" {
			time.Sleep(5 * time.Millisecond)
		}
	}
	client := &http.Client{Timeout: 2 * time.Second}
	var statusBody string
	for _, path := range []string{"/healthz", "/readyz", "/status"} {
		response, err := client.Get(address + path)
		if err != nil {
			t.Fatalf("request %s before shutdown: %v", path, err)
		}
		body, _ := io.ReadAll(response.Body)
		if err := response.Body.Close(); err != nil {
			t.Fatalf("close %s response: %v", path, err)
		}
		if path == "/status" {
			statusBody = string(body)
		}
	}

	cancel()
	select {
	case <-result:
		cleanupReturned = true
	case <-time.After(3 * time.Second):
		t.Fatal("run did not exit after cancellation")
	}

	logs := stdout.text() + stderr.text()
	for _, secret := range []string{clientSecret, gitRemoteURL} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs disclosed configured secret %q", secret)
		}
		if strings.Contains(statusBody, secret) {
			t.Fatalf("status disclosed configured secret %q", secret)
		}
	}
	if statusBody == "" {
		t.Fatal("status request returned no body")
	}
}
