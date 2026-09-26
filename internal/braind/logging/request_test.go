package logging

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestHTTPMiddlewarePropagatesInboundRequestIDWithoutLoggingRequestData(t *testing.T) {
	const (
		requestID           = "upstream-request_123"
		authorizationMarker = "authorization-secret"
		cookieMarker        = "cookie-secret"
		queryMarker         = "query-secret"
		formMarker          = "private-form-body"
		noteMarker          = "private-note-body"
		responseMarker      = "private-response-body"
	)
	requestBody := "password=" + formMarker + "&note=" + noteMarker

	var logs bytes.Buffer
	logger, err := New(&logs, nil, "info")
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	handler := HTTPMiddleware(logger, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := RequestID(request.Context()); got != requestID {
			t.Errorf("unexpected request ID in context: %q", got)
		}
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Fatalf("read request body: %v", readErr)
		}
		if string(body) != requestBody {
			t.Errorf("middleware changed request body: %q", body)
		}
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, responseMarker)
	}))

	request := httptest.NewRequest(http.MethodPost, "/notes/example?access_token="+queryMarker, strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set(RequestIDHeader, requestID)
	request.Header.Set("Authorization", "Bearer "+authorizationMarker)
	request.AddCookie(&http.Cookie{Name: "session", Value: cookieMarker})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get(RequestIDHeader); got != requestID {
		t.Fatalf("unexpected response request ID: %q", got)
	}
	record := decodeRecord(t, logs.String())
	if record["component"] != "http" || record["request_id"] != requestID || record["method"] != http.MethodPost || record["path"] != "/notes/example" || record["status"] != float64(http.StatusCreated) {
		t.Fatalf("unexpected request record: %#v", record)
	}
	if _, ok := record["duration"]; !ok {
		t.Fatalf("request record has no duration: %#v", record)
	}
	for _, marker := range []string{authorizationMarker, cookieMarker, queryMarker, formMarker, noteMarker, responseMarker} {
		if strings.Contains(logs.String(), marker) {
			t.Errorf("request log disclosed %q: %s", marker, logs.String())
		}
	}
}

func TestHTTPMiddlewareReplacesUnsafeRequestID(t *testing.T) {
	var logs bytes.Buffer
	logger, err := New(&logs, nil, "info")
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	handler := HTTPMiddleware(logger, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if RequestID(request.Context()) == "unsafe request id" {
			t.Error("unsafe request ID reached handler context")
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set(RequestIDHeader, "unsafe request id")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	generated := recorder.Header().Get(RequestIDHeader)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(generated) {
		t.Fatalf("generated request ID has unexpected form: %q", generated)
	}
	if strings.Contains(logs.String(), "unsafe request id") {
		t.Fatalf("unsafe inbound request ID reached logs: %s", logs.String())
	}
	if record := decodeRecord(t, logs.String()); record["request_id"] != generated {
		t.Fatalf("log request ID does not match response: %#v", record)
	}
}
