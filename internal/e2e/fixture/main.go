// Command fixture starts braind behind local HTTPS with an auto-approving
// simulated OIDC provider. It is only used by the Playwright E2E suite.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/johnrowl/brain/internal/braind"
	"github.com/johnrowl/brain/internal/braind/auth/authtest"
)

const (
	appAddress     = "127.0.0.1:4174"
	proxyAddress   = "127.0.0.1:4173"
	controlAddress = "127.0.0.1:4175"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	provider := authtest.NewStandalone()
	defer provider.Close()

	dataDir, err := os.MkdirTemp("", "braind-e2e-data-")
	if err != nil {
		return fmt.Errorf("create fixture data directory: %w", err)
	}
	defer os.RemoveAll(dataDir)
	for _, dir := range []string{
		filepath.Join(dataDir, "vault"),
		filepath.Join(dataDir, "01 - Note Box/Hobby/Books"),
		filepath.Join(dataDir, "06 - Meta/Attachments"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create fixture directory: %w", err)
		}
	}
	seed := filepath.Join(dataDir, "01 - Note Box/Hobby/Books/E2E Undated Book.md")
	if err := os.WriteFile(seed, []byte("---\ntype: Zettel\nstatus (Media): Finished\n---\n\n# E2E Undated Book\n\nA fixture book for browser tests.\n"), 0o600); err != nil {
		return fmt.Errorf("seed browser fixture book: %w", err)
	}
	preservedCover := filepath.Join(dataDir, "06 - Meta/Attachments/fixture-cover.png")
	if err := os.WriteFile(preservedCover, []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\x0bIDAT\x08\xd7c\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\xd1\xa0\x00\x00\x00\x00IEND\xaeB`\x82"), 0o600); err != nil {
		return fmt.Errorf("seed fixture cover: %w", err)
	}
	preserved := filepath.Join(dataDir, "01 - Note Box/Hobby/Books/E2E Preserved Book.md")
	if err := os.WriteFile(preserved, []byte("---\ntype: Zettel\nstatus (Media): Finished\ninterest: \"🔥\"\nauthor: Preservation Author\nrating: 4/5\ncover: fixture-cover.png\nfinished: 2025-03-04\ncustom: keep me\n---\n\n# E2E Preserved Book\n\nThe original review remains visible.\n"), 0o600); err != nil {
		return fmt.Errorf("seed preserved browser fixture book: %w", err)
	}

	appAddr := envOr("BRAIND_E2E_APP_ADDR", appAddress)
	proxyAddr := envOr("BRAIND_E2E_PROXY_ADDR", proxyAddress)
	controlAddr := envOr("BRAIND_E2E_CONTROL_ADDR", controlAddress)
	proxyListener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		return fmt.Errorf("listen for browser HTTPS: %w", err)
	}
	defer proxyListener.Close()

	appURL, err := url.Parse("http://" + appAddr)
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(appURL)
	proxyTransport := &http.Transport{}
	proxy.Transport = proxyTransport
	tlsProxy := httptest.NewUnstartedServer(proxy)
	tlsProxy.Listener = proxyListener
	tlsProxy.StartTLS()
	defer tlsProxy.Close()
	controlListener, err := net.Listen("tcp", controlAddr)
	if err != nil {
		return fmt.Errorf("listen for browser fixture controls: %w", err)
	}
	controlTCP, ok := controlListener.Addr().(*net.TCPAddr)
	if !ok || !controlTCP.IP.IsLoopback() {
		controlListener.Close()
		return fmt.Errorf("browser fixture controls must listen on a loopback address, got %s", controlListener.Addr())
	}
	defer controlListener.Close()
	controlServer := &http.Server{Handler: fixtureControl(provider)}
	go func() { _ = controlServer.Serve(controlListener) }()
	defer controlServer.Close()

	setEnv := map[string]string{
		"BRAIND_LISTEN_ADDR":               appAddr,
		"BRAIND_EXTERNAL_URL":              tlsProxy.URL,
		"BRAIND_DATA_PATH":                 dataDir,
		"BRAIND_VAULT_PATH":                filepath.Join(dataDir, "vault"),
		"BRAIND_GIT_DIR":                   filepath.Join(dataDir, "git"),
		"BRAIND_SYNC_ENABLED":              "false",
		"BRAIND_GIT_BACKUP_ENABLED":        "false",
		"BRAIND_READ_ONLY":                 "false",
		"OIDC_CONFIGURATION_URL":           provider.URL(),
		"OIDC_CLIENT_ID":                   authtest.ClientID,
		"OIDC_CLIENT_SECRET":               authtest.ClientSecret,
		"OIDC_ADMIN_GROUP":                 "access-brain-admin",
		"OIDC_USER_GROUP":                  "access-brain-user",
		"OIDC_VIEWER_GROUP":                "access-brain-viewer",
		"OIDC_AUTO_REDIRECT":               "true",
		"OIDC_PROVIDER_NAME":               "e2e-test",
		"BRAIND_SESSION_IDLE_TIMEOUT":      "1h",
		"BRAIND_SESSION_ABSOLUTE_LIFETIME": "2h",
		"BRAIND_SHUTDOWN_TIMEOUT":          "15s",
	}
	for key, value := range setEnv {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := make(chan int, 1)
	go func() { result <- braind.Run(ctx, nil, "e2e", "", io.Discard, os.Stderr) }()
	if err := waitUntilReady(ctx, tlsProxy); err != nil {
		stop()
		<-result
		return err
	}
	fmt.Printf("E2E_READY %s\n", tlsProxy.URL)
	<-ctx.Done()
	// Close browser and reverse-proxy keep-alive connections before asking the
	// application server to drain, so test teardown does not wait on idle clients.
	tlsProxy.CloseClientConnections()
	proxyTransport.CloseIdleConnections()
	_ = controlServer.Close()
	if code := <-result; code != 0 {
		return fmt.Errorf("braind fixture exited with status %d", code)
	}
	return nil
}

// fixtureControl allows sequential browser tests to sign in with different
// OIDC roles. It listens only on the fixture's loopback interface.
func fixtureControl(provider *authtest.Provider) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /__test/role", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		groups := map[string][]string{
			"admin":  {"access-brain-admin"},
			"user":   {"access-brain-user"},
			"viewer": {"access-brain-viewer"},
		}[body.Role]
		if groups == nil {
			http.Error(w, "unsupported role", http.StatusBadRequest)
			return
		}
		provider.SetClaims(map[string]any{
			"email":  "test-" + body.Role + "@example.test",
			"groups": groups,
		})
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func waitUntilReady(ctx context.Context, server *httptest.Server) error {
	client := server.Client()
	client.Timeout = time.Second
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/healthz", nil)
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("braind did not become ready at %s", server.URL)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
