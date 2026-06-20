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

