package main

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// loadDotEnv reads a .env file and sets any variable that is not already
// present in the process environment. Variables already set in the real
// environment always win — this matches godotenv's default and keeps
// docker-compose and shell exports authoritative. A missing file is not an
// error: in Docker the variables come from Compose and no .env exists.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	for key, val := range parseDotEnv(f) {
		if _, ok := os.LookupEnv(key); !ok {
			os.Setenv(key, val)
		}
	}
	return nil
}

// parseDotEnv reads KEY=VALUE pairs from r. It skips blank lines and comments,
// tolerates an "export " prefix, and strips a single layer of matching single
// or double quotes around the value. Lines without "=" are ignored.
func parseDotEnv(r io.Reader) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = unquote(strings.TrimSpace(val))
	}
	return out
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
