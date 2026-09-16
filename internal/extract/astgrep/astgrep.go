// Package astgrep implements the evidenceports.PatternProvider port for ast-grep.
// It shells out to the "sg" binary and parses its JSON output.
// If "sg" is absent and mode is ModeAuto, Find returns empty matches with
// coverage status "absent" — it never returns an error for a missing tool.
package astgrep

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// toolName is the coverage/name identifier for this adapter.
const toolName = "ast-grep"

// Adapter satisfies evidenceports.PatternProvider using the "sg" (ast-grep) binary.
type Adapter struct {
	runner toolrun.Runner
	// Cache is the extractor fact cache; nil disables caching (--no-cache).
	Cache *factcache.Store
	// versionOnce/versionValue memoize the `sg --version` probe; see sgVersion.
	versionOnce  sync.Once
	versionValue string
}

// New returns an Adapter configured with the given runner.
func New(runner toolrun.Runner) *Adapter {
	return &Adapter{runner: runner}
}

// PatternCoverageTool returns the coverage name for configured pattern rules.
func (a *Adapter) PatternCoverageTool() string { return toolName }

// SyntaxCoverageTool returns the coverage name for embedded syntax rules.
func (a *Adapter) SyntaxCoverageTool() string { return syntaxToolName }

// cachedRunner wraps the runner in a fact-cache decorator for the sg
// invocations of one Find/Syntax call (fact-cache.md D5 seam 1). Returns the
// plain runner when the cache is off or key material cannot be derived —
// never fails the run. The rules/patterns ride each command's argv (folded
// into the entry address), so the Key carries only the sg version, the scan
// root, and the whole-tree content hash — ast-grep scans every language, so
// its input scope is the full tree. Timed-out runs are exec-level errors and
// are never recorded; a non-zero sg exit is not cached either (it means a
// rejected rule file, which the caller reports as partial).
func (a *Adapter) cachedRunner(ctx context.Context, root string) toolrun.Runner {
	if a.Cache == nil {
		return a.runner
	}
	cfgHash, err := factcache.HashJSON(struct{ Root string }{root})
	if err != nil {
		return a.runner
	}
	files := factcache.ListInputs(root, factcache.MatchAll, nil)
	treeHash, err := factcache.HashTree(root, files)
	if err != nil {
		return a.runner
	}
	return &factcache.Runner{
		Inner:     a.runner,
		Store:     a.Cache,
		Analyzer:  "astgrep",
		Key:       factcache.Key("astgrep", a.sgVersion(ctx), cfgHash, treeHash),
		Cacheable: func(out toolrun.Output) bool { return out.ExitCode == 0 },
	}
}

// sgVersion probes `sg --version`. Best-effort: "" on any failure.
//
// Memoized: the same value keys the fact cache AND stamps both Coverage rows
// (patterns and syntax), so an un-memoized probe would shell out up to three
// times per run — and would not run at all when the cache is off, leaving the
// coverage row's version empty for a reason that has nothing to do with sg.
func (a *Adapter) sgVersion(ctx context.Context) string {
	a.versionOnce.Do(func() {
		out, err := a.runner.Run(ctx, toolrun.ToolCmd{
			Name:    "sg",
			Args:    []string{"--version"},
			Timeout: 30 * time.Second,
		})
		if err != nil || out.ExitCode != 0 {
			return
		}
		a.versionValue = lastNonEmptyLine(string(out.Stdout))
	})
	return a.versionValue
}

// lastNonEmptyLine returns the final non-blank line of s, trimmed.
//
// `sg --version` prints a multi-line banner telling the caller to use the
// `ast-grep` name instead, ABOVE the version, so the whole stdout is not a
// version: it would put ~200 bytes of prose into the fact-cache key and, since
// the value now also stamps Coverage.Version, into the published report. The
// version is the last line in both the banner and the bare form.
func lastNonEmptyLine(s string) string {
	for _, line := range slices.Backward(strings.Split(s, "\n")) {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
func (a *Adapter) Find(ctx context.Context, s scope.Scope, c pattern.Config) ([]pattern.Match, evidence.Coverage, error) {
	_, ok := a.runner.Detect(ctx, "sg")
	if !ok {
		return nil, evidence.Coverage{Tool: toolName, Status: "absent"}, nil
	}

	seen := make(map[dedupeKey]struct{})
	var matches []pattern.Match
	fileSet := make(map[string]struct{})

	runner := a.cachedRunner(ctx, s.Root)
	for _, def := range c {
		out, err := runner.Run(ctx, toolrun.ToolCmd{
			Name:    "sg",
			Args:    []string{"--lang", def.Lang, "--json", "run", "--pattern", def.Rule, "."},
			WorkDir: s.Root,
		})
		if err != nil {
			return nil, evidence.Coverage{}, fmt.Errorf("astgrep: run sg for pattern %q: %w", def.ID, err)
		}
		if len(out.Stdout) == 0 {
			continue
		}

		var raw []sgMatch
		if err := json.Unmarshal(out.Stdout, &raw); err != nil {
			return nil, evidence.Coverage{}, fmt.Errorf("astgrep: parse sg output for pattern %q: %w", def.ID, err)
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

	cov := evidence.Coverage{
		Tool:      toolName,
		Version:   a.sgVersion(ctx),
		FilesSeen: len(fileSet),
		Status:    "ok",
	}
	return matches, cov, nil
}

// Compile-time interface check.
var _ evidenceports.PatternProvider = (*Adapter)(nil)
