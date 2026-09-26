package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggerEmitsJSONByLevelAndComponent(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	logger, err := New(&stdout, &stderr, "info")
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	logger.Debug("filtered")
	logger.With("component", "server").Info("started", "address", "127.0.0.1:8080")
	logger.With("component", "sync").Error("stopped", "exit_code", 7)

	if strings.Contains(stdout.String(), "filtered") || strings.Contains(stderr.String(), "filtered") {
		t.Fatal("debug record was not filtered at info level")
	}
	info := decodeRecord(t, stdout.String())
	if info["level"] != "INFO" || info["msg"] != "started" || info["component"] != "server" || info["address"] != "127.0.0.1:8080" {
		t.Fatalf("unexpected stdout record: %#v", info)
	}
	if _, ok := info["time"]; !ok {
		t.Fatalf("stdout record has no timestamp: %#v", info)
	}
	errorRecord := decodeRecord(t, stderr.String())
	if errorRecord["level"] != "ERROR" || errorRecord["component"] != "sync" || errorRecord["exit_code"] != float64(7) {
		t.Fatalf("unexpected stderr record: %#v", errorRecord)
	}
}

func TestLoggerRedactsSensitiveData(t *testing.T) {
	const (
		clientSecret = "configured-client-secret-value"
		remoteURL    = "ssh://git@example.test/private-vault.git"
	)
	markers := []string{
		"opaque-access-token",
		"session-cookie-value",
		"basic-authorization-value",
		"private-form-body",
		"private-note-body",
		"nested-refresh-token",
		"nested-arbitrary-token",
		clientSecret,
		remoteURL,
	}

	var stdout bytes.Buffer
	logger, err := New(&stdout, nil, "debug", clientSecret, remoteURL)
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	logger = logger.With("client_secret", clientSecret)
	logger.Info("backup failed for "+remoteURL,
		"access_token", markers[0],
		"cookie", markers[1],
		"authorization", markers[2],
		"form_body", markers[3],
		"note_body", markers[4],
		"error", errors.New("provider rejected "+clientSecret),
		"details", map[string]string{"token": markers[6]},
		slog.Group("oidc", "refresh_token", markers[5], "issuer", "https://issuer.example.test"),
	)
	logger.WithGroup("credentials").Info("credential group", "value", "grouped-secret-value")
	markers = append(markers, "grouped-secret-value")

	output := stdout.String()
	for _, marker := range markers {
		if strings.Contains(output, marker) {
			t.Errorf("structured log disclosed %q: %s", marker, output)
		}
	}
	if count := strings.Count(output, redacted); count < 7 {
		t.Fatalf("expected redacted values in log, found %d: %s", count, output)
	}
	decodeRecord(t, strings.Split(strings.TrimSpace(output), "\n")[0])
}

func TestNewRejectsUnknownLevel(t *testing.T) {
	if _, err := New(nil, nil, "verbose"); err == nil {
		t.Fatal("expected unknown log level to fail")
	}
}

func decodeRecord(t *testing.T, raw string) map[string]any {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &record); err != nil {
		t.Fatalf("decode log record %q: %v", raw, err)
	}
	return record
}
