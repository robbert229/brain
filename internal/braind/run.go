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
	"time"
)

const (
	defaultListenAddress   = "0.0.0.0:8080"
	defaultShutdownTimeout = 5 * time.Second
	readHeaderTimeout      = 5 * time.Second
)

// Run parses command-line arguments, listens for HTTP requests, and blocks
// until the context is canceled or the server fails.
func Run(ctx context.Context, args []string, version string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("braind", flag.ContinueOnError)
	fs.SetOutput(stderr)

	listenAddress := fs.String("listen", defaultListenAddress, "HTTP listen address")
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

	listener, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		fmt.Fprintf(stderr, "braind: listen: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "braind %s listening on %s\n", version, listener.Addr())
	if err := Serve(ctx, listener, Handler(), defaultShutdownTimeout); err != nil {
		fmt.Fprintf(stderr, "braind: serve: %v\n", err)
		return 1
	}

	return 0
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
		return normalizeServeError(err)
	case <-ctx.Done():
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

func normalizeServeError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
