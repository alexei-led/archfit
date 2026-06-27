package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/syntax"
)

// ---------------------------------------------------------------------------
// ForbiddenRoleDependency
// ---------------------------------------------------------------------------

// forbiddenRoleDependency fires when an edge exists from a node whose role
// matches def.FromRole (at or above def.MinConfidence) to a node whose role
// matches def.ToRole (at or above def.MinConfidence). When def.MinConfidence
// is empty, syntax.ConfHigh is used as the default threshold. When
// ev.Roles is nil (syntax off), the rule silently returns nil.
type forbiddenRoleDependency struct {
	def config.RuleDef
}

func (r *forbiddenRoleDependency) ID() string { return r.def.ID }

func (r *forbiddenRoleDependency) Check(g *graph.Graph, ev Evidence) []finding.Finding {
	if ev.Roles == nil {
		return nil
	}
	minConf := r.def.MinConfidence
	if minConf == "" {
		minConf = syntax.ConfHigh
	}

	var out []finding.Finding
	for _, e := range g.Edges() {
		fromHits := ev.Roles.RolesFor(e.From)
		toHits := ev.Roles.RolesFor(e.To)

		fromMatch := false
		for _, h := range fromHits {
			if h.Role == r.def.FromRole && syntax.ConfidenceMeets(h.Confidence, minConf) {
				fromMatch = true
				break
			}
		}
		if !fromMatch {
			continue
		}

		toMatch := false
		for _, h := range toHits {
			if h.Role == r.def.ToRole && syntax.ConfidenceMeets(h.Confidence, minConf) {
				toMatch = true
				break
			}
		}
		if !toMatch {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"from_role": r.def.FromRole,
			"to_role":   r.def.ToRole,
		}
		f.Why = "Role dependency from " + r.def.FromRole + " to " + r.def.ToRole + " is forbidden"
		f.Constraint = "Remove the dependency or restructure the architectural layers"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// TestInProduction
// ---------------------------------------------------------------------------

// testInProduction fires when a production (non-test) file imports a test
// framework. This detects test code compiled into the production binary —
// the canonical case is mock files with package X (no _test.go suffix) that
// import testify/mock or gomock.
//
// Evidence: SyntaxFacts of Kind="test_import" whose File passes IsTestFile=false.
// One finding per (file, framework) pair. When ev.SyntaxFacts is empty, returns nil.
// Default gate is "warn" (advisory, non-blocking); config can promote to "fail".
type testInProduction struct {
	def config.RuleDef
	mm  config.ModuleMap
}

func (r *testInProduction) ID() string { return r.def.ID }

func (r *testInProduction) Check(_ *graph.Graph, ev Evidence) []finding.Finding {
	if len(ev.SyntaxFacts) == 0 {
		return nil
	}

	// Deduplicate on (file, framework) to avoid repeated identical findings.
	type key struct{ file, framework string }
	seen := make(map[key]struct{})

	var out []finding.Finding
	for _, f := range ev.SyntaxFacts {
		if f.Kind != "test_import" {
			continue
		}
		if syntax.IsTestFile(f.Language, f.File) {
			continue // expected: test files may import test frameworks
		}

		k := key{file: f.File, framework: f.Framework}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}

		// Use module path for the endpoint when the file is owned by a declared module.
		endpoint := f.File
		if mod, ok := r.mm.ModuleFor(f.File); ok {
			endpoint = mod
		}

		h := sha256.Sum256([]byte(r.def.ID + "\x00" + f.File + "\x00" + f.Framework))
		fnd := finding.Finding{
			ID:       hex.EncodeToString(h[:16]),
			Kind:     kindGate,
			RuleID:   r.def.ID,
			Status:   finding.StatusNew,
			Severity: finding.SeverityHigh,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: endpoint},
				To:   finding.Endpoint{Path: endpoint},
			},
			MatchedBy: map[string]string{
				matchedByFile: f.File,
				"framework":   f.Framework,
				"language":    f.Language,
			},
			Why:        fmt.Sprintf("Production file %q imports test framework %q", f.File, f.Framework),
			Constraint: "Move to *_test.go or add a build tag; test frameworks must not ship in the production binary",
		}
		out = append(out, fnd)
	}
	return out
}
