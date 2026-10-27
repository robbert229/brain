package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// LoadDotEnv returns a LookupEnv backed by base plus the dot-env file at path.
// Variables already set in base (the process environment) take precedence over
// file values, so a shell or container never has its settings silently
// overridden. A missing file is not an error: base is returned unchanged.
//
// The file format is deliberately minimal:
//   - one KEY=VALUE per line; blank lines and lines starting with "#" are ignored
//   - an optional "export " prefix is accepted
//   - values wrapped in matching single or double quotes have the quotes removed
//   - no variable interpolation or line continuation is performed
func LoadDotEnv(base LookupEnv, path string) (LookupEnv, error) {
	values, err := readDotEnv(path)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return base, nil
	}
	return func(name string) (string, bool) {
		if value, ok := base(name); ok {
			return value, true
		}
		value, ok := values[name]
		return value, ok
	}, nil
}

func readDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export"); ok &&
			(len(rest) == 0 || rest[0] == ' ' || rest[0] == '\t') {
			line = strings.TrimSpace(rest)
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNo)
		}
		key = strings.TrimSpace(key)
		if !validEnvName(key) {
			return nil, fmt.Errorf("%s:%d: invalid variable name %q", path, lineNo, key)
		}
		value = unquote(strings.TrimSpace(value))
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return values, nil
}

// validEnvName reports whether key matches [A-Za-z_][A-Za-z0-9_]*.
func validEnvName(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// unquote removes a single matching pair of single or double quotes.
func unquote(value string) string {
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
		return value[1 : len(value)-1]
	}
	return value
}
