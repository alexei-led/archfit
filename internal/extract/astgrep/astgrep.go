// Package astgrep implements the ports.PatternProvider port for ast-grep.
// It shells out to the "sg" binary and parses its JSON output.
// If "sg" is absent and mode is ModeAuto, Find returns empty matches with
// coverage status "absent" — it never returns an error for a missing tool.
package astgrep

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// toolName is the coverage/name identifier for this adapter.
const toolName = "ast-grep"

// Adapter satisfies ports.PatternProvider using the "sg" (ast-grep) binary.
type Adapter struct {
	runner toolrun.Runner
}

// New returns an Adapter configured with the given runner.
func New(runner toolrun.Runner) *Adapter {
	return &Adapter{runner: runner}
}

// Name returns the tool identifier.
func (a *Adapter) Name() string { return toolName }

// sgMatch is the JSON shape ast-grep emits for each match.
type sgMatch struct {
	Text  string `json:"text"`
	Range struct {
		Start struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"start"`
	} `json:"range"`
	File string `json:"file"`
	Rule struct {
		ID string `json:"id"`
	} `json:"rule"`
}

// dedupeKey is used to eliminate duplicate matches.
type dedupeKey struct {
	file    string
	line    int
	pattern string
}

// Find runs all patterns against the given scope and returns deduplicated,
// sorted matches plus a Coverage record. A missing "sg" binary returns empty
// matches with status "absent" — never an error.
func (a *Adapter) Find(ctx context.Context, s scope.Scope, c config.PatternConfig) ([]pattern.Match, diagnostic.Coverage, error) {
	_, ok := a.runner.Detect(ctx, "sg")
	if !ok {
		return nil, diagnostic.Coverage{Tool: toolName, Status: "absent"}, nil
	}

	seen := make(map[dedupeKey]struct{})
	var matches []pattern.Match
	fileSet := make(map[string]struct{})

	for _, def := range c {
		out, err := a.runner.Run(ctx, toolrun.ToolCmd{
			Name:    "sg",
			Args:    []string{"--lang", def.Lang, "--json", "run", "--pattern", def.Rule, "."},
			WorkDir: s.Root,
		})
		if err != nil {
			return nil, diagnostic.Coverage{}, fmt.Errorf("astgrep: run sg for pattern %q: %w", def.ID, err)
		}
		if len(out.Stdout) == 0 {
			continue
		}

		var raw []sgMatch
		if err := json.Unmarshal(out.Stdout, &raw); err != nil {
			return nil, diagnostic.Coverage{}, fmt.Errorf("astgrep: parse sg output for pattern %q: %w", def.ID, err)
		}

		for _, m := range raw {
			// ast-grep reports 0-based lines; normalize to 1-based.
			line := m.Range.Start.Line + 1
			k := dedupeKey{file: m.File, line: line, pattern: def.ID}
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			fileSet[m.File] = struct{}{}
			matches = append(matches, pattern.Match{
				File:    m.File,
				Pattern: def.ID,
				Text:    m.Text,
				Node:    m.Rule.ID,
				Line:    line,
				Column:  m.Range.Start.Column,
			})
		}
	}

	// Sort by (file, line) for deterministic output.
	slices.SortFunc(matches, func(a, b pattern.Match) int {
		if a.File != b.File {
			if a.File < b.File {
				return -1
			}
			return 1
		}
		return a.Line - b.Line
	})

	cov := diagnostic.Coverage{
		Tool:      "ast-grep",
		FilesSeen: len(fileSet),
		Status:    "ok",
	}
	return matches, cov, nil
}

// Compile-time interface check.
var _ ports.PatternProvider = (*Adapter)(nil)
