package braind

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/lifecycle"
	"github.com/johnrowl/brain/internal/braind/logging"
)

// wiredHandler returns a ready handler: lifecycle running plus an accessible
// vault directory.
func wiredHandler(t *testing.T) (http.Handler, *lifecycle.Lifecycle, string) {
	t.Helper()
	vault := t.TempDir()
	process := lifecycle.New()
	if err := process.MarkRunning(); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	return newHandler(handlerState{process: process, vaultPath: vault}), process, vault
}

func TestReadyzServes(t *testing.T) {
	handler, _, _ := wiredHandler(t)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if body := recorder.Body.String(); body != "ready\n" {
		t.Fatalf("unexpected body %q", body)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("unexpected Cache-Control header: %q", got)
	}
}

func TestReadyzFailsClosed(t *testing.T) {
	t.Run("unwired handler", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
		}
		if body := recorder.Body.String(); body != "not ready\n" {
			t.Fatalf("unexpected body %q", body)
		}
	})

	t.Run("vault becomes inaccessible during operation", func(t *testing.T) {
		vault := filepath.Join(t.TempDir(), "vault")
		if err := os.Mkdir(vault, 0o700); err != nil {
			t.Fatalf("create vault: %v", err)
		}
		process := lifecycle.New()
		if err := process.MarkRunning(); err != nil {
			t.Fatalf("mark running: %v", err)
		}
		handler := newHandler(handlerState{process: process, vaultPath: vault})

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected ready before vault loss, got %d", recorder.Code)
		}

		if err := os.RemoveAll(vault); err != nil {
			t.Fatalf("remove vault: %v", err)
		}
		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 after vault loss, got %d", recorder.Code)
		}

		if err := os.Mkdir(vault, 0o700); err != nil {
			t.Fatalf("recreate vault: %v", err)
		}
		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected readiness recovery, got %d", recorder.Code)
		}
	})

	t.Run("liveness survives vault loss", func(t *testing.T) {
		vault := filepath.Join(t.TempDir(), "vault")
		process := lifecycle.New()
		if err := process.MarkRunning(); err != nil {
			t.Fatalf("mark running: %v", err)
		}
		handler := newHandler(handlerState{process: process, vaultPath: vault})

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected not-ready with missing vault, got %d", recorder.Code)
		}
		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("liveness must remain successful while alive (US2-AC1), got %d", recorder.Code)
		}
	})
}

func TestReadyzRejectsOtherMethods(t *testing.T) {
	handler, _, _ := wiredHandler(t)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/readyz", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, recorder.Code)
	}
}

// TestProbeRoutesCarryRequestIDs proves FR-007 correlation on both new routes:
// a valid inbound identifier is echoed and used in exactly one completion log;
// without one, a generated 32-hex identifier takes its place.
func TestProbeRoutesCarryRequestIDs(t *testing.T) {
	var logs bytes.Buffer
	logger, err := logging.New(&logs, io.Discard, "debug")
	if err != nil {
		t.Fatalf("create fixture logger: %v", err)
	}
	handler, _, _ := statusFixture(t)
	wrapped := logging.HTTPMiddleware(logger, handler)

	tests := []struct {
		name    string
		inbound string
		want    string
	}{
		{name: "echoed", inbound: "trace-me-123", want: "trace-me-123"},
		{name: "generated", inbound: "", want: "generated-32-hex"},
	}
	for _, path := range []string{"/readyz", "/status"} {
		for _, test := range tests {
			t.Run(strings.TrimPrefix(path, "/")+":"+test.name, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				if test.inbound != "" {
					request.Header.Set(logging.RequestIDHeader, test.inbound)
				}
				recorder := httptest.NewRecorder()
				wrapped.ServeHTTP(recorder, request)

				got := recorder.Header().Get(logging.RequestIDHeader)
				if test.want == "generated-32-hex" {
					if len(got) != 32 {
						t.Fatalf("generated request id is not 32 characters: %q", got)
					}
					for _, character := range got {
						if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
							t.Fatalf("generated request id is not hex: %q", got)
						}
					}
					return
				}
				if got != test.want {
					t.Fatalf("expected echoed request id %q, got %q", test.want, got)
				}
			})
		}
	}

	echoedCount := 0
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, `"request completed"`) && strings.Contains(line, `"request_id":"trace-me-123"`) {
			echoedCount++
		}
	}
	if echoedCount != 2 {
		t.Fatalf("expected one completion log per echoed request (2), got %d", echoedCount)
	}
}

func TestReadyzReportsShutdownBeforeExit(t *testing.T) {
	stateHandler, process, _ := wiredHandler(t)

	// Hold one request open so the graceful drain window is observable. The
	// contract (SC-003) is that readiness becomes unsuccessful before the
	// process exits: during the drain, in-process readiness is already false
	// and no external probe reports success (server.Shutdown closes listeners
	// before draining, so probes answer 503 or refuse connection — either is
	// an unsuccessful readiness result).
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			select {
			case <-requestStarted:
			default:
				close(requestStarted)
			}
			<-releaseRequest
			w.WriteHeader(http.StatusNoContent)
			return
		}
		stateHandler.ServeHTTP(w, r)
	})

	listener := listenOnLoopback(t)
	beginShutdown := func() bool { return process.BeginShutdown() }
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { done <- serve(ctx, listener, handler, 5*time.Second, beginShutdown) }()
	var release sync.Once
	t.Cleanup(func() {
		release.Do(func() { close(releaseRequest) })
		cancel()
		select {
		case <-done: // serve already exited in the test body
		default: // still draining; cancellation and release will end it
		}
	})

	client := &http.Client{Timeout: 2 * time.Second}
	address := "http://" + listener.Addr().String()

	response, err := client.Get(address + "/readyz")
	if err != nil {
		t.Fatalf("readiness probe before shutdown: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected ready before shutdown, got %d", response.StatusCode)
	}

	inFlight := make(chan struct{})
	go func() {
		defer close(inFlight)
		response, err := client.Get(address + "/slow")
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
	}()
	<-requestStarted

	cancel()

	// Readiness must flip before Serve returns, while the held request keeps
	// the drain open.
	deadline := time.Now().Add(time.Second)
	for process.Snapshot().Ready {
		if time.Now().After(deadline) {
			t.Fatal("readiness stayed available after cancellation")
		}
		time.Sleep(2 * time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("serve returned before the held request drained: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	probeSucceeded := false
	if response, err := client.Get(address + "/readyz"); err == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		probeSucceeded = response.StatusCode == http.StatusOK
	}
	if probeSucceeded {
		t.Fatal("readiness probe reported success during shutdown drain")
	}

	release.Do(func() { close(releaseRequest) })
	<-inFlight
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned an error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after draining the held request")
	}
}
