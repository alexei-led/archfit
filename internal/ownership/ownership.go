// Package ownership resolves module owners from CODEOWNERS files or git-author
// history, producing a module→owner map that fills gaps in config-authored
// ownership metadata.
//
// Source precedence (all-or-nothing per source, never mixed per-file):
//  1. CODEOWNERS — searched at .github/CODEOWNERS, CODEOWNERS, docs/CODEOWNERS.
//     If any of these files exists, it is the sole source. Files not matched by
//     any rule get no owner. git-author is NOT consulted for individual misses.
//  2. git-author — used ONLY when no CODEOWNERS file exists anywhere. A single
//     git log pass aggregates authors per file; the dominant author becomes the
//     module owner.
//
// When neither source exists, Resolve returns an empty map — ownership is never
// fabricated.
//
// The returned map is keyed by config module name (from the ModuleMap). Paths
// that do not match any configured module are silently skipped.
package ownership

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	gitTool    = "git"
	gitTimeout = 15 * time.Second
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

// Resolve returns a map from config module name to owner string.
//
// root is the repository root (absolute path). modules is the config module map
// used to aggregate file paths to module names. runner is used only for the
// git-author fallback path.
//
// Never returns an error — tool/file absence yields an empty map.
func Resolve(ctx context.Context, root string, modules config.ModuleMap, runner toolrun.Runner) map[string]string {
	// Try CODEOWNERS first.
	rules, found := loadCodeowners(root)
	if found {
		return resolveFromCodeowners(rules, root, modules)
	}

	// Fall back to git-author only when no CODEOWNERS exists at all.
	return resolveFromGitAuthor(ctx, root, modules, runner)
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
		return strings.HasPrefix(repoPath, strings.TrimSuffix(p, "/"))
	}

	if anchored {
		// Root-anchored: match against the full path.
		ok, _ := filepath.Match(p, repoPath)
		return ok
	}

	// Unanchored: match last component or full path.
	// A pattern containing "/" is treated as a path relative to root when not
	// anchored (GitHub CODEOWNERS behaviour).
	if strings.Contains(p, "/") {
		ok, _ := filepath.Match(p, repoPath)
		return ok
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

// resolveFromCodeowners builds a module→owner map from CODEOWNERS rules by
// scanning all files under root and mapping matched files to config modules.
func resolveFromCodeowners(rules []ownerRule, root string, modules config.ModuleMap) map[string]string {
	// module name → map[owner]count
	ownerCount := make(map[string]map[string]int)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
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
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		owner, ok := matchCodeowners(rules, rel)
		if !ok {
			return nil
		}

		mod, ok := modules.ModuleFor(rel)
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

// resolveFromGitAuthor runs a single git log pass and aggregates author emails
// per file to produce a module→owner map. Non-git dirs and git failures produce
// an empty map, never an error.
func resolveFromGitAuthor(ctx context.Context, root string, modules config.ModuleMap, runner toolrun.Runner) map[string]string {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    []string{"log", "--format=%ae", "--name-only"},
		Timeout: gitTimeout,
		WorkDir: root,
	})
	if err != nil || out.ExitCode != 0 {
		return map[string]string{}
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
		// A line with "@" (or no "/" and not a path) is treated as an author.
		if isAuthorLine(line) {
			currentAuthor = line
			continue
		}
		if currentAuthor == "" {
			continue
		}
		rel := filepath.ToSlash(line)
		mod, ok := modules.ModuleFor(rel)
		if !ok {
			continue
		}
		if ownerCount[mod] == nil {
			ownerCount[mod] = make(map[string]int)
		}
		ownerCount[mod][currentAuthor]++
	}

	return dominantOwners(ownerCount)
}

// isAuthorLine heuristically identifies an author-email line in git log output.
// Email addresses contain "@"; file paths do not (in practice).
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

// FormatOwnerMap formats a module→owner map as a multi-line string for display.
// Used for diagnostics only.
func FormatOwnerMap(owners map[string]string) string {
	if len(owners) == 0 {
		return "(no ownership data)"
	}
	keys := make([]string, 0, len(owners))
	for k := range owners {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s: %s\n", k, owners[k])
	}
	return sb.String()
}
