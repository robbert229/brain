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
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := strings.TrimSpace(out.String()); got != "[stub] tn interactive mode" {
		t.Fatalf("unexpected tn output: %q", got)
	}
}

func TestRun_TNCreateFromNaturalLanguage(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"tn", "Review PR #123 tomorrow high priority @work"}, &out, &errOut)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := strings.TrimSpace(out.String()); got != "[stub] tn create: Review PR #123 tomorrow high priority @work" {
		t.Fatalf("unexpected tn create output: %q", got)
	}
}

func TestRun_TNListFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"tn", "list", "--today", "--overdue", "--completed", "--filter", "priority:urgent AND tags:work", "--json", "--limit", "10"}, &out, &errOut)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	got := strings.TrimSpace(out.String())
	if !strings.Contains(got, "[stub] tn list") {
		t.Fatalf("unexpected tn list output: %q", got)
	}
	if !strings.Contains(got, `today=true overdue=true completed=true`) {
		t.Fatalf("expected list state flags in output: %q", got)
	}
	if !strings.Contains(got, `filter="priority:urgent AND tags:work"`) {
		t.Fatalf("expected list filter in output: %q", got)
	}
	if !strings.Contains(got, `json=true`) {
		t.Fatalf("expected json flag in output: %q", got)
	}
	if !strings.Contains(got, `limit=10`) {
		t.Fatalf("expected limit flag in output: %q", got)
	}
}

func TestRun_TNListDefaultLimit(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"tn", "list"}, &out, &errOut)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	got := strings.TrimSpace(out.String())
	if !strings.Contains(got, `limit=20`) {
		t.Fatalf("expected default limit in output: %q", got)
	}
}

func TestRun_TNUpdateFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"tn", "update", "abc123", "--status", "completed", "--priority", "high", "--due", "2025-08-20", "--add-tags", "urgent,bug", "--remove-tags", "low-priority", "--add-contexts", "office", "--add-projects", "Website"}, &out, &errOut)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	got := strings.TrimSpace(out.String())
	if !strings.Contains(got, "[stub] tn update abc123") {
		t.Fatalf("unexpected tn update output: %q", got)
	}
	for _, expected := range []string{
		`status="completed"`,
		`priority="high"`,
		`due="2025-08-20"`,
		`add-tags="urgent,bug"`,
		`remove-tags="low-priority"`,
		`add-contexts="office"`,
		`add-projects="Website"`,
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing expected update flag %q in output: %q", expected, got)
		}
	}
}

func TestRun_TNSearch(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exitCode := run([]string{"tn", "search", "groceries"}, &out, &errOut)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if got := strings.TrimSpace(out.String()); got != `[stub] tn search "groceries"` {
		t.Fatalf("unexpected tn search output: %q", got)
	}
}

