// Package braind implements the HTTP daemon lifecycle.
package braind

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/johnrowl/brain/internal/braind/config"
	"github.com/johnrowl/brain/internal/braind/datalock"
	"github.com/johnrowl/brain/internal/braind/lifecycle"
	"github.com/johnrowl/brain/internal/braind/logging"
)

const (
	defaultShutdownTimeout = 5 * time.Second
	readHeaderTimeout      = 5 * time.Second
)

// Run parses command-line arguments, listens for HTTP requests, and blocks
// until the context is canceled or the server fails.
func Run(ctx context.Context, args []string, version string, stdout, stderr io.Writer) int {
	return run(ctx, args, version, stdout, stderr, os.LookupEnv)
}

func run(ctx context.Context, args []string, version string, stdout, stderr io.Writer, lookup config.LookupEnv) (exitCode int) {
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
	processLifecycle := lifecycle.New()

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
	serverLogger.Info("server listening", "version", version, "address", listener.Addr().String())
	if err := serve(ctx, listener, logging.HTTPMiddleware(logger, Handler()), defaultShutdownTimeout, processLifecycle.BeginShutdown); err != nil {
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

// Handler returns the daemon's HTTP handler. FND-001 deliberately exposes
// only process liveness; readiness and application routes are later tasks.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	return mux
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
