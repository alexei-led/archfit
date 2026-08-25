package staleness_test

import (
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/staleness"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	globBarAll   = "internal/bar/**"
	globGhostAll = "internal/ghost/**"
	globAuthAll  = "internal/auth/**"
	pathAuthGo   = "internal/auth/auth.go"
	modAuth      = "auth"
)

// Node kinds the relationship contract carries, restated locally: staleness
// consumes relationship.Set and never sees the extractor graph that names them.
const (
	nodeKindRepo     = "repo"
	nodeKindModule   = "module"
	nodeKindPackage  = "package"
	nodeKindExternal = "external"
)

// buildGraph builds a relationship.Set fixture. Node IDs follow the contract's
// "kind:path" form, which is what NodePath and ModuleKey read.
func buildGraph(nodes []relationship.Node) relationship.Set {
	set := relationship.Set{Nodes: make([]relationship.Node, 0, len(nodes))}
	for _, n := range nodes {
		n.ID = n.Kind + ":" + n.Path
		set.Nodes = append(set.Nodes, n)
	}
	return set
}

// stalenessCase is the test-local module-review policy input: the staleness
// knobs plus the module topology the check resolves declarations against.
type stalenessCase struct {
	Enabled   bool
	Threshold time.Duration
	Modules   map[string]policy.ModuleDef
}

func assessmentPolicy(cfg stalenessCase) policy.AssessmentPolicy {
	return policy.AssessmentPolicy{Topology: policy.TopologyView{Modules: cfg.Modules}, Staleness: policy.StalenessPolicy{Enabled: cfg.Enabled, Threshold: cfg.Threshold}}
}

func byRule(findings []finding.Finding, ruleID string) []finding.Finding {
	var out []finding.Finding
	for _, f := range findings {
		if f.RuleID == ruleID {
			out = append(out, f)
		}
	}
	return out
}

func TestCheck_DisabledReturnsNil(t *testing.T) {
	g := buildGraph([]relationship.Node{
		{Kind: nodeKindPackage, Path: "internal/foo"},
	})
	cfg := stalenessCase{
		Enabled: false,
		Modules: map[string]policy.ModuleDef{
			"foo": {Paths: []string{globBarAll}},
		},
	}
	got := staleness.Check(g, assessmentPolicy(cfg), time.Now())
	if got != nil {
		t.Errorf("disabled: expected nil, got %v", got)
	}
}

func TestCheck_UncoveredPath(t *testing.T) {
	// Package node with no matching module glob → uncovered_path.
	// A second node that IS claimed → no uncovered_path for it.
	g := buildGraph([]relationship.Node{
		{Kind: nodeKindPackage, Path: "internal/foo/foo.go"},
		{Kind: nodeKindPackage, Path: "internal/bar/bar.go"},
	})
	cfg := stalenessCase{
		Enabled: true,
		Modules: map[string]policy.ModuleDef{
			// Only claims internal/bar/**.
			"bar": {Paths: []string{globBarAll}},
		},
	}
	findings := staleness.Check(g, assessmentPolicy(cfg), time.Now())

	uncovered := byRule(findings, "map/uncovered_path")
	dead := byRule(findings, "map/dead_rule")

	if len(uncovered) != 1 || uncovered[0].MatchedBy["subject"] != "internal/foo/foo.go" {
		t.Errorf("uncovered_path: want subject internal/foo/foo.go, got %v", uncovered)
	}
	// globBarAll matches internal/bar/bar.go → no dead rule.
	if len(dead) != 0 {
		t.Errorf("dead_rule: expected none, got %v", dead)
	}
}

func TestCheck_DeadRule(t *testing.T) {
	// Module whose paths glob matches no graph node → dead_rule finding.
	g := buildGraph([]relationship.Node{
		{Kind: nodeKindPackage, Path: "internal/bar/bar.go"},
	})
	cfg := stalenessCase{
		Enabled: true,
		Modules: map[string]policy.ModuleDef{
			"bar":   {Paths: []string{globBarAll}},
			"ghost": {Paths: []string{globGhostAll}}, // matches nothing
		},
	}
	findings := staleness.Check(g, assessmentPolicy(cfg), time.Now())

	dead := byRule(findings, "map/dead_rule")
	uncovered := byRule(findings, "map/uncovered_path")

	if len(dead) != 1 || dead[0].MatchedBy["subject"] != globGhostAll {
		t.Errorf("dead_rule: want subject %s, got %v", globGhostAll, dead)
	}
	// internal/bar/bar.go is claimed → no uncovered path.
	if len(uncovered) != 0 {
		t.Errorf("uncovered_path: expected none, got %v", uncovered)
	}
}

func TestCheck_StaleReview_Triggers(t *testing.T) {
	// reviewed_at 100 days ago with 30-day threshold → stale_review.
	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	reviewedAt := now.Add(-100 * 24 * time.Hour)

	g := buildGraph([]relationship.Node{
		{Kind: nodeKindPackage, Path: pathAuthGo},
	})
	cfg := stalenessCase{
		Enabled:   true,
		Threshold: 30 * 24 * time.Hour,
		Modules: map[string]policy.ModuleDef{
			modAuth: {
				Paths:      []string{globAuthAll},
				ReviewedAt: reviewedAt,
			},
		},
	}
	findings := staleness.Check(g, assessmentPolicy(cfg), now)

	stale := byRule(findings, "map/stale_review")
	if len(stale) != 1 || stale[0].MatchedBy["subject"] != modAuth {
		t.Errorf("stale_review triggered: want [%s], got %v", modAuth, stale)
	}
}

func TestCheck_StaleReview_DoesNotTrigger(t *testing.T) {
	// reviewed_at 10 days ago with 30-day threshold → no stale_review.
	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	reviewedAt := now.Add(-10 * 24 * time.Hour)

	g := buildGraph([]relationship.Node{
		{Kind: nodeKindPackage, Path: pathAuthGo},
	})
	cfg := stalenessCase{
		Enabled:   true,
		Threshold: 30 * 24 * time.Hour,
		Modules: map[string]policy.ModuleDef{
			modAuth: {
				Paths:      []string{globAuthAll},
				ReviewedAt: reviewedAt,
			},
		},
	}
	findings := staleness.Check(g, assessmentPolicy(cfg), now)

	for _, f := range findings {
		if f.RuleID == "map/stale_review" {
			t.Errorf("stale_review must not trigger for recent review: %+v", f)
		}
	}
}

func TestCheck_StaleReview_ZeroReviewedAt_NoFinding(t *testing.T) {
	// ReviewedAt zero (unset) → no stale_review regardless of threshold.
	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)

	g := buildGraph([]relationship.Node{
		{Kind: nodeKindPackage, Path: pathAuthGo},
	})
	cfg := stalenessCase{
		Enabled:   true,
		Threshold: 30 * 24 * time.Hour,
		Modules: map[string]policy.ModuleDef{
			modAuth: {
				Paths: []string{globAuthAll},
				// ReviewedAt zero: never reviewed.
			},
		},
	}
	findings := staleness.Check(g, assessmentPolicy(cfg), now)

	for _, f := range findings {
		if f.RuleID == "map/stale_review" {
			t.Errorf("stale_review must not fire for zero reviewed_at: %+v", f)
		}
	}
}

func TestCheck_DefaultThreshold(t *testing.T) {
	// Threshold zero → defaults to 90 days. 91-day-old review must trigger.
	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	reviewedAt := now.Add(-91 * 24 * time.Hour)

	g := buildGraph([]relationship.Node{
		{Kind: nodeKindPackage, Path: "pkg/api/api.go"},
	})
	cfg := stalenessCase{
		Enabled:   true,
		Threshold: 0, // zero → defaultThreshold (90 days)
		Modules: map[string]policy.ModuleDef{
			"api": {
				Paths:      []string{"pkg/api/**"},
				ReviewedAt: reviewedAt,
			},
		},
	}
	findings := staleness.Check(g, assessmentPolicy(cfg), now)

	stale := byRule(findings, "map/stale_review")
	if len(stale) != 1 || stale[0].MatchedBy["subject"] != "api" {
		t.Errorf("default threshold: want stale [api], got %v", stale)
	}
}

func TestCheck_NonPackageNodesNotUncovered(t *testing.T) {
	// Repo/module/external nodes are not candidates for uncovered_path.
	g := buildGraph([]relationship.Node{
		{Kind: nodeKindRepo, Path: "."},
		{Kind: nodeKindModule, Path: "internal/auth"},
		{Kind: nodeKindExternal, Path: "github.com/some/lib"},
	})
	cfg := stalenessCase{
		Enabled: true,
		Modules: map[string]policy.ModuleDef{},
	}
	findings := staleness.Check(g, assessmentPolicy(cfg), time.Now())

	for _, f := range findings {
		if f.RuleID == "map/uncovered_path" {
			t.Errorf("non-package node must not generate uncovered_path: %+v", f)
		}
	}
}

func TestCheck_AllFindingsAreAdvisory(t *testing.T) {
	// Every finding returned by Check must carry Kind == "advisory".
	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	oldReview := now.Add(-200 * 24 * time.Hour)

	g := buildGraph([]relationship.Node{
		{Kind: nodeKindPackage, Path: "internal/covered/a.go"},
		{Kind: nodeKindPackage, Path: "internal/uncovered/b.go"},
	})
	cfg := stalenessCase{
		Enabled:   true,
		Threshold: 30 * 24 * time.Hour,
		Modules: map[string]policy.ModuleDef{
			// covered: valid glob, old review → stale_review
			"covered": {
				Paths:      []string{"internal/covered/**"},
				ReviewedAt: oldReview,
			},
			// ghost: dead glob → dead_rule
			"ghost": {Paths: []string{globGhostAll}},
			// internal/uncovered/b.go not claimed → uncovered_path
		},
	}
	findings := staleness.Check(g, assessmentPolicy(cfg), now)

	if len(findings) == 0 {
		t.Fatal("expected at least one finding; got none")
	}
	for _, f := range findings {
		if f.Kind != "advisory" {
			t.Errorf("finding %q (rule=%s) has Kind=%q, want \"advisory\"", f.ID, f.RuleID, f.Kind)
		}
	}
}

func TestCheck_EmptyGraph_NoUncovered(t *testing.T) {
	// Empty graph → no uncovered_path; dead_rule fires for every pattern.
	g := buildGraph(nil)
	cfg := stalenessCase{
		Enabled: true,
		Modules: map[string]policy.ModuleDef{
			"foo": {Paths: []string{"internal/foo/**"}},
		},
	}
	findings := staleness.Check(g, assessmentPolicy(cfg), time.Now())

	uncovered := byRule(findings, "map/uncovered_path")
	dead := byRule(findings, "map/dead_rule")

	if len(uncovered) != 0 {
		t.Errorf("empty graph: expected 0 uncovered_path, got %d", len(uncovered))
	}
	if len(dead) != 1 {
		t.Errorf("empty graph: expected 1 dead_rule for internal/foo/**, got %d", len(dead))
	}
}
