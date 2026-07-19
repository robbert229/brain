package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_DefaultGreeting(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run(nil, &out, &errOut)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := out.String(); got != "hello, world\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRun_WithName(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"--name", "john"}, &out, &errOut)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := out.String(); got != "hello, john\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRun_Version(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"--version"}, &out, &errOut)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := strings.TrimSpace(out.String()); got != "brain "+version {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestRun_InvalidFlagReturnsTwo(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"--not-a-real-flag"}, &out, &errOut)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}
}

func TestRun_TNSubcommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"tn"}, &out, &errOut)
	requireNotImplemented(t, exitCode, out.String(), errOut.String())
}

func TestRun_TNCreateFromNaturalLanguage(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	// Create is now implemented and should succeed
	exitCode := run([]string{"tn", "Review PR #123 tomorrow high priority @work"}, &out, &errOut)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. stderr: %s", exitCode, errOut.String())
	}

	// Verify success message
	if !strings.Contains(out.String(), "Task created successfully") {
		t.Fatalf("expected success message in output, got: %q", out.String())
	}
}

func TestRun_TNListFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"tn", "list", "--today", "--overdue", "--completed", "--filter", "priority:urgent AND tags:work", "--limit", "10"}, &out, &errOut)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	got := strings.TrimSpace(out.String())
	if !strings.Contains(got, "Found 0 tasks") {
		t.Fatalf("unexpected tn list output: %q", got)
	}
	if !strings.Contains(got, "No tasks found matching your criteria") {
		t.Fatalf("expected empty list output: %q", got)
	}
}

//func TestRun_TNListDefaultLimit(t *testing.T) {
//	var out bytes.Buffer
//	var errOut bytes.Buffer
//
//	exitCode := run([]string{"tn", "list"}, &out, &errOut)
//	if exitCode != 0 {
//		t.Fatalf("expected exit code 0, got %d", exitCode)
//	}
//
//	got := strings.TrimSpace(out.String())
//	if !strings.Contains(got, "Found 0 tasks") {
//		t.Fatalf("unexpected tn list output: %q", got)
//	}
//}

func TestRun_TNUpdateFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"tn", "update", "abc123", "--status", "completed", "--priority", "high", "--due", "2025-08-20", "--add-tags", "urgent,bug", "--remove-tags", "low-priority", "--add-contexts", "office", "--add-projects", "Website"}, &out, &errOut)
	requireNotImplemented(t, exitCode, out.String(), errOut.String())
}

func TestRun_TNSearch(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"tn", "search", "groceries"}, &out, &errOut)
	requireNotImplemented(t, exitCode, out.String(), errOut.String())
}

func requireNotImplemented(t *testing.T, exitCode int, stdout string, stderr string) {
	t.Helper()

	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}
	if stdout != "" {
		t.Fatalf("expected no stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "not implemented") {
		t.Fatalf("expected not implemented error, got %q", stderr)
	}
}
