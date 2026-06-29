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
