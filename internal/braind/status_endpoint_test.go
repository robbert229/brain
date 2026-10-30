package braind

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/config"
	"github.com/johnrowl/brain/internal/braind/lifecycle"
	"github.com/johnrowl/brain/internal/braind/status"
)

// zeroConfig returns an untouched Config whose subsystems are all disabled.
func zeroConfig() config.Config { return config.Config{} }

func statusFixture(t *testing.T) (http.Handler, *status.Store, *lifecycle.Lifecycle) {
	t.Helper()
	process := lifecycle.New()
	store := status.New(status.Initial("test-version", "test-commit", zeroConfig(), process.Snapshot(), time.Now()), "s3cr3t-oidc-secret")
	if err := process.MarkRunning(); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	store.SetLifecycle(process.Snapshot())
	return newHandler(handlerState{
		process:     process,
		vaultPath:   t.TempDir(),
		statusStore: store,
	}), store, process
}

func TestStatusServesPublicSnapshot(t *testing.T) {
	handler, _, _ := statusFixture(t)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("unexpected Cache-Control header: %q", got)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected Content-Type: %q", got)
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode status document: %v", err)
	}
	for _, section := range []string{"build", "lifecycle", "vault", "oidc", "search", "sync", "backup"} {
		if _, ok := document[section]; !ok {
			t.Fatalf("status document missing section %q: %s", section, recorder.Body.String())
		}
	}

	var public status.PublicSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &public); err != nil {
		t.Fatalf("decode status snapshot: %v", err)
	}
	if public.Build.Version != "test-version" {
		t.Fatalf("unexpected build version: %q", public.Build.Version)
	}
	if public.Build.Commit != "test-commit" {
		t.Fatalf("unexpected build commit: %q", public.Build.Commit)
	}
	if public.Lifecycle.State != lifecycle.Running || !public.Lifecycle.Ready {
		t.Fatalf("unexpected lifecycle section: %+v", public.Lifecycle)
	}
	if public.OIDC.Discovery != status.OIDCPending || public.Sync.State != status.SyncDisabled || public.Backup.State != status.BackupDisabled {
		t.Fatalf("subsystems must report their disabled/initial states: %+v", public)
	}
}

func TestStatusRedactsConfiguredSecrets(t *testing.T) {
	handler, store, _ := statusFixture(t)

	store.Update(func(snapshot *status.Snapshot) {
		snapshot.Backup.LastError = "push failed with token s3cr3t-oidc-secret"
		snapshot.Sync.OutputTail = []string{"stderr: s3cr3t-oidc-secret"}
	})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
	if body := recorder.Body.String(); containsSecret(body) {
		t.Fatalf("status document disclosed a configured secret: %s", body)
	}
}

// TestStatusConcurrentReaders never tears: each response is a complete snapshot.
func TestStatusConcurrentReaders(t *testing.T) {
	handler, store, _ := statusFixture(t)

	var writers sync.WaitGroup
	stop := make(chan struct{})
	for writerIndex := 0; writerIndex < 4; writerIndex++ {
		writers.Add(1)
		go func(index int) {
			defer writers.Done()
			for count := 0; ; count++ {
				select {
				case <-stop:
					return
				default:
				}
				store.Update(func(snapshot *status.Snapshot) {
					snapshot.Search.Results = index*1000 + count
				})
			}
		}(writerIndex)
	}

	recorders := make(chan int, 32)
	for requestIndex := 0; requestIndex < 32; requestIndex++ {
		go func() {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
			recorders <- recorder.Code
		}()
	}
	for requestIndex := 0; requestIndex < 32; requestIndex++ {
		if code := <-recorders; code != http.StatusOK {
			t.Fatalf("concurrent reader got status %d", code)
		}
	}
	close(stop)
	writers.Wait()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
	var public status.PublicSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &public); err != nil {
		t.Fatalf("decode after concurrent updates: %v", err)
	}
}

func TestStatusReportsShuttingDownBeforeExit(t *testing.T) {
	handler, store, process := statusFixture(t)

	process.BeginShutdown()
	store.SetLifecycle(process.Snapshot())

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
	var public status.PublicSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &public); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if public.Lifecycle.State != lifecycle.ShuttingDown {
		t.Fatalf("expected shutting-down state during drain, got %q", public.Lifecycle.State)
	}
}

func TestStatusRejectsOtherMethods(t *testing.T) {
	handler, _, _ := statusFixture(t)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/status", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, recorder.Code)
	}
}

func TestStatusWithoutWiringServesEmptySnapshot(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	var document map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
		t.Fatalf("unwired status is not JSON: %v", err)
	}
}

func containsSecret(body string) bool {
	return strings.Contains(body, "s3cr3t-oidc-secret")
}
