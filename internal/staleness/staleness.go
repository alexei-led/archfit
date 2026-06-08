// Package staleness detects map-quality advisory findings: uncovered paths,
// dead module rules, and stale reviewed_at timestamps.
//
// All findings carry kind "advisory" and never contribute to gate verdicts.
package staleness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// defaultThreshold is the staleness threshold used when cfg.Threshold is zero.
const defaultThreshold = 90 * 24 * time.Hour

// Check inspects the graph against the module map and returns advisory findings
// for any of the three staleness conditions:
//
//   - "map/uncovered_path": a package or file node has no matching module paths glob.
//   - "map/dead_rule": a module paths glob matches zero graph nodes.
//   - "map/stale_review": a module's reviewed_at is set and older than threshold.
//
// Returns nil when cfg.Enabled is false.
func Check(g *graph.Graph, cfg config.StalenessConfig, now time.Time) []finding.Finding {
	if !cfg.Enabled {
		return nil
	}

	threshold := cfg.Threshold
	if threshold == 0 {
		threshold = defaultThreshold
	}

	uncovered := uncoveredPaths(g, cfg.Modules)
	dead := deadRules(g, cfg.Modules)
	stale := staleReviews(cfg.Modules, threshold, now)
	findings := make([]finding.Finding, 0, len(uncovered)+len(dead)+len(stale))
	findings = append(findings, uncovered...)
	findings = append(findings, dead...)
	findings = append(findings, stale...)
	return findings
}

// uncoveredPaths returns one advisory finding per package or file node that
// is not claimed by any module's paths globs.
func uncoveredPaths(g *graph.Graph, modules map[string]config.ModuleDef) []finding.Finding {
	var findings []finding.Finding
	for _, n := range g.Nodes() {
		if n.Kind != graph.NodeKindPackage && n.Kind != graph.NodeKindFile {
			continue
		}
		if !claimedByAnyModule(n.Path, modules) {
			f := advisoryFinding(
				"map/uncovered_path",
				fmt.Sprintf("node %q is not covered by any module paths glob", n.Path),
				n.Path,
			)
			findings = append(findings, f)
		}
	}
	return findings
}

// deadRules returns one advisory finding per module paths glob that matches
// zero nodes in the graph.
func deadRules(g *graph.Graph, modules map[string]config.ModuleDef) []finding.Finding {
	nodes := g.Nodes()
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	slices.Sort(names)
	var findings []finding.Finding
	for _, modName := range names {
		def := modules[modName]
		for _, pattern := range def.Paths {
			if !patternMatchesAnyNode(pattern, nodes) {
				f := advisoryFinding(
					"map/dead_rule",
					fmt.Sprintf("module %q paths glob %q matches no graph nodes", modName, pattern),
					pattern,
				)
				findings = append(findings, f)
			}
		}
	}
	return findings
}

// staleReviews returns one advisory finding per module whose reviewed_at is
// set and is older than threshold relative to now.
func staleReviews(modules map[string]config.ModuleDef, threshold time.Duration, now time.Time) []finding.Finding {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	slices.Sort(names)
	var findings []finding.Finding
	for _, modName := range names {
		def := modules[modName]
		if def.ReviewedAt.IsZero() {
			continue
		}
		age := now.Sub(def.ReviewedAt)
		if age > threshold {
			f := advisoryFinding(
				"map/stale_review",
				fmt.Sprintf(
					"module %q was last reviewed %.0f days ago (threshold %.0f days)",
					modName,
					age.Hours()/24,
					threshold.Hours()/24,
				),
				modName,
			)
			findings = append(findings, f)
		}
	}
	return findings
}

// claimedByAnyModule reports whether path is matched by at least one paths
// glob across all modules.
func claimedByAnyModule(path string, modules map[string]config.ModuleDef) bool {
	for _, def := range modules {
		for _, pattern := range def.Paths {
			if matched, _ := doublestar.Match(pattern, path); matched {
				return true
			}
		}
	}
	return false
}

// patternMatchesAnyNode reports whether pattern matches the path of at least
// one node in nodes.
func patternMatchesAnyNode(pattern string, nodes []graph.Node) bool {
	for _, n := range nodes {
		if matched, _ := doublestar.Match(pattern, n.Path); matched {
			return true
		}
	}
	return false
}

// advisoryFinding builds a Finding with kind "advisory" and no edge.
// The subject (a path, glob, or module name) is recorded in MatchedBy["subject"].
func advisoryFinding(ruleID, why, subject string) finding.Finding {
	return finding.Finding{
		ID:     fingerprintAdvisory(ruleID, subject),
		Kind:   "advisory",
		RuleID: ruleID,
		Status: finding.StatusNew,
		Why:    why,
		MatchedBy: map[string]string{
			"subject": subject,
		},
	}
}

// fingerprintAdvisory computes hex(sha256(ruleID + "\x00" + subject)[:16]),
// producing a stable 32-character hex ID for an advisory finding.
func fingerprintAdvisory(ruleID, subject string) string {
	h := sha256.Sum256([]byte(ruleID + "\x00" + subject))
	return hex.EncodeToString(h[:16])
}
