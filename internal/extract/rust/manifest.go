package rust

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// depKindNormal is the kind of a normal dependency, which cargo metadata
// reports without one. Kinds select the Cargo.toml table a dependency is
// declared in.
const depKindNormal = ""

// dependencyTables maps each Cargo.toml dependency table name, in both the
// current and the legacy underscore spelling, to the dependency kind it holds.
var dependencyTables = map[string]string{
	"dependencies":       depKindNormal,
	"dev-dependencies":   depKindDev,
	"dev_dependencies":   depKindDev,
	"build-dependencies": depKindBuild,
	"build_dependencies": depKindBuild,
}

// depLineKey names one dependency declaration: its table kind and its TOML key
// (the rename when the dependency is renamed, else the package name).
type depLineKey struct{ kind, key string }

// relToRoot returns abs as a slash path relative to rootAbs, or false when abs
// lies outside the analysed root. "." becomes "".
func relToRoot(rootAbs, abs string) (string, bool) {
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	if rel == "." {
		return "", true
	}
	return filepath.ToSlash(rel), true
}

// canonicalRoot resolves root the way cargo spells manifest paths: symlinks and
// case variants resolved (macOS /tmp vs /private/tmp), falling back to the
// absolute path when root does not exist.
func canonicalRoot(root string) (string, bool) {
	if abs, err := filepath.EvalSymlinks(root); err == nil {
		return abs, true
	}
	abs, err := filepath.Abs(root)
	return abs, err == nil
}

// readDependencyLines reads a member manifest and indexes the line that
// declares each dependency. A manifest that cannot be read yields no lines.
func readDependencyLines(manifestPath string) map[depLineKey]int {
	data, err := os.ReadFile(manifestPath) //nolint:gosec // cargo-reported workspace member manifest
	if err != nil {
		return nil
	}
	return dependencyLines(data)
}

// dependencyLines indexes the 1-based line that first declares each dependency
// of a Cargo.toml, per table kind. It reads the forms Cargo.toml uses for
// dependencies: `key = …` and dotted `key.workspace = true` lines in a
// [dependencies]-style table (top level or under [target.<cfg>]), quoted keys,
// and `[dependencies.key]` sub-table headers. [workspace.dependencies] is
// inheritance, not a member dependency, and is skipped.
//
// It is a line scanner, not a TOML parser. Continuation lines of a multi-line
// inline table or array are skipped by bracket depth, and a manifest holding a
// multi-line string is not indexed at all: a dependency it cannot place keeps
// line 0 rather than a guessed line.
func dependencyLines(data []byte) map[depLineKey]int {
	if bytes.Contains(data, []byte(`"""`)) || bytes.Contains(data, []byte(`'''`)) {
		return nil
	}
	out := make(map[depLineKey]int)
	record := func(kind, key string, line int) {
		k := depLineKey{kind: kind, key: key}
		if _, seen := out[k]; !seen {
			out[k] = line
		}
	}
	section, inDeps := "", false
	depth := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		text := stripComment(scanner.Text())
		if depth > 0 {
			depth = max(depth+bracketDelta(text), 0)
			continue
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inDeps = false
			if strings.HasPrefix(trimmed, "[[") {
				continue // array of tables: never a dependency table
			}
			kind, key, ok := dependencyHeader(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			switch {
			case ok && key != "":
				record(kind, key, line)
			case ok:
				section, inDeps = kind, true
			}
			continue
		}
		eq := indexOutsideQuotes(trimmed, '=')
		if eq < 0 {
			continue
		}
		if inDeps {
			if segs := splitKey(trimmed[:eq]); len(segs) > 0 {
				record(section, segs[0], line)
			}
		}
		depth = max(bracketDelta(trimmed[eq+1:]), 0)
	}
	return out
}

// dependencyHeader classifies a table header. ok reports a dependency table of
// the returned kind; key is set when the header names one dependency
// ([dependencies.key]). Accepted prefixes: none, or target.<cfg>.
func dependencyHeader(header string) (kind, key string, ok bool) {
	segs := splitKey(header)
	start := 0
	if len(segs) >= 3 && segs[0] == "target" {
		start = 2
	}
	if start >= len(segs) {
		return "", "", false
	}
	kind, ok = dependencyTables[segs[start]]
	if !ok {
		return "", "", false
	}
	switch len(segs) - start {
	case 1:
		return kind, "", true
	case 2:
		return kind, segs[start+1], true
	default:
		return "", "", false
	}
}

// splitKey splits a TOML dotted key into unquoted segments, honoring quoted
// segments ('cfg(unix)', "my.crate"). It returns nil for a malformed key.
func splitKey(key string) []string {
	var segs []string
	var cur strings.Builder
	quote := byte(0)
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '.':
			segs = append(segs, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if quote != 0 {
		return nil
	}
	segs = append(segs, strings.TrimSpace(cur.String()))
	for _, s := range segs {
		if s == "" {
			return nil
		}
	}
	return segs
}

// stripComment drops a trailing # comment that is not inside a string.
func stripComment(line string) string {
	if i := indexOutsideQuotes(line, '#'); i >= 0 {
		return line[:i]
	}
	return line
}

// indexOutsideQuotes returns the index of the first c outside a quoted string,
// or -1.
func indexOutsideQuotes(s string, c byte) int {
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		switch ch := s[i]; {
		case quote != 0:
			if ch == '\\' && quote == '"' {
				i++
			} else if ch == quote {
				quote = 0
			}
		case ch == '"' || ch == '\'':
			quote = ch
		case ch == c:
			return i
		}
	}
	return -1
}

// bracketDelta returns the net count of opening minus closing braces and
// brackets outside quoted strings: how many inline tables or arrays a line
// leaves open.
func bracketDelta(s string) int {
	delta := 0
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		switch ch := s[i]; {
		case quote != 0:
			if ch == '\\' && quote == '"' {
				i++
			} else if ch == quote {
				quote = 0
			}
		case ch == '"' || ch == '\'':
			quote = ch
		case ch == '{' || ch == '[':
			delta++
		case ch == '}' || ch == ']':
			delta--
		}
	}
	return delta
}
