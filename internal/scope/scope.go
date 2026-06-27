// Package scope resolves the analysis scope: the repo root, changed files
// (delta mode), and the mode (full vs delta) for a run.
package scope

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/config"
)

// DefaultExclusions are tool-artifact, cache, and dependency directories archfit
// never analyses: measuring them yields non-deterministic or irrelevant facts —
// a vendored tree's complexity, a generated index, or a report written back into
// the scanned repo (the OpenAI study's self-scan "instability"). They are MERGED
// with — never replace — the config `exclusions` (see MergeExclusions). Kept here
// in the core ring as pure data so scope stays free of I/O. Globs use doublestar
// `**/<name>/**` form so they match the directory at the repo root and at any
// nested depth without over-matching siblings (e.g. `reports.go`).
var DefaultExclusions = []string{
	"**/.archfit-cache/**",
	"**/.archfit-baseline.json",
	"**/.gitnexus/**",  // ignore index dirs; tool removed.
	"**/.codegraph/**", // ignore index dirs; tool removed.
	"**/reports/**",
	"**/.venv/**",
	"**/node_modules/**",
	"**/vendor/**",
	"**/pkg/mod/**", // Go module download cache (GOPATH/pkg/mod): third-party deps, not first-party source.
	"**/dist/**",
	"**/build/**",
	// testdata trees hold fixture repos used by test suites; they are not
	// production architecture and would distort self-dogfooding signals
	// (false coverage gaps, phantom language detections). Re-include with
	// "!testdata" in the config exclusions when analysing fixtures
	// intentionally.
	"**/testdata/**",
}

// MergeExclusions returns the effective exclusion globs: DefaultExclusions
// supplemented by the configured ones, de-duplicated and sorted so a double-run
// stays byte-identical regardless of config order. A configured entry prefixed
// with "!" re-includes a default — it removes the matching default from the
// result and is itself dropped, so extractors never see the negation marker.
// "!reports", "!.archfit-baseline.json", and the exact "!**/reports/**" all
// remove their default. Pure: no filesystem access.
func MergeExclusions(configured []string) []string {
	set := make(map[string]struct{}, len(DefaultExclusions)+len(configured))
	for _, d := range DefaultExclusions {
		set[d] = struct{}{}
	}
	for _, c := range configured {
		if neg, ok := strings.CutPrefix(c, "!"); ok {
			// Re-include: drop the default whether the user named the bare
			// artifact (reports), the file (.archfit-baseline.json), or the exact glob.
			delete(set, neg)
			delete(set, "**/"+neg+"/**")
			delete(set, "**/"+neg)
			continue
		}
		set[c] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ScopeMode distinguishes full-repo analysis from delta (diff-based) analysis.
// The name is intentionally ScopeMode (not Mode) to match the design contract
// shared across packages; the stutter is acceptable here.
//
//nolint:revive // ScopeMode is the design-specified name; used as scope.ScopeMode across packages intentionally
type ScopeMode string

const (
	// ModeDelta analyses only files changed between Base and Head.
	ModeDelta ScopeMode = "delta"
	// ModeFull analyses the entire repository.
	ModeFull ScopeMode = "full"
)

// Scope carries the resolved analysis scope for a single run.
type Scope struct {
	// Base is the git ref used as the diff base in delta mode; empty in full mode.
	Base string
	// Head is the resolved HEAD SHA in delta mode; empty in full mode.
	Head string
	// Changed is the sorted list of repo-relative files changed since Base.
	// Nil/empty in full mode.
	Changed []string
	// Root is the absolute path of the analysis boundary (ScanRoot). All
	// extractors walk this directory. Equal to GitRoot when --root is absent.
	Root string
	// GitRoot is the absolute path to the git repository toplevel, or empty
	// when the directory is not inside a git repository (non-git full-mode runs).
	// Git operations (history, diff) use this as their working root.
	GitRoot string
	// SubtreePrefix is the GitRoot-relative path from GitRoot to Root
	// (filepath.Rel(GitRoot, Root)). Empty when Root equals GitRoot, or when
	// GitRoot is empty (non-git). Git history consumers use this to scope
	// commands to the subtree (Task 3).
	SubtreePrefix string
	// Mode indicates whether this is a delta or full-repo run.
	Mode ScopeMode
}

// Resolver supplies the version-control queries scope resolution needs.
// cmd wires the concrete git implementation; tests use an in-memory fake.
// scope itself stays free of process and tool dependencies.
type Resolver interface {
	// RepoRoot returns the absolute repository root.
	RepoRoot(ctx context.Context) (string, error)
	// HeadRef returns the resolved HEAD SHA.
	HeadRef(ctx context.Context) (string, error)
	// Changed returns the repo-relative files changed between base and head.
	Changed(ctx context.Context, base, head string) ([]string, error)
}

// Resolve determines the Scope for a run.
//
// It resolves the git root via the Resolver. In full mode a failing resolver
// (e.g. non-git directory) is non-fatal: GitRoot is set to "" and analysis
// continues. In delta mode a failing resolver is a hard error (no git → no diff).
//
// The analysis boundary (Scope.Root) is set from cfg.Root when non-empty;
// otherwise it falls back to the git root; when both are empty it falls back to
// cfg.WorkDir. Changed files are sorted here so scope's determinism contract
// does not depend on resolver discipline.
func Resolve(ctx context.Context, cfg config.ScopeConfig, r Resolver) (Scope, error) {
	gitRoot, rootErr := r.RepoRoot(ctx)

	if cfg.Full {
		// Non-git directories are analysable in full mode: degrade gracefully.
		if rootErr != nil {
			gitRoot = ""
		}
		scanRoot := resolveScanRoot(cfg, gitRoot)
		return Scope{
			Root:          scanRoot,
			GitRoot:       gitRoot,
			SubtreePrefix: subtreePrefix(gitRoot, scanRoot),
			Mode:          ModeFull,
		}, nil
	}

	// Delta mode requires git — no git means no diff base.
	if rootErr != nil {
		return Scope{}, fmt.Errorf("scope: resolve repo root: %w", rootErr)
	}

	head, err := r.HeadRef(ctx)
	if err != nil {
		return Scope{}, fmt.Errorf("scope: resolve HEAD: %w", err)
	}

	changed, err := r.Changed(ctx, cfg.Base, head)
	if err != nil {
		return Scope{}, fmt.Errorf("scope: resolve changed files: %w", err)
	}
	scanRoot := resolveScanRoot(cfg, gitRoot)
	prefix := subtreePrefix(gitRoot, scanRoot)
	changed = rebaseChangedFiles(prefix, changed)
	sort.Strings(changed)

	return Scope{
		Base:          cfg.Base,
		Head:          head,
		Changed:       changed,
		Root:          scanRoot,
		GitRoot:       gitRoot,
		SubtreePrefix: prefix,
		Mode:          ModeDelta,
	}, nil
}

// rebaseChangedFiles keeps only paths under prefix and strips the prefix
// component. When prefix is empty, the input is returned unchanged —
// the no-op path for --root-absent runs where Root==GitRoot.
// Git paths always use "/" as separator.
func rebaseChangedFiles(prefix string, files []string) []string {
	if prefix == "" {
		return files
	}
	sep := prefix + "/"
	var out []string
	for _, f := range files {
		if rel, ok := strings.CutPrefix(f, sep); ok {
			out = append(out, rel)
		}
	}
	return out
}

// resolveScanRoot determines the analysis boundary from the config and resolved
// git root. Priority: explicit cfg.Root → gitRoot → cfg.WorkDir.
// When cfg.Root is empty and gitRoot is non-empty the result equals gitRoot,
// so --root-absent runs are byte-identical to before this change.
func resolveScanRoot(cfg config.ScopeConfig, gitRoot string) string {
	if cfg.Root != "" {
		return cfg.Root
	}
	if gitRoot != "" {
		return gitRoot
	}
	return cfg.WorkDir
}

// subtreePrefix returns the gitRoot-relative path from gitRoot to scanRoot.
// Returns "" when they are equal, when gitRoot is empty (non-git), or when
// scanRoot is not under gitRoot (e.g. a relative escape via "..").
func subtreePrefix(gitRoot, scanRoot string) string {
	if gitRoot == "" || gitRoot == scanRoot {
		return ""
	}
	rel, err := filepath.Rel(gitRoot, scanRoot)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	return rel
}
