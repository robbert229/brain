// Package braind implements the HTTP daemon lifecycle.
package braind

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/config"
	"github.com/johnrowl/brain/internal/braind/datalock"
	"github.com/johnrowl/brain/internal/braind/lifecycle"
	"github.com/johnrowl/brain/internal/braind/logging"
	"github.com/johnrowl/brain/internal/braind/status"
)

const (
	readHeaderTimeout = 5 * time.Second
)

// envFile is the dot-env file loaded by Run, relative to the current working
// directory. It is a package-level variable only so tests can point it at a
// temporary file; production behavior is the default ".env".
var envFile = ".env"

// Run parses command-line arguments, listens for HTTP requests, and blocks
// until the context is canceled or the server fails. Configuration is read
// from the process environment, overlaid with values from the dot-env file
// (envFile, ".env" in the current working directory) when one exists;
// process environment values take precedence.
func Run(ctx context.Context, args []string, version, commit string, stdout, stderr io.Writer) int {
	lookup, err := config.LoadDotEnv(os.LookupEnv, envFile)
	if err != nil {
		fmt.Fprintf(stderr, "braind: .env: %v\n", err)
		return 1
	}
	return run(ctx, args, version, commit, stdout, stderr, lookup)
}

func run(ctx context.Context, args []string, version, commit string, stdout, stderr io.Writer, lookup config.LookupEnv) (exitCode int) {
	startedAt := time.Now()
	fs := flag.NewFlagSet("braind", flag.ContinueOnError)
	fs.SetOutput(stderr)

	showVersion := fs.Bool("version", false, "print version")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "braind does not accept positional arguments")
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "braind %s\n", version)
		return 0
	}

	processConfig, err := config.Load(lookup)
	if err != nil {
		bootstrapLogger, _ := logging.New(stdout, stderr, "info", rawLogSecrets(lookup)...)
		bootstrapLogger.Error("invalid configuration", "component", "config", "error", err)
		return 1
	}

	oidcConfig := processConfig.OIDC()
	backupConfig := processConfig.Backup()
	logger, err := logging.New(stdout, stderr, processConfig.LogLevel(), oidcConfig.ClientSecret, backupConfig.RemoteURL)
	if err != nil {
		fmt.Fprintf(stderr, "braind: logging: %v\n", err)
		return 1
	}
	serverLogger := logger.With("component", "server")
	authSurface, err := newAuthSurface(processConfig, logger)
	if err != nil {
		// OIDC is mandatory: an unusable sign-in surface is fatal, because
		// serving without it would expose protected routes anonymously.
		// A provider that is merely unreachable stays non-fatal (FR-013);
		// that path runs through the discovery watch, not here.
		serverLogger.Error("authentication unavailable", "error", err)
		return 1
	}
	defer authSurface.Close()
	processLifecycle := lifecycle.New()
	statusStore := status.New(
		status.Initial(version, commit, processConfig, processLifecycle.Snapshot(), startedAt),
		oidcConfig.ClientSecret,
		backupConfig.RemoteURL,
	)

	dataLock, err := datalock.Acquire(processConfig.Vault().DataPath)
	if err != nil {
		serverLogger.Error("data lock failed", "error", err)
		return 1
	}
	defer func() {
		if err := dataLock.Release(); err != nil {
			serverLogger.Error("data lock release failed", "error", err)
			exitCode = 1
		}
	}()

	listener, err := net.Listen("tcp", processConfig.Server().ListenAddr)
	if err != nil {
		serverLogger.Error("listener failed", "error", err)
		return 1
	}

	if err := processLifecycle.MarkRunning(); err != nil {
		serverLogger.Error("lifecycle transition failed", "error", err)
		return 1
	}
	statusStore.SetLifecycle(processLifecycle.Snapshot())
	beginShutdown := func() bool {
		changed := processLifecycle.BeginShutdown()
		statusStore.SetLifecycle(processLifecycle.Snapshot())
		return changed
	}
	serverLogger.Info("server listening", "version", version, "address", listener.Addr().String())
	go authSurface.watch(ctx, statusStore)
	serverHandler := newHandler(handlerState{
		process:     processLifecycle,
		vaultPath:   processConfig.Vault().VaultPath,
		statusStore: statusStore,
		auth:        authSurface,
		books:       newBooksSurface(processConfig.Vault()),
	})
	if err := serve(ctx, listener, logging.HTTPMiddleware(logger, serverHandler), processConfig.Server().ShutdownTimeout, beginShutdown); err != nil {
		serverLogger.Error("server stopped with error", "error", err)
		return 1
	}
	serverLogger.Info("server stopped")

	return 0
}

func rawLogSecrets(lookup config.LookupEnv) []string {
	if lookup == nil {
		return nil
	}
	secrets := make([]string, 0, 4)
	for _, name := range []string{config.EnvOIDCClientSecret, config.EnvGitRemoteURL} {
		if value, ok := lookup(name); ok && value != "" {
			secrets = append(secrets, value)
			if trimmed := strings.TrimSpace(value); trimmed != value && trimmed != "" {
				secrets = append(secrets, trimmed)
			}
		}
	}
	return secrets
}

// Handler returns the daemon's HTTP handler without operational state, serving
// liveness, the landing page (spec 009), and its static assets. The unready
// readiness surface answers /readyz with 503 (contracts/http.md).
func Handler() http.Handler { return newHandler(handlerState{}) }

// handlerState bundles the concrete operational state that probe routes read.
// Zero values mean "not wired"; readiness then fails closed.
type handlerState struct {
	process     *lifecycle.Lifecycle // process lifecycle, nil before lifecycle wiring
	vaultPath   string               // local vault directory probed by readiness
	statusStore *status.Store        // operational snapshot publisher, nil until wired
	auth        *authSurface         // OIDC sign-in surface, nil only for the unwired Handler() export
	books       *booksSurface        // bookshelf surface (spec 010), nil only for the unwired export
}

func newHandler(state handlerState) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	// FND-006 (ADR-0003): readiness reports local serving capability, never
	// upstream OIDC/Sync/Backup state.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if readiness(state) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "ready\n")
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "not ready\n")
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticAssetsHandler()))
	// FR-008: the snapshot store marshals only the privacy-safe public view.
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		encoder := json.NewEncoder(w)
		if state.statusStore != nil {
			_ = encoder.Encode(state.statusStore)
			return
		}
		_ = encoder.Encode(status.Snapshot{})
	})
	// "GET /{$}" matches only the exact root path so unknown paths keep
	// returning 404 (contracts/http.md; run_test.go rejection cases). The
	// index page sits behind the shared authentication decision whenever the
	// sign-in surface is wired; the unwired bootstrap export stays anonymous
	// because it has no auth surface to consult.
	var landing http.Handler = http.HandlerFunc(serveLanding)
	if state.auth != nil {
		landing = state.auth.handler.RequireAuth(auth.Policy{}, landing)
	}
	mux.Handle("GET /{$}", landing)
	// Spec 010: the books shelf shares the sign-in surface; without one wired
	// its routes stay absent, like the sign-in routes themselves.
	if state.auth != nil && state.books != nil {
		state.books.register(mux, state.auth.handler)
	}
	// Spec 002: the sign-in surface is mandatory and always registered in run().
	// Probes, status, and static assets stay registered above so they remain
	// reachable without a provider round trip (ADR-0003).
	state.auth.register(mux)
	return mux
}

// readiness is the single readiness decision: the process lifecycle must
// report ready AND the local vault must be accessible (FR-004, FR-005).
func readiness(state handlerState) bool {
	if state.process == nil || !state.process.Snapshot().Ready {
		return false
	}
	return vaultAccessible(state.vaultPath)
}

// vaultAccessible reports whether path is a directory whose contents can be
// read. It fails closed on any error and never caches (research D2): the next
// probe always sees current truth.
func vaultAccessible(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	dir, err := os.Open(path)
	if err != nil {
		return false
	}
	defer dir.Close()
	if _, err := dir.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		return false // io.EOF only means the directory is empty: accessible
	}
	return true
}

// Serve runs an HTTP server on listener. Canceling ctx begins graceful
// shutdown; connections are forcibly closed if the deadline expires.
func Serve(ctx context.Context, listener net.Listener, handler http.Handler, shutdownTimeout time.Duration) error {
	return serve(ctx, listener, handler, shutdownTimeout, nil)
}

func serve(ctx context.Context, listener net.Listener, handler http.Handler, shutdownTimeout time.Duration, beginShutdown func() bool) error {
	if shutdownTimeout <= 0 {
		return errors.New("shutdown timeout must be positive")
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		callBeginShutdown(beginShutdown)
		return normalizeServeError(err)
	case <-ctx.Done():
		callBeginShutdown(beginShutdown)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		err := server.Shutdown(shutdownCtx)
		cancel()
		if err != nil {
			closeErr := server.Close()
			if closeErr != nil {
				return errors.Join(fmt.Errorf("graceful shutdown: %w", err), fmt.Errorf("close server: %w", closeErr))
			}
			return fmt.Errorf("graceful shutdown: %w", err)
		}

		return normalizeServeError(<-serveErr)
	}
}

func callBeginShutdown(beginShutdown func() bool) {
	if beginShutdown != nil {
		beginShutdown()
	}
}

func normalizeServeError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
