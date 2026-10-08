// Package staleness detects map-quality advisory findings: dead module rules
// and stale reviewed_at timestamps. Production source no module owns is the
// evaluation package's map completeness check, because it reads the source
// inventory rather than the graph.
//
// All findings carry kind "advisory" and never contribute to gate verdicts:
// a dead glob depends on producer completeness and a stale review on the
// clock, so neither may block a change.
package staleness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	modelrule "github.com/alexei-led/archfit/v3/internal/model/rule"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

// defaultThreshold is the staleness threshold used when cfg.Threshold is zero.
const defaultThreshold = 90 * 24 * time.Hour

// Check inspects the relationship set against the assessment policy and returns
// advisory findings for both staleness conditions.
func Check(s relationship.Set, cfg policy.AssessmentPolicy, now time.Time) []finding.Finding {
	modules, enabled, threshold := cfg.Topology.Modules, cfg.Staleness.Enabled, cfg.Staleness.Threshold
	if !enabled {
		return nil
	}

	if threshold == 0 {
		threshold = defaultThreshold
	}

	dead := deadRules(s, modules)
	stale := staleReviews(modules, threshold, now)
	findings := make([]finding.Finding, 0, len(dead)+len(stale))
	findings = append(findings, dead...)
	findings = append(findings, stale...)
	return findings
}

// deadRules returns one advisory finding per module paths glob that matches
// zero nodes in the relationship set.
func deadRules(s relationship.Set, modules map[string]policy.ModuleDef) []finding.Finding {
	nodes := s.Nodes
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
					modelrule.RuleIDMapDeadRule,
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
func staleReviews(modules map[string]policy.ModuleDef, threshold time.Duration, now time.Time) []finding.Finding {
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
				modelrule.RuleIDMapStaleReview,
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

// patternMatchesAnyNode reports whether pattern matches the path of at least
// one node in nodes.
func patternMatchesAnyNode(pattern string, nodes []relationship.Node) bool {
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
