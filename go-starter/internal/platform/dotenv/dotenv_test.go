package dotenv

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/codingseeker/go-starter/internal/config"
)

func TestParse(t *testing.T) {
	content := `# a comment
APP_NAME=demo

  export APP_ENV=testing
LOG_LEVEL="warn"
QUOTED='single quoted value'
EMPTY=
SPACED =  trimmed value
WITH_EQUALS=a=b=c
`

	values, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := Values{
		"APP_NAME":    "demo",
		"APP_ENV":     "testing",
		"LOG_LEVEL":   "warn",
		"QUOTED":      "single quoted value",
		"EMPTY":       "",
		"SPACED":      "trimmed value",
		"WITH_EQUALS": "a=b=c",
	}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("Parse() = %#v, want %#v", values, want)
	}
}

func TestParseKeysAreSorted(t *testing.T) {
	values, err := Parse(strings.NewReader("B=2\nA=1\nC=3\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got, want := strings.Join(values.Keys(), ","), "A,B,C"; got != want {
		t.Errorf("Keys() = %q, want %q", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantMsg string
	}{
		{"missing_separator", "APP_NAME\n", "expected KEY=VALUE"},
		{"invalid_key", "APP-NAME=demo\n", "invalid key"},
		{"empty_key", "=demo\n", "invalid key"},
		{"leading_digit", "1APP=demo\n", "invalid key"},
		{"unbalanced_quotes", "APP_NAME=\"demo\n", "unbalanced quotes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.content))
			if err == nil {
				t.Fatalf("Parse(%q) error = nil, want an error", tt.content)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("Parse(%q) error = %v, want it to mention %q", tt.content, err, tt.wantMsg)
			}
		})
	}
}

func TestLoadIntoMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.env")

	loaded, err := LoadInto(path, os.LookupEnv, func(string, string) error { return nil })
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}
	if loaded {
		t.Error("LoadInto() reported a missing file as loaded")
	}
}

func TestLoadIntoDoesNotOverrideExistingVariables(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "APP_NAME=from-file\nLOG_LEVEL=debug\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	existing := map[string]string{config.EnvAppName: "from-environment"}
	applied := map[string]string{}

	loaded, err := LoadInto(path,
		func(key string) (string, bool) {
			value, ok := existing[key]
			return value, ok
		},
		func(key, value string) error {
			applied[key] = value
			return nil
		},
	)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}
	if !loaded {
		t.Fatal("LoadInto() loaded = false, want true")
	}

	want := map[string]string{"LOG_LEVEL": "debug"}
	if !reflect.DeepEqual(applied, want) {
		t.Errorf("applied = %#v, want %#v (environment must win)", applied, want)
	}
}

func TestLoadIntoPropagatesSetenvError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("APP_NAME=demo\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadInto(path,
		func(string) (string, bool) { return "", false },
		func(string, string) error { return os.ErrPermission },
	)
	if err == nil {
		t.Fatal("LoadInto() error = nil, want the setenv failure")
	}
	if !strings.Contains(err.Error(), "APP_NAME") {
		t.Errorf("LoadInto() error = %v, want it to mention the failing key", err)
	}
}

func TestLoadReadsRealEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	t.Setenv("DOTENV_TEST_PRESET", "existing")
	t.Setenv("DOTENV_TEST_FROM_FILE", "")
	if err := os.Unsetenv("DOTENV_TEST_FROM_FILE"); err != nil {
		t.Fatalf("unsetenv: %v", err)
	}
	if err := os.WriteFile(path, []byte("DOTENV_TEST_PRESET=file\nDOTENV_TEST_FROM_FILE=file\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !loaded {
		t.Fatal("Load() loaded = false, want true")
	}
	if got := os.Getenv("DOTENV_TEST_PRESET"); got != "existing" {
		t.Errorf("DOTENV_TEST_PRESET = %q, want %q (must not be overwritten)", got, "existing")
	}
	if got := os.Getenv("DOTENV_TEST_FROM_FILE"); got != "file" {
		t.Errorf("DOTENV_TEST_FROM_FILE = %q, want %q", got, "file")
	}
}

func TestLoadRejectsMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("not a pair\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want a parse error")
	}
}
