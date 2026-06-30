package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// envSearchDirs extracts directories from CLI args whose .env should be autoloaded
// in addition to CWD: the analyzed repo (--root/-r) and the config's directory
// (--config/-c). Both "--flag=val" and "--flag val" forms are handled. This lets a
// key kept alongside the target repo or config be picked up without --env-file;
// CWD and real env still win because loadDotEnv never overrides an already-set var.
func envSearchDirs(args []string) []string {
	var dirs []string
	flagVal := func(i *int, long, short string) (string, bool) {
		a := args[*i]
		if a == long || (short != "" && a == short) {
			if *i+1 < len(args) {
				*i++
				return args[*i], true
			}
			return "", false
		}
		if v, ok := strings.CutPrefix(a, long+"="); ok {
			return v, true
		}
		if short != "" {
			if v, ok := strings.CutPrefix(a, short+"="); ok {
				return v, true
			}
		}
		return "", false
	}
	for i := 0; i < len(args); i++ {
		if v, ok := flagVal(&i, "--root", "-r"); ok && v != "" {
			dirs = append(dirs, v)
			continue
		}
		if v, ok := flagVal(&i, "--config", "-c"); ok && v != "" {
			dirs = append(dirs, filepath.Dir(v))
		}
	}
	return dirs
}

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
		if _, set := os.LookupEnv(key); set {
			continue // real env / CI secret wins (even when set to "")
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
