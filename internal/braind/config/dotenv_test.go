package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDotEnvMissingFileReturnsBase(t *testing.T) {
	base := mapLookup(map[string]string{"KEEP": "base"})
	lookup, err := LoadDotEnv(base, filepath.Join(t.TempDir(), "does-not-exist.env"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if got, ok := lookup("KEEP"); !ok || got != "base" {
		t.Fatalf("expected base value to remain, got %q ok=%v", got, ok)
	}
	if _, ok := lookup("OTHER"); ok {
		t.Fatal("unexpected variable resolved")
	}
}

func TestLoadDotEnvParsesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := []byte(strings.Join([]string{
		"# a comment",
		"",
		"PLAIN=hello",
		"export EXPORTED=world",
		`SINGLE='single quoted'`,
		`DOUBLE="double quoted"`,
		"DUP=first",
		"DUP=second",
		"SPACED =  padded value  ",
	}, "\n"))
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	lookup, err := LoadDotEnv(mapLookup(map[string]string{}), path)
	if err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	want := map[string]string{
		"PLAIN":    "hello",
		"EXPORTED": "world",
		"SINGLE":   "single quoted",
		"DOUBLE":   "double quoted",
		"DUP":      "second", // last occurrence wins
		"SPACED":   "padded value",
	}
	for key, expected := range want {
		if got, ok := lookup(key); !ok || got != expected {
			t.Errorf("key %q: expected %q, got %q ok=%v", key, expected, got, ok)
		}
	}
}

func TestLoadDotEnvBaseTakesPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("SHARED=file\nFROM_FILE=yes\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	lookup, err := LoadDotEnv(mapLookup(map[string]string{"SHARED": "base"}), path)
	if err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got, ok := lookup("SHARED"); !ok || got != "base" {
		t.Errorf("expected process environment to win over file, got %q ok=%v", got, ok)
	}
	if got, ok := lookup("FROM_FILE"); !ok || got != "yes" {
		t.Errorf("expected file value to fill unset variable, got %q ok=%v", got, ok)
	}
}

func TestLoadDotEnvInvalidLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("VALID=ok\nthis line has no equals\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	if _, err := LoadDotEnv(mapLookup(nil), path); err == nil {
		t.Fatal("expected an error for a malformed line")
	} else if !strings.Contains(err.Error(), ".env:2") {
		t.Fatalf("expected error to identify the line, got %v", err)
	}
}

func TestLoadDotEnvInvalidName(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("1NOT_A_NAME=value\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	if _, err := LoadDotEnv(mapLookup(nil), path); err == nil {
		t.Fatal("expected an error for an invalid variable name")
	}
}

func TestLoadDotEnvUnreadableFile(t *testing.T) {
	// A directory opens successfully but fails when read.
	dir := t.TempDir()
	if _, err := LoadDotEnv(mapLookup(nil), dir); err == nil {
		t.Fatal("expected an error when the file cannot be read")
	}
}
