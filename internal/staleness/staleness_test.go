package staleness_test

import (
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/staleness"
)

const (
	globBarAll   = "internal/bar/**"
	globGhostAll = "internal/ghost/**"
	globAuthAll  = "internal/auth/**"
	pathAuthGo   = "internal/auth/auth.go"
	modAuth      = "auth"
)

// buildGraph is a test helper that builds a Graph from a slice of nodes.
func buildGraph(nodes []graph.Node) *graph.Graph {
	return graph.Build([]graph.Facts{{Nodes: nodes}})
}

// byRule returns all findings with the given RuleID.
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
	g := buildGraph([]graph.Node{
		{Kind: graph.NodeKindPackage, Path: "internal/foo"},
	})
	cfg := config.StalenessConfig{
		Enabled: false,
		Modules: map[string]config.ModuleDef{
			"foo": {Paths: []string{globBarAll}},
		},
	}
	got := staleness.Check(g, cfg, time.Now())
	if got != nil {
		t.Errorf("disabled: expected nil, got %v", got)
	}
}

func TestCheck_UncoveredPath(t *testing.T) {
	// Package node with no matching module glob → uncovered_path.
	// A second node that IS claimed → no uncovered_path for it.
	g := buildGraph([]graph.Node{
		{Kind: graph.NodeKindPackage, Path: "internal/foo/foo.go"},
		{Kind: graph.NodeKindPackage, Path: "internal/bar/bar.go"},
	})
	cfg := config.StalenessConfig{
		Enabled: true,
		Modules: map[string]config.ModuleDef{
			// Only claims internal/bar/**.
			"bar": {Paths: []string{globBarAll}},
		},
	}
	findings := staleness.Check(g, cfg, time.Now())

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
	g := buildGraph([]graph.Node{
		{Kind: graph.NodeKindPackage, Path: "internal/bar/bar.go"},
	})
	cfg := config.StalenessConfig{
		Enabled: true,
		Modules: map[string]config.ModuleDef{
			"bar":   {Paths: []string{globBarAll}},
			"ghost": {Paths: []string{globGhostAll}}, // matches nothing
		},
	}
	findings := staleness.Check(g, cfg, time.Now())

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

	g := buildGraph([]graph.Node{
		{Kind: graph.NodeKindPackage, Path: pathAuthGo},
	})
	cfg := config.StalenessConfig{
		Enabled:   true,
		Threshold: 30 * 24 * time.Hour,
		Modules: map[string]config.ModuleDef{
			modAuth: {
				Paths:      []string{globAuthAll},
				ReviewedAt: reviewedAt,
			},
		},
	}
	findings := staleness.Check(g, cfg, now)

	stale := byRule(findings, "map/stale_review")
	if len(stale) != 1 || stale[0].MatchedBy["subject"] != modAuth {
		t.Errorf("stale_review triggered: want [%s], got %v", modAuth, stale)
	}
}

func TestCheck_StaleReview_DoesNotTrigger(t *testing.T) {
	// reviewed_at 10 days ago with 30-day threshold → no stale_review.
	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	reviewedAt := now.Add(-10 * 24 * time.Hour)

	g := buildGraph([]graph.Node{
		{Kind: graph.NodeKindPackage, Path: pathAuthGo},
	})
	cfg := config.StalenessConfig{
		Enabled:   true,
		Threshold: 30 * 24 * time.Hour,
		Modules: map[string]config.ModuleDef{
			modAuth: {
				Paths:      []string{globAuthAll},
				ReviewedAt: reviewedAt,
			},
		},
	}
	findings := staleness.Check(g, cfg, now)

	for _, f := range findings {
		if f.RuleID == "map/stale_review" {
			t.Errorf("stale_review must not trigger for recent review: %+v", f)
		}
	}
}

func TestCheck_StaleReview_ZeroReviewedAt_NoFinding(t *testing.T) {
	// ReviewedAt zero (unset) → no stale_review regardless of threshold.
	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)

	g := buildGraph([]graph.Node{
		{Kind: graph.NodeKindPackage, Path: pathAuthGo},
	})
	cfg := config.StalenessConfig{
		Enabled:   true,
		Threshold: 30 * 24 * time.Hour,
		Modules: map[string]config.ModuleDef{
			modAuth: {
				Paths: []string{globAuthAll},
				// ReviewedAt zero: never reviewed.
			},
		},
	}
	findings := staleness.Check(g, cfg, now)

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

	g := buildGraph([]graph.Node{
		{Kind: graph.NodeKindPackage, Path: "pkg/api/api.go"},
	})
	cfg := config.StalenessConfig{
		Enabled:   true,
		Threshold: 0, // zero → defaultThreshold (90 days)
		Modules: map[string]config.ModuleDef{
			"api": {
				Paths:      []string{"pkg/api/**"},
				ReviewedAt: reviewedAt,
			},
		},
	}
	findings := staleness.Check(g, cfg, now)

	stale := byRule(findings, "map/stale_review")
	if len(stale) != 1 || stale[0].MatchedBy["subject"] != "api" {
		t.Errorf("default threshold: want stale [api], got %v", stale)
	}
}

func TestCheck_NonPackageNodesNotUncovered(t *testing.T) {
	// Repo/module/external nodes are not candidates for uncovered_path.
	g := buildGraph([]graph.Node{
		{Kind: graph.NodeKindRepo, Path: "."},
		{Kind: graph.NodeKindModule, Path: "internal/auth"},
		{Kind: graph.NodeKindExternal, Path: "github.com/some/lib"},
	})
	cfg := config.StalenessConfig{
		Enabled: true,
		Modules: map[string]config.ModuleDef{},
	}
	findings := staleness.Check(g, cfg, time.Now())

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

	g := buildGraph([]graph.Node{
		{Kind: graph.NodeKindPackage, Path: "internal/covered/a.go"},
		{Kind: graph.NodeKindPackage, Path: "internal/uncovered/b.go"},
	})
	cfg := config.StalenessConfig{
		Enabled:   true,
		Threshold: 30 * 24 * time.Hour,
		Modules: map[string]config.ModuleDef{
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
	findings := staleness.Check(g, cfg, now)

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
	cfg := config.StalenessConfig{
		Enabled: true,
		Modules: map[string]config.ModuleDef{
			"foo": {Paths: []string{"internal/foo/**"}},
		},
	}
	findings := staleness.Check(g, cfg, time.Now())

	uncovered := byRule(findings, "map/uncovered_path")
	dead := byRule(findings, "map/dead_rule")

	if len(uncovered) != 0 {
		t.Errorf("empty graph: expected 0 uncovered_path, got %d", len(uncovered))
	}
	if len(dead) != 1 {
		t.Errorf("empty graph: expected 1 dead_rule for internal/foo/**, got %d", len(dead))
	}
}
