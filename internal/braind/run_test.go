package braind

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/config"
)

func TestHandlerHealthz(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Body.String(); got != "ok\n" {
		t.Fatalf("unexpected response body: %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("unexpected Cache-Control header: %q", got)
	}
}

func TestHandlerRejectsOtherMethodsAndPaths(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		status int
	}{
		{name: "post health", method: http.MethodPost, path: "/healthz", status: http.StatusMethodNotAllowed},
		{name: "unknown path", method: http.MethodGet, path: "/readyz", status: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, nil)
			Handler().ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("expected status %d, got %d", test.status, recorder.Code)
			}
		})
	}
}

func TestServeStopsAfterCancellation(t *testing.T) {
	listener := listenOnLoopback(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, listener, Handler(), time.Second)
	}()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil {
		t.Fatalf("request health endpoint: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close health response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned an error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after cancellation")
	}
}

func TestServeForcesBoundedShutdown(t *testing.T) {
	listener := listenOnLoopback(t)
	ctx, cancel := context.WithCancel(context.Background())
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
		w.WriteHeader(http.StatusNoContent)
	})

	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, listener, handler, 25*time.Millisecond)
	}()

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := (&http.Client{Timeout: time.Second}).Get("http://" + listener.Addr().String())
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	<-requestStarted
	cancel()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "graceful shutdown") {
			t.Fatalf("expected graceful shutdown timeout, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server shutdown was not bounded")
	}
	close(releaseRequest)
	<-requestDone
}

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"--version"}, "test-version", &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "braind test-version\n" {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestRunRejectsInvalidArguments(t *testing.T) {
	tests := [][]string{
		{"--not-a-real-flag"},
		{"unexpected-position"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if got := Run(context.Background(), args, "test", &stdout, &stderr); got != 2 {
				t.Fatalf("expected exit code 2, got %d", got)
			}
		})
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(context.Background(), nil, "test", &stdout, &stderr, mapLookup(map[string]string{}))

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), config.EnvOIDCClientSecret) {
		t.Fatalf("configuration error did not identify missing setting: %q", stderr.String())
	}
}

func TestRunConfigurationLogDoesNotDiscloseConfiguredSecrets(t *testing.T) {
	const secret = "invalid-configuration-client-secret"
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	environment := map[string]string{
		config.EnvOIDCClientSecret: secret,
	}

	exitCode := run(context.Background(), nil, "test", &stdout, &stderr, mapLookup(environment))

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if strings.Contains(stderr.String(), secret) || strings.Contains(stdout.String(), secret) {
		t.Fatalf("configuration log disclosed secret: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunUsesEnvironmentConfiguration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	environment := map[string]string{
		config.EnvListenAddr:       "127.0.0.1:0",
		config.EnvOIDCEnabled:      "false",
		config.EnvGitBackupEnabled: "false",
	}

	exitCode := run(ctx, nil, "test", &stdout, &stderr, mapLookup(environment))

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"msg":"server listening"`) || !strings.Contains(stdout.String(), `"address":"127.0.0.1:`) {
		t.Fatalf("startup output did not contain configured listener: %q", stdout.String())
	}
}

func TestServeRejectsNonPositiveShutdownTimeout(t *testing.T) {
	listener := listenOnLoopback(t)
	defer listener.Close()
	if err := Serve(context.Background(), listener, Handler(), 0); err == nil {
		t.Fatal("expected an error for a non-positive shutdown timeout")
	}
}

func listenOnLoopback(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on loopback: %v", err)
	}
	return listener
}

func mapLookup(values map[string]string) config.LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
