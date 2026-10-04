// Package dotenv provides a minimal, dependency free reader for .env files.
// It is only used to ease local development: production deployments are
// expected to provide real environment variables, so the CLI never loads a
// dotenv file when the environment resolves to production.
package dotenv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// Values holds parsed key/value pairs in file order.
type Values map[string]string

// Keys returns the parsed keys in ascending order.
func (v Values) Keys() []string {
	keys := make([]string, 0, len(v))
	for key := range v {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Parse reads dotenv content from r. Supported syntax is intentionally small:
// blank lines, comments starting with '#', an optional "export " prefix,
// KEY=VALUE pairs and single or double quoted values. No variable expansion or
// escape processing is performed.
func Parse(r io.Reader) (Values, error) {
	values := Values{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}

		raw := strings.TrimSpace(strings.TrimPrefix(text, "export "))
		key, value, found := strings.Cut(raw, "=")
		if !found {
			return nil, fmt.Errorf("dotenv: line %d: expected KEY=VALUE, got %q", line, text)
		}

		key = strings.TrimSpace(key)
		if !validKey(key) {
			return nil, fmt.Errorf("dotenv: line %d: invalid key %q", line, key)
		}

		parsed, err := parseValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("dotenv: line %d: %w", line, err)
		}
		values[key] = parsed
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("dotenv: read file: %w", err)
	}
	return values, nil
}

// LoadInto parses path and passes every value to setenv unless the key is
// already present in lookup. It reports whether the file existed. Using
// injected lookup and setenv functions keeps the precedence rules testable
// without mutating global process state.
func LoadInto(path string, lookup func(string) (string, bool), setenv func(string, string) error) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("dotenv: open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	values, err := Parse(file)
	if err != nil {
		return false, fmt.Errorf("dotenv: parse %s: %w", path, err)
	}

	for _, key := range values.Keys() {
		if _, ok := lookup(key); ok {
			continue
		}
		if err := setenv(key, values[key]); err != nil {
			return true, fmt.Errorf("dotenv: set %s: %w", key, err)
		}
	}
	return true, nil
}

// Load applies path to the process environment without overriding variables
// that are already set.
func Load(path string) (bool, error) {
	return LoadInto(path, os.LookupEnv, os.Setenv)
}

func parseValue(value string) (string, error) {
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			return value[1 : len(value)-1], nil
		}
	}
	if strings.ContainsAny(value, "\"'") {
		return "", fmt.Errorf("unbalanced quotes in value %q", value)
	}
	return value, nil
}

func validKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case (r >= '0' && r <= '9') && i > 0:
		default:
			return false
		}
	}
	return true
}
