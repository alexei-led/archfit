// Erosion gates (CI): the named fitness checks that keep the
// architecture-state contract from decaying back into the scalar report it
// replaced. Each check has one executable owner, and each owner proves it fires
// on a fixture that violates it — a structural rule nobody has watched fail is
// a rule nobody knows still works.
//
// The six names and their owners:
//
//	no_scalar_decision        TestErosion_NoScalarDecision        (this file)
//	no_dead_archfit_rule      TestErosion_NoDeadArchfitRule       (this file)
//	dimension_status_required TestErosion_DimensionStatusRequired (cmd/archfit)
//	config_hash_required      TestErosion_ConfigHashRequired      (cmd/archfit)
//	label_evidence_required   TestErosion_LabelEvidenceRequired   (cmd/archfit)
//	baseline_idempotent       TestErosion_BaselineIdempotent      (cmd/archfit)
package arch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decisionPath names the sources that turn evidence into the architecture
// verdict and the process exit code. Everything a run's decision passes through
// is listed here; nothing else is.
//
// Most entries are whole files or directories. application/analysis.go is
// scoped to one function on purpose: the run result still CARRIES the scorecard
// for the legacy renderers, and carrying a retired fact for one release is not
// the same defect as deciding from it.
var decisionPath = []decisionTarget{
	{path: "assessment/state"},                    // the metric-blind aggregator
	{path: "assessment/evaluation/state.go"},      // the finding split feeding it
	{path: "assessment/evaluation/dimensions.go"}, // the nine envelopes it reads
	{path: "assessment/score/gate.go"},            // the distributed-monolith seam gate
	{path: "application/analysis.go", funcs: []string{"outcomeFor", "seamAnchor"}},
}

// decisionTarget is one file or directory on the decision path. An empty funcs
// list checks the whole source; a non-empty one checks only those declarations.
type decisionTarget struct {
	path  string
	funcs []string
}

// scalarIdents are the repository-scalar names the decision path may not touch.
// The scalar still exists for the legacy diagnostic envelope; the point is that
// nothing on the path from evidence to exit code can read it.
var scalarIdents = map[string]string{
	"Scorecard":   "the repository scorecard",
	"Overall":     "the averaged repository score",
	"OverallBand": "the banded repository score",
}

// TestErosion_NoScalarDecision (no_scalar_decision) asserts no source on the
// decision path names a repository scalar.
//
// The whole point of the architecture-state contract is that an averaged number
// cannot produce or move the verdict: a repository with one catastrophic seam
// and eight healthy dimensions must not average its way to green. A single
// `if sc.Overall < threshold` reintroduces exactly that, and it would read as
// ordinary code at the call site.
func TestErosion_NoScalarDecision(t *testing.T) {
	checked := 0
	for _, target := range decisionPath {
		for _, path := range goSourcesUnder(t, target.path) {
			checked++
			for ident, what := range scalarIdentsFoundIn(t, path, target.funcs) {
				t.Errorf("%s names %s (%s): the architecture decision reads explicit gate results, "+
					"dimension statuses, and finding classifications — never a repository scalar",
					path, ident, what)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no decision-path source found: the rule would pass vacuously")
	}
}

// TestErosion_NoScalarDecisionFiresOnAScalarRead proves the check above is not
// passing because it looks at nothing.
func TestErosion_NoScalarDecisionFiresOnAScalarRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decide.go")
	const src = `package decide

type card struct{ Overall int }

func verdict(sc card) string {
	if sc.Overall < 50 {
		return "blocked"
	}
	return "healthy"
}
`
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	found := scalarIdentsFoundIn(t, path, []string{"verdict"})
	if _, ok := found["Overall"]; !ok {
		t.Errorf("scalarIdentsFoundIn(%s) = %v, want the scalar read reported", path, found)
	}
}

// TestErosion_NoDeadArchfitRule (no_dead_archfit_rule) asserts every rule in the
// self-config still aims at source that exists.
//
// A rule matching nothing is worse than no rule: it reports "0 violations" for
// a boundary nobody is checking, and the report cannot tell that apart from a
// boundary being honoured. The dead-rule logic itself is owned by the self-model
// suite; this is the named entry point CI runs it under.
func TestErosion_NoDeadArchfitRule(t *testing.T) {
	TestSelfModelHasNoDeadRules(t)
}

// scalarIdentsFoundIn returns the scalar identifiers a source names, keyed by
// identifier. It matches selector expressions and type names alike, because
// `sc.Overall`, `score.Scorecard{}`, and a `Scorecard` parameter are all reads
// of the same retired fact.
//
// An empty funcs list inspects the whole file; otherwise only those top-level
// declarations are inspected.
func scalarIdentsFoundIn(t *testing.T, path string, funcs []string) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	roots := []ast.Node{file}
	if len(funcs) > 0 {
		roots = declsNamed(t, file, path, funcs)
	}

	found := map[string]string{}
	for _, root := range roots {
		ast.Inspect(root, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if what, forbidden := scalarIdents[ident.Name]; forbidden {
				found[ident.Name] = what
			}
			return true
		})
	}
	return found
}

// declsNamed resolves function names to their declarations, failing when one is
// missing: a scoped rule whose target was renamed away silently checks nothing.
func declsNamed(t *testing.T, file *ast.File, path string, funcs []string) []ast.Node {
	t.Helper()
	byName := map[string]ast.Node{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			byName[fn.Name.Name] = fn
		}
	}
	out := make([]ast.Node, 0, len(funcs))
	for _, name := range funcs {
		decl, ok := byName[name]
		if !ok {
			t.Fatalf("%s declares no func %s: the scoped decision rule now checks nothing — "+
				"point it at the renamed decision function", path, name)
		}
		out = append(out, decl)
	}
	return out
}

// goSourcesUnder lists the production Go sources at target, which is either one
// file or a directory to read non-recursively.
func goSourcesUnder(t *testing.T, target string) []string {
	t.Helper()
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat %s: %v", target, err)
	}
	if !info.IsDir() {
		return []string{target}
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != goSourceExt || strings.HasSuffix(name, "_test"+goSourceExt) {
			continue
		}
		out = append(out, filepath.Join(target, name))
	}
	return out
}
