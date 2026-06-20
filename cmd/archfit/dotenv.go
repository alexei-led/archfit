package main

import (
	"bufio"
	"os"
	"strings"
)

// loadDotEnv reads simple KEY=VALUE lines from path and sets each in the process
// environment ONLY when the key is currently unset — real environment variables
// and CI secrets always win. Best-effort: a missing file or malformed line is
// silently ignored. Supports `export KEY=VALUE`, `#` comments, blank lines, and
// optional surrounding single/double quotes on the value.
func loadDotEnv(path string) {
	f, err := os.Open(path) //#nosec G304 -- operator-controlled local .env
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
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
		if os.Getenv(key) != "" {
			continue // real env / CI secret wins
		}
		_ = os.Setenv(key, unquoteEnvValue(strings.TrimSpace(val)))
	}
}

// unquoteEnvValue strips one layer of matching single or double quotes.
func unquoteEnvValue(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}
