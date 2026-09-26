// Package logging provides braind's structured logging and redaction policy.
package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"strings"
	"unicode"
)

const redacted = "[redacted]"

// New returns a JSON logger that writes levels below error to stdout and error
// and above to stderr. Configured secret values are removed from messages and
// attributes in addition to the key-based redaction policy.
func New(stdout, stderr io.Writer, level string, secrets ...string) (*slog.Logger, error) {
	minimumLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	options := &slog.HandlerOptions{Level: minimumLevel}
	handler := splitHandler{
		stdout: slog.NewJSONHandler(stdout, options),
		stderr: slog.NewJSONHandler(stderr, options),
	}
	return slog.New(redactingHandler{
		next:     handler,
		redactor: newRedactor(secrets),
	}), nil
}

func parseLevel(value string) (slog.Level, error) {
	switch value {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, errors.New("log level must be one of debug, info, warn, error")
	}
}

type splitHandler struct {
	stdout slog.Handler
	stderr slog.Handler
}

func (h splitHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler(level).Enabled(ctx, level)
}

func (h splitHandler) Handle(ctx context.Context, record slog.Record) error {
	return h.handler(record.Level).Handle(ctx, record)
}

func (h splitHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return splitHandler{
		stdout: h.stdout.WithAttrs(attrs),
		stderr: h.stderr.WithAttrs(attrs),
	}
}

func (h splitHandler) WithGroup(name string) slog.Handler {
	return splitHandler{
		stdout: h.stdout.WithGroup(name),
		stderr: h.stderr.WithGroup(name),
	}
}

func (h splitHandler) handler(level slog.Level) slog.Handler {
	if level >= slog.LevelError {
		return h.stderr
	}
	return h.stdout
}

type redactingHandler struct {
	next      slog.Handler
	redactor  redactor
	redactAll bool
}

func (h redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, h.redactor.string(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(h.cleanAttr(attr))
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for index, attr := range attrs {
		clean[index] = h.cleanAttr(attr)
	}
	return redactingHandler{next: h.next.WithAttrs(clean), redactor: h.redactor, redactAll: h.redactAll}
}

func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{
		next:      h.next.WithGroup(name),
		redactor:  h.redactor,
		redactAll: h.redactAll || sensitiveKey(name),
	}
}

func (h redactingHandler) cleanAttr(attr slog.Attr) slog.Attr {
	if h.redactAll && !attr.Equal(slog.Attr{}) {
		return slog.String(attr.Key, redacted)
	}
	return h.redactor.attr(attr)
}

type redactor struct {
	secrets []string
}

func newRedactor(values []string) redactor {
	seen := make(map[string]struct{}, len(values))
	secrets := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		secrets = append(secrets, value)
	}
	// Replace longer values first so overlapping secrets cannot expose a suffix.
	sort.Slice(secrets, func(left, right int) bool {
		return len(secrets[left]) > len(secrets[right])
	})
	return redactor{secrets: secrets}
}

func (r redactor) attr(attr slog.Attr) slog.Attr {
	if attr.Equal(slog.Attr{}) {
		return attr
	}
	if sensitiveKey(attr.Key) {
		return slog.String(attr.Key, redacted)
	}

	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		members := value.Group()
		clean := make([]slog.Attr, len(members))
		for index, member := range members {
			clean[index] = r.attr(member)
		}
		return slog.Group(attr.Key, attrsToAny(clean)...)
	}
	if value.Kind() == slog.KindString {
		return slog.String(attr.Key, r.string(value.String()))
	}
	if value.Kind() == slog.KindAny {
		if err, ok := value.Any().(error); ok {
			return slog.String(attr.Key, r.string(err.Error()))
		}
		// Arbitrary objects can hide sensitive nested fields that slog would
		// otherwise reflect without passing their keys through this policy.
		return slog.String(attr.Key, redacted)
	}
	return slog.Attr{Key: attr.Key, Value: value}
}

func (r redactor) string(value string) string {
	for _, secret := range r.secrets {
		value = strings.ReplaceAll(value, secret, redacted)
	}
	return value
}

func attrsToAny(attrs []slog.Attr) []any {
	values := make([]any, len(attrs))
	for index, attr := range attrs {
		values[index] = attr
	}
	return values
}

func sensitiveKey(key string) bool {
	normalized := strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToLower(character)
		}
		return -1
	}, key)

	for _, fragment := range []string{
		"authorization",
		"cookie",
		"credential",
		"password",
		"secret",
		"token",
	} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}

	switch normalized {
	case "args", "argv", "body", "claims", "commandline", "form", "formbody", "formvalues", "groups", "groupclaim", "headers", "markdown", "notebody", "query", "queryparams", "requestbody", "responsebody":
		return true
	default:
		return false
	}
}
