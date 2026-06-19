package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotEnv(t *testing.T) {
	input := `
# a comment line
DATABASE_URL=postgresql://localhost/db
export PORT=8080
QUOTED="hello world"
SINGLE='abc'
EMPTY=
no_equals_here
  SPACED  =  trimmed
`
	got := parseDotEnv(strings.NewReader(input))
	want := map[string]string{
		"DATABASE_URL": "postgresql://localhost/db",
		"PORT":         "8080",
		"QUOTED":       "hello world",
		"SINGLE":       "abc",
		"EMPTY":        "",
		"SPACED":       "trimmed",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d keys %v, want %d", len(got), got, len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("key %q: got %q, want %q", k, got[k], v)
		}
	}
}

func TestLoadDotEnvDoesNotOverrideExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=fromfile\nBAR=barfile\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FOO", "fromenv")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("FOO"); got != "fromenv" {
		t.Errorf("FOO was overridden: got %q, want %q", got, "fromenv")
	}
	if got := os.Getenv("BAR"); got != "barfile" {
		t.Errorf("BAR not set from file: got %q", got)
	}
}

func TestLoadDotEnvMissingFileIsNotError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.env")
	if err := loadDotEnv(missing); err != nil {
		t.Errorf("missing file should not error, got: %v", err)
	}
}
