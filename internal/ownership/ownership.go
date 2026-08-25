// Package ownership resolves module owners from CODEOWNERS files or git-author
// history, producing a module→owner map that fills gaps in config-authored
// ownership metadata.
//
// Source precedence (all-or-nothing per source, never mixed per-file):
//  1. CODEOWNERS — searched at .github/CODEOWNERS, CODEOWNERS, docs/CODEOWNERS.
//     If any of these files exists, it is the sole source. Files not matched by
//     any rule get no owner. git-author is NOT consulted for individual misses.
//  2. git-author — used ONLY when no CODEOWNERS file exists anywhere. A git log
//     pass bounded to the most recent maxCommitsGitAuthor commits aggregates
//     authors per file; the dominant author becomes the module owner. When that
//     bounded pass resolves zero modules, a second, unbounded pass is tried
//     before giving up (see resolveFromGitAuthor).
//
// When neither source exists, Resolve returns an empty map with SourceNone —
// ownership is never fabricated. A git-log run that hits its timeout (gitTimeout)
// without resolving any data is reported as SourceGitTimeout instead, so
// downstream diagnostics can tell "the walk didn't finish" from "ran clean, found
// nothing."
//
// The returned map is keyed by config module name (from the ModuleMap). Paths
// that do not match any configured module are silently skipped.
package ownership

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	gitTool    = "git"
	gitTimeout = 15 * time.Second

	// maxCommitsGitAuthor bounds the git-author history walk by COMMIT COUNT,
	// not calendar time. A --since=N-months window would reintroduce the exact
	// silent-none failure mode this bound closes: a stable, low-churn module
	// with no commits inside the window would resolve to no owner. A count-based
	// bound keeps the walk fast regardless of repo age/size while covering
	// realistic per-repo history depth; see resolveFromGitAuthor for the
	// unbounded fallback used when the bounded walk resolves nothing.
	maxCommitsGitAuthor = 5000
)

// codeownersLocations are the candidate paths for CODEOWNERS, searched in order.
var codeownersLocations = []string{
	".github/CODEOWNERS",
	"CODEOWNERS",
	"docs/CODEOWNERS",
}

// ownerRule is a single parsed line from a CODEOWNERS file.
type ownerRule struct {
	pattern string
	owner   string // first owner on the line; empty lines and comments are skipped
}

// Source identifies where resolved owners came from, surfaced as owner_source in
// the diagnostic so consumers can judge distance confidence.
type Source string

const (
	// SourceCodeowners means owners were resolved from a CODEOWNERS file.
	SourceCodeowners Source = "codeowners"
	// SourceGit means owners were resolved from git-author history (no CODEOWNERS).
	SourceGit Source = "git"
	// SourceNone means no owner signal — neither source produced any owner. This
	// is a clean result: the source ran (or there was nothing to run) and
	// genuinely found no data to attribute.
	SourceNone Source = "none"
	// SourceGitTimeout means the git-author history walk hit its timeout
	// (gitTimeout) without resolving any owner data. Distinct from SourceNone —
	// this is "the walk didn't finish," not "ran clean, found nothing" — so
	// downstream diagnostics don't read a timeout as a genuinely-unattributed repo.
	SourceGitTimeout Source = "git_timeout"
	// SourceCodeownersNoMatch means a CODEOWNERS file with at least one real
	// rule was found but none of its rules matched any path belonging to a
	// configured module. Distinct from SourceNone — real rules resolving zero
	// owners is far more often a subtree/path-prefix or module-glob mismatch
	// (e.g. the case-variant --root bug) than a genuinely unattributed repo,
	// so callers surface it as a disclosure-worthy degradation rather than
	// silently collapsing to SourceNone. An empty or comment-only CODEOWNERS
	// (a fresh-repo placeholder) carries no signal and falls through to the
	// git-author path instead — it must not trigger the degradation warning.
	SourceCodeownersNoMatch Source = "codeowners_no_match"
)

// Resolve returns a map from config module name to owner string, plus the Source
// that produced it.
//
// scanRoot is the analysis boundary (absolute path) — the subtree archfit walks,
// and the root that module path globs are relative to. gitRoot is the git
// repository toplevel (absolute path), or "" when not a git repo; CODEOWNERS is
// located here and git-log paths are relative to it. subtreePrefix is the
// gitRoot-relative path to scanRoot (filepath.Rel(gitRoot, scanRoot)), empty when
// scanRoot == gitRoot or gitRoot is "".
//
// When scanRoot is a subtree of a larger monorepo, CODEOWNERS lives at gitRoot
// (not the subtree) and its patterns are gitRoot-relative, while module globs are
// scanRoot-relative — so file paths are matched against CODEOWNERS using their
// gitRoot-relative form (subtreePrefix + scanRel) but mapped to modules using
// their scanRoot-relative form. subtreePrefix=="" makes this byte-identical to a
// whole-repo scan.
//
// modules is the config module map used to aggregate file paths to module names.
// runner is used only for the git-author fallback path.
//
// Never returns an error — tool/file absence yields an empty map.
func Resolve(ctx context.Context, scanRoot, gitRoot, subtreePrefix string, modules policy.ModuleMap, runner toolrun.Runner) (map[string]string, Source) {
	// CODEOWNERS lives at the git repository root (the monorepo root), not the
	// analyzed subtree. Fall back to scanRoot when gitRoot is unknown (non-git).
	coRoot := gitRoot
	if coRoot == "" {
		coRoot = scanRoot
	}

	// Try CODEOWNERS first. A file that parses to zero rules (empty or
	// comment-only placeholder) carries no signal — treat it like no
	// CODEOWNERS at all rather than a suspicious no-match.
	rules, found := loadCodeowners(coRoot)
	if found && len(rules) > 0 {
		m := resolveFromCodeowners(rules, scanRoot, subtreePrefix, modules)
		if len(m) == 0 {
			// CODEOWNERS present but no rule mapped to a configured module —
			// a suspicious combination (see SourceCodeownersNoMatch), not the
			// benign "nothing to attribute" case.
			return m, SourceCodeownersNoMatch
		}
		return m, SourceCodeowners
	}

	// Fall back to git-author only when no CODEOWNERS exists at all.
	m, timedOut := resolveFromGitAuthor(ctx, scanRoot, gitRoot, subtreePrefix, modules, runner)
	if timedOut {
		// The walk did not finish — distinct from a clean run that found
		// nothing, so downstream diagnostics don't mistake "unknown" for
		// "genuinely unattributed."
		return map[string]string{}, SourceGitTimeout
	}
	if len(m) == 0 {
		return m, SourceNone
	}
	return m, SourceGit
}

// ---------------------------------------------------------------------------
// CODEOWNERS path
// ---------------------------------------------------------------------------

// loadCodeowners searches the candidate locations and returns the parsed rules
// from the first file found, along with found=true. Returns nil, false when no
// CODEOWNERS file exists at any candidate location.
func loadCodeowners(root string) ([]ownerRule, bool) {
	for _, loc := range codeownersLocations {
		path := filepath.Join(root, filepath.FromSlash(loc))
		data, err := os.ReadFile(path) // #nosec G304 — well-known relative path under repo root
		if err != nil {
			continue
		}
		return parseCodeowners(data), true
	}
	return nil, false
}

// parseCodeowners parses CODEOWNERS content into a slice of rules.
// Comment lines (#) and blank lines are skipped. The slice preserves
// file order so last-match-wins can be applied by iterating from front
// to back and keeping the final match.
func parseCodeowners(data []byte) []ownerRule {
	var rules []ownerRule
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			// Pattern with no owner — skip (means "clear ownership").
			continue
		}
		rules = append(rules, ownerRule{pattern: fields[0], owner: fields[1]})
	}
	return rules
}

// matchCodeowners returns the owner for a repo-relative path using last-match-wins
// semantics (CODEOWNERS spec). Returns ("", false) when no rule matches.
func matchCodeowners(rules []ownerRule, repoPath string) (string, bool) {
	var matched string
	found := false
	for _, r := range rules {
		if codeownersMatch(r.pattern, repoPath) {
			matched = r.owner
			found = true
		}
	}
	return matched, found
}

// codeownersMatch reports whether a CODEOWNERS pattern matches a repo-relative
// path. It handles the two main CODEOWNERS pattern forms:
//   - Patterns without a "/" (or only a trailing "/") match any path component
//     using filepath.Match semantics (e.g. "*.go" matches "foo/bar.go").
//   - Patterns with a leading "/" are anchored to the repo root.
//   - Patterns ending in "/" match any file under that directory.
func codeownersMatch(pattern, repoPath string) bool {
	// Normalise path to forward slashes for consistent matching.
	repoPath = filepath.ToSlash(repoPath)

	// Strip leading slash — CODEOWNERS patterns with "/" are root-relative.
	anchored := strings.HasPrefix(pattern, "/")
	p := strings.TrimPrefix(pattern, "/")

	// Directory patterns: "src/" matches everything under src/.
	if strings.HasSuffix(p, "/") {
		dir := strings.TrimSuffix(p, "/")
		return repoPath == dir || strings.HasPrefix(repoPath, dir+"/")
	}

	if anchored {
		// Root-anchored: match against the full path.
		return matchPathPattern(p, repoPath)
	}

	// Unanchored: match last component or full path.
	// A pattern containing "/" is treated as a path relative to root when not
	// anchored (GitHub CODEOWNERS behaviour).
	if strings.Contains(p, "/") {
		return matchPathPattern(p, repoPath)
	}

	// No "/" — match against every path component (basename).
	base := filepath.Base(repoPath)
	ok, _ := filepath.Match(p, base)
	if ok {
		return true
	}
	// Also try matching against the full path for patterns like "*.go".
	ok, _ = filepath.Match(p, repoPath)
	return ok
}

// matchPathPattern matches a slash-bearing CODEOWNERS path pattern (leading slash
// already stripped) against a repo-relative path, applying gitignore/CODEOWNERS
// directory semantics: a pattern WITHOUT a trailing slash matches the named path
// AND, when it names a directory, everything beneath it. It does so by testing
// the pattern against repoPath and each of its ancestor directories. filepath.Match
// handles "*" segment globs, so this also covers wildcard directory patterns —
// e.g. "crates/ty*" matches "crates/ty_project/foo.rs" via the ancestor
// "crates/ty_project", and "svc/api" matches "svc/api/handler/v1.go" via "svc/api".
func matchPathPattern(p, repoPath string) bool {
	for d := repoPath; d != "" && d != "." && d != "/"; d = path.Dir(d) {
		if d == p {
			return true
		}
		if ok, _ := filepath.Match(p, d); ok {
			return true
		}
	}
	return false
}

// resolveFromCodeowners builds a module→owner map from CODEOWNERS rules by
// scanning all files under scanRoot and mapping matched files to config modules.
//
// CODEOWNERS patterns are gitRoot-relative, so each file is matched against its
// gitRoot-relative path (subtreePrefix + scanRoot-relative path); modules are
// keyed on the scanRoot-relative path. subtreePrefix=="" collapses both to the
// same value (whole-repo scan).
func resolveFromCodeowners(rules []ownerRule, scanRoot, subtreePrefix string, modules policy.ModuleMap) map[string]string {
	// module name → map[owner]count
	ownerCount := make(map[string]map[string]int)

	err := filepath.WalkDir(scanRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			name := d.Name()
			// Skip hidden dirs and common non-source directories.
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		scanRel, err := filepath.Rel(scanRoot, p)
		if err != nil {
			return nil
		}
		scanRel = filepath.ToSlash(scanRel)
		// repoRel is the gitRoot-relative path CODEOWNERS patterns match against.
		repoRel := path.Join(subtreePrefix, scanRel)

		owner, ok := matchCodeowners(rules, repoRel)
		if !ok {
			return nil
		}

		mod, ok := modules.ModuleForFile(scanRel)
		if !ok {
			return nil
		}

		if ownerCount[mod] == nil {
			ownerCount[mod] = make(map[string]int)
		}
		ownerCount[mod][owner]++
		return nil
	})
	if err != nil {
		return map[string]string{}
	}

	return dominantOwners(ownerCount)
}

// ---------------------------------------------------------------------------
// git-author fallback path
// ---------------------------------------------------------------------------

// resolveFromGitAuthor runs a git log pass bounded to the most recent
// maxCommitsGitAuthor commits and aggregates author emails per file to produce
// a module→owner map. When that bounded pass resolves zero modules (e.g. a
// stable module whose only touching commits fall outside the bounded window),
// a second, unbounded pass is tried before giving up — that pathological case
// is the one scenario where paying the full-history cost is worth it. Non-git
// dirs and git failures produce an empty map, never an error.
//
// The second return is true only when a git-log subprocess itself hit its
// timeout (gitTimeout) — distinct from a clean run that legitimately found no
// data. On a bounded-pass timeout the unbounded fallback is skipped: a slower,
// unbounded walk after a bounded one already timed out would almost certainly
// time out too, at greater cost.
//
// git log emits gitRoot-relative paths regardless of the working directory, so
// when scanRoot is a subtree the run is scoped with a subtreePrefix pathspec and
// each path is stripped back to its scanRoot-relative form before module mapping
// (module globs are scanRoot-relative). gitRoot=="" (non-git) runs from scanRoot.
func resolveFromGitAuthor(ctx context.Context, scanRoot, gitRoot, subtreePrefix string, modules policy.ModuleMap, runner toolrun.Runner) (map[string]string, bool) {
	workDir := gitRoot
	if workDir == "" {
		workDir = scanRoot
	}

	m, timedOut := runGitAuthorLog(ctx, workDir, subtreePrefix, modules, runner, maxCommitsGitAuthor)
	if timedOut || len(m) > 0 {
		return m, timedOut
	}

	// Bounded walk found nothing usable — fall back to the full history.
	return runGitAuthorLog(ctx, workDir, subtreePrefix, modules, runner, 0)
}

// runGitAuthorLog runs one `git log --format=%ae --name-only` pass, bounded to
// maxCommits (0 means unbounded — full history), and aggregates author emails
// per file into a module→owner map. The second return is true only when the
// subprocess itself hit gitTimeout; any other failure (non-git dir, git error)
// returns an empty map with false, matching Resolve's "never an error" contract.
func runGitAuthorLog(ctx context.Context, workDir, subtreePrefix string, modules policy.ModuleMap, runner toolrun.Runner, maxCommits int) (map[string]string, bool) {
	args := []string{"log", "--format=%ae", "--name-only"}
	if maxCommits > 0 {
		args = append(args, "-n", strconv.Itoa(maxCommits))
	}
	if subtreePrefix != "" {
		// Scope history to the analyzed subtree.
		args = append(args, "--", subtreePrefix)
	}
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    args,
		Timeout: gitTimeout,
		WorkDir: workDir,
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return map[string]string{}, true
	}
	if err != nil || out.ExitCode != 0 {
		return map[string]string{}, false
	}

	// module name → map[author]count
	ownerCount := make(map[string]map[string]int)
	var currentAuthor string

	for _, line := range strings.Split(string(out.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Lines from --format=%ae are author emails; file paths follow.
		if isAuthorLine(line) {
			currentAuthor = line
			continue
		}
		if currentAuthor == "" {
			continue
		}
		gitRel := filepath.ToSlash(line)
		// git paths are gitRoot-relative; map to the scanRoot-relative form.
		scanRel := gitRel
		if subtreePrefix != "" {
			trimmed := strings.TrimPrefix(gitRel, subtreePrefix+"/")
			if trimmed == gitRel {
				continue // file outside the analyzed subtree
			}
			scanRel = trimmed
		}
		mod, ok := modules.ModuleFor(scanRel)
		if !ok {
			continue
		}
		if ownerCount[mod] == nil {
			ownerCount[mod] = make(map[string]int)
		}
		ownerCount[mod][currentAuthor]++
	}

	return dominantOwners(ownerCount), false
}

// isAuthorLine heuristically identifies an author-email line in git log output.
func isAuthorLine(line string) bool {
	return strings.Contains(line, "@")
}

// ---------------------------------------------------------------------------
// Aggregation helpers
// ---------------------------------------------------------------------------

// dominantOwners reduces a module→{owner→count} map to module→dominantOwner.
// When counts tie, the alphabetically-first owner wins for determinism.
func dominantOwners(ownerCount map[string]map[string]int) map[string]string {
	result := make(map[string]string, len(ownerCount))
	for mod, counts := range ownerCount {
		result[mod] = dominant(counts)
	}
	return result
}

// dominant returns the key with the highest count; alphabetically-first on ties.
func dominant(counts map[string]int) string {
	type entry struct {
		key   string
		count int
	}
	entries := make([]entry, 0, len(counts))
	for k, c := range counts {
		entries = append(entries, entry{k, c})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].key < entries[j].key
	})
	if len(entries) == 0 {
		return ""
	}
	return entries[0].key
}
