package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	RequestIDHeader        = "X-Request-ID"
	maximumRequestIDLength = 128
)

type requestIDContextKey struct{}

// RequestID returns the request correlation identifier stored in ctx.
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

// HTTPMiddleware adds a request ID to the request context and response, then
// emits one completion record without inspecting headers, query parameters, or
// request and response bodies.
func HTTPMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if next == nil {
		next = http.NotFoundHandler()
	}
	httpLogger := logger.With("component", "http")

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		requestID := strings.TrimSpace(request.Header.Get(RequestIDHeader))
		if !validRequestID(requestID) {
			requestID = newRequestID()
		}

		writer.Header().Set(RequestIDHeader, requestID)
		capture := &statusWriter{ResponseWriter: writer}
		request = request.WithContext(context.WithValue(request.Context(), requestIDContextKey{}, requestID))
		defer func() {
			status := capture.statusCode()
			if recovered := recover(); recovered != nil {
				status = http.StatusInternalServerError
				logRequest(httpLogger, request, requestID, status, started)
				panic(recovered)
			}
			logRequest(httpLogger, request, requestID, status, started)
		}()
		next.ServeHTTP(capture, request)
	})
}

func logRequest(logger *slog.Logger, request *http.Request, requestID string, status int, started time.Time) {
	logger.InfoContext(request.Context(), "request completed",
		"request_id", requestID,
		"method", request.Method,
		"path", request.URL.Path,
		"status", status,
		"duration", time.Since(started),
	)
}

func validRequestID(value string) bool {
	if value == "" || len(value) > maximumRequestIDLength {
		return false
	}
	for _, character := range []byte(value) {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			continue
		}
		switch character {
		case '-', '_', '.', ':':
			continue
		default:
			return false
		}
	}
	return true
}

func newRequestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		// crypto/rand failures make safely correlating requests impossible. Panic
		// rather than silently falling back to a predictable identifier.
		panic("generate request ID: " + err.Error())
	}
	return hex.EncodeToString(random[:])
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}
