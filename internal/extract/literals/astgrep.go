package literals

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const astGrep = "ast-grep"

// batchSize is the number of paths per ast-grep call; it keeps the command
// line short on large trees.
const batchSize = 500

// sgNoMatch is the exit code ast-grep gives a scan that matched nothing.
// Any code other than it and 0, including -1 for a process a signal ended,
// is a failed run whose output may be cut short.
const sgNoMatch = 1

// The rule ids of rules/literals.yml: each names how a value is read.
const (
	ruleJSQuoted   = "js-quoted"   // '…', "…" or `…`
	rulePyString   = "py-string"   // optional prefix, then '…', "…", '''…''' or """…"""
	ruleRsString   = "rs-string"   // "…", or raw r"…" with any number of #
	ruleCSQuoted   = "cs-quoted"   // "…"
	ruleCSVerbatim = "cs-verbatim" // @"…"
)

//go:embed rules/literals.yml
var rules string

// noIgnore makes ast-grep read every file it is given, whatever ignore files say.
var noIgnore = func() []string {
	kinds := []string{"hidden", "dot", "exclude", "global", "parent", "vcs"}
	args := make([]string, 0, 2*len(kinds))
	for _, kind := range kinds {
		args = append(args, "--no-ignore", kind)
	}
	return args
}()

// sgMatch is the part of an ast-grep --json=stream match the reader uses.
// Lines are 0-based.
type sgMatch struct {
	Text   string `json:"text"`
	File   string `json:"file"`
	RuleID string `json:"ruleId"`
	Range  struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
		End struct {
			Line int `json:"line"`
		} `json:"end"`
	} `json:"range"`
}

// readAstGrep returns the single-line literals of files, root-relative.
func readAstGrep(ctx context.Context, runner toolrun.Runner, root string, files []string) ([]Literal, error) {
	var lits []Literal
	for batch := range slices.Chunk(files, batchSize) {
		args := slices.Concat([]string{"scan", "--inline-rules", rules, "--json=stream"}, noIgnore, []string{"--"}, batch)
		out, err := runner.Run(ctx, toolrun.ToolCmd{Name: astGrep, Args: args, WorkDir: root})
		if err != nil {
			return nil, fmt.Errorf("literals: run ast-grep: %w", err)
		}
		if out.ExitCode != 0 && out.ExitCode != sgNoMatch {
			return nil, fmt.Errorf("literals: ast-grep exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
		}
		for line := range bytes.SplitSeq(out.Stdout, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var m sgMatch
			if err := json.Unmarshal(line, &m); err != nil {
				return nil, fmt.Errorf("literals: read ast-grep output: %w", err)
			}
			if m.Range.Start.Line != m.Range.End.Line {
				continue // multi-line literals are out of milestone 1
			}
			file := path.Clean(m.File)
			v, ok, err := value(m.RuleID, m.Text)
			if err != nil {
				return nil, fmt.Errorf("literals: %s:%d: %w", file, m.Range.Start.Line+1, err)
			}
			if ok {
				lits = append(lits, Literal{File: file, Line: m.Range.Start.Line + 1, Value: v})
			}
		}
	}
	return lits, nil
}

// value returns the text between the delimiters of a matched string. ok is
// false for a string that is not plain text: a Python f-, t- or b-string, a
// Rust byte or C string, a C# "…"u8 string. An error is a match the rule id
// cannot read.
func value(rule, text string) (string, bool, error) {
	bad := fmt.Errorf("rule %s matched %q, which is not a string it reads", rule, text)
	switch rule {
	case ruleJSQuoted:
		if len(text) < 2 || !strings.ContainsRune(`'"`+"`", rune(text[0])) || text[len(text)-1] != text[0] {
			return "", false, bad
		}
		return text[1 : len(text)-1], true, nil
	case rulePyString:
		q := strings.IndexAny(text, `'"`)
		if q < 0 {
			return "", false, bad
		}
		if strings.ContainsAny(strings.ToLower(text[:q]), "fbt") {
			return "", false, nil
		}
		quote := text[q : q+1]
		if strings.HasPrefix(text[q:], strings.Repeat(quote, 3)) && len(text)-q >= 6 {
			quote = strings.Repeat(quote, 3)
		}
		body := text[q+len(quote):]
		if !strings.HasSuffix(body, quote) {
			return "", false, bad
		}
		return strings.TrimSuffix(body, quote), true, nil
	case ruleRsString:
		q := strings.IndexByte(text, '"')
		if q < 0 {
			return "", false, bad
		}
		prefix := text[:q]
		hashes := strings.TrimPrefix(prefix, "r")
		if prefix != "" && (hashes == prefix || strings.Trim(hashes, "#") != "") {
			return "", false, nil // b"…", br"…", c"…", cr"…"
		}
		end := `"` + hashes
		if len(text)-q < 1+len(end) || !strings.HasSuffix(text, end) {
			return "", false, bad
		}
		return text[q+1 : len(text)-len(end)], true, nil
	case ruleCSQuoted, ruleCSVerbatim:
		start := `"`
		if rule == ruleCSVerbatim {
			start = `@"`
		}
		if !strings.HasPrefix(text, start) {
			return "", false, bad
		}
		if len(text) < len(start)+1 || !strings.HasSuffix(text, `"`) {
			return "", false, nil // "…"u8 is UTF-8 bytes, not a string
		}
		return text[len(start) : len(text)-1], true, nil
	}
	return "", false, fmt.Errorf("unknown rule %q", rule)
}
