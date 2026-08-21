package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/view"
)

// TestGitFindingDelta covers the `--base` git-origin block: how tasks are placed
// (introduced / pre-existing / unknown), which analyzer evidence is comparable,
// which families are active for a config, and the end-to-end `check --base
// --json` contract at every gate exit code.
//
// One exported test function by design — cmd/archfit sits at its public_api_max
// ceiling, so new coverage arrives as subtests, never as new exported names.
func TestGitFindingDelta(t *testing.T) {
	t.Parallel()
	t.Run("origin", testGitDeltaOrigin)
	t.Run("analyzer_evidence", testGitDeltaAnalyzerEvidence)
	t.Run("active_families", testGitDeltaActiveFamilies)
	t.Run("base_finding_ids", testGitDeltaBaseFindingIDs)
	t.Run("effective_config", testGitDeltaEffectiveConfig)
	t.Run("check_base_json", testGitDeltaCheckBaseJSON)
}

// gitDeltaRef is the base ref label used by the pure-comparison subtests.
const gitDeltaRef = "main"

func covRow(tool, status string) diagnostic.Coverage {
	return diagnostic.Coverage{Tool: tool, Status: status}
}

func agentTask(findingID, ruleID string) diagnostic.AgentTask {
	return diagnostic.AgentTask{FindingID: findingID, RuleID: ruleID}
}

// goPrimaryFamily is the single-family fixture used by the origin table: the
// pairing rules themselves are covered by testGitDeltaAnalyzerEvidence.
var goPrimaryFamily = []analyzerFamily{{name: toolGoPackages, tools: []string{toolGoPackages}, primary: true}}

func testGitDeltaOrigin(t *testing.T) {
	t.Parallel()
	const hash = "cfg-hash"
	comparableSide := analyzerEvidence{Coverage: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, Hash: hash}
	partialSide := analyzerEvidence{Coverage: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusPartial)}, Hash: hash}

	tests := []struct {
		name            string
		tasks           []diagnostic.AgentTask
		baseIDs         []string
		base            analyzerEvidence
		wantIntroduced  []string
		wantPreExisting []string
		wantUnknown     []string
		wantStatus      string
	}{
		{
			name:            "exact base match is pre-existing",
			tasks:           []diagnostic.AgentTask{agentTask("f1", "arch/forbidden")},
			baseIDs:         []string{"f1"},
			base:            comparableSide,
			wantPreExisting: []string{"f1"},
			wantStatus:      diagnostic.GitComparisonComparable,
		},
		{
			name:           "unmatched task with comparable evidence is introduced",
			tasks:          []diagnostic.AgentTask{agentTask("f2", "arch/forbidden")},
			baseIDs:        []string{"f1"},
			base:           comparableSide,
			wantIntroduced: []string{"f2"},
			wantStatus:     diagnostic.GitComparisonComparable,
		},
		{
			// A base entry the base run reported as fixed is dropped by
			// baseFindingIDs, so the same ID on head is genuinely new work.
			name:           "base fixed entry does not make a task pre-existing",
			tasks:          []diagnostic.AgentTask{agentTask("f1", "arch/forbidden")},
			baseIDs:        nil,
			base:           comparableSide,
			wantIntroduced: []string{"f1"},
			wantStatus:     diagnostic.GitComparisonComparable,
		},
		{
			name:        "unavailable analyzer evidence makes an unmatched task unknown",
			tasks:       []diagnostic.AgentTask{agentTask("f2", "arch/forbidden")},
			baseIDs:     []string{"f1"},
			base:        partialSide,
			wantUnknown: []string{"f2"},
			wantStatus:  diagnostic.GitComparisonUnknown,
		},
		{
			// Incomplete evidence never downgrades an exact ID match.
			name:            "exact match survives unavailable evidence",
			tasks:           []diagnostic.AgentTask{agentTask("f1", "arch/forbidden")},
			baseIDs:         []string{"f1"},
			base:            partialSide,
			wantPreExisting: []string{"f1"},
			wantStatus:      diagnostic.GitComparisonComparable,
		},
		{
			name:        "synthetic coupling-gate task is unknown before ID matching",
			tasks:       []diagnostic.AgentTask{agentTask(findingIDCouplingGate, ruleIDBCCouplingGate)},
			baseIDs:     []string{findingIDCouplingGate},
			base:        comparableSide,
			wantUnknown: []string{findingIDCouplingGate},
			wantStatus:  diagnostic.GitComparisonUnknown,
		},
		{
			name:    "config hash mismatch makes every unmatched task unknown",
			tasks:   []diagnostic.AgentTask{agentTask("f1", "arch/forbidden"), agentTask("f2", "arch/forbidden")},
			baseIDs: []string{"f1"},
			base: analyzerEvidence{
				Coverage: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)},
				Hash:     "other-hash",
			},
			wantPreExisting: []string{"f1"},
			wantUnknown:     []string{"f2"},
			wantStatus:      diagnostic.GitComparisonUnknown,
		},
		{
			name:            "lists use a stable sorted order",
			tasks:           []diagnostic.AgentTask{agentTask("z", "r"), agentTask("a", "r"), agentTask("m", "r"), agentTask("b", "r")},
			baseIDs:         []string{"m", "b"},
			base:            comparableSide,
			wantIntroduced:  []string{"a", "z"},
			wantPreExisting: []string{"b", "m"},
			wantStatus:      diagnostic.GitComparisonComparable,
		},
		{
			name:       "clean run still emits the block with empty lists",
			base:       comparableSide,
			wantStatus: diagnostic.GitComparisonComparable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildGitFindingDelta(gitDeltaInput{
				BaseRef:        gitDeltaRef,
				Tasks:          tc.tasks,
				BaseFindingIDs: tc.baseIDs,
				Head: analyzerEvidence{
					Coverage: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)},
					Hash:     hash,
				},
				Base:     tc.base,
				Families: goPrimaryFamily,
			})
			if got == nil {
				t.Fatal("buildGitFindingDelta returned nil; the block must always be present with --base")
			}
			if got.BaseRef != gitDeltaRef {
				t.Errorf("base_ref = %q, want %q", got.BaseRef, gitDeltaRef)
			}
			if got.ComparisonStatus != tc.wantStatus {
				t.Errorf("comparison_status = %q, want %q", got.ComparisonStatus, tc.wantStatus)
			}
			assertIDs(t, "introduced_finding_ids", got.IntroducedFindingIDs, tc.wantIntroduced)
			assertIDs(t, "pre_existing_finding_ids", got.PreExistingFindingIDs, tc.wantPreExisting)
			assertIDs(t, "unknown_origin_finding_ids", got.UnknownOriginFindingIDs, tc.wantUnknown)
			if got.ComparisonReasons == nil {
				t.Error("comparison_reasons must be a non-null array")
			}
			assertNonNullJSONArrays(t, got)
		})
	}
}

// assertIDs compares one ID list against its expectation and rejects a nil
// slice, which would serialise as JSON null instead of [].
func assertIDs(t *testing.T, field string, got, want []string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s must be a non-null array", field)
		return
	}
	if want == nil {
		want = []string{}
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", field, got, want)
	}
}

func assertNonNullJSONArrays(t *testing.T, d *diagnostic.GitFindingDelta) {
	t.Helper()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal git_finding_delta: %v", err)
	}
	if strings.Contains(string(raw), "null") {
		t.Errorf("git_finding_delta must never serialise a null list: %s", raw)
	}
}

func testGitDeltaAnalyzerEvidence(t *testing.T) {
	t.Parallel()
	goFam := analyzerFamily{name: toolGoPackages, tools: []string{toolGoPackages}, primary: true}
	scipFam := analyzerFamily{name: toolScip, tools: []string{toolScip}}
	astFam := analyzerFamily{name: toolAstGrep, tools: []string{toolAstGrep, toolAstGrepSyntax}}
	goGap := []diagnostic.CoverageGap{{Tool: toolGoPackages}}

	tests := []struct {
		name           string
		fam            analyzerFamily
		head, base     []diagnostic.Coverage
		headGap, bsGap []diagnostic.CoverageGap
		want           bool
	}{
		{name: "ok/ok", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: true},
		{name: "ok/not_applicable", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, want: true},
		{name: "not_applicable/ok", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: true},
		{name: "not_applicable both sides is ignored", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, want: true},
		{name: "primary absent with a coverage gap is unavailable", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, bsGap: goGap, want: false},
		{name: "partial is unavailable", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusPartial)}, want: false},
		{name: "timed out is unavailable", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusTimedOut)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: false},
		{name: "missing row on one side is unavailable", fam: goFam, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: false},
		{name: "missing row on both sides is unavailable", fam: goFam, want: false},
		{name: "duplicate row on one side is unavailable", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK), covRow(toolGoPackages, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: false},
		{name: "matching duplicate rows are comparable", fam: astFam, head: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrep, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrep, diagnostic.StatusOK)}, want: true},
		{name: "ast-grep family spans the pattern and syntax coverage names", fam: astFam, head: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrepSyntax, diagnostic.StatusDisabled)}, base: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrepSyntax, diagnostic.StatusDisabled)}, want: true},
		{name: "a row disabled on one side only still fails a mixed family", fam: astFam, head: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrepSyntax, diagnostic.StatusDisabled)}, base: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrepSyntax, diagnostic.StatusOK)}, want: false},
		{name: "disabled on both sides is ignored", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusDisabled)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusDisabled)}, want: true},
		{name: "disabled on one side only is unavailable", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusDisabled)}, want: false},
		{name: "non-primary absent is unavailable even without a gap", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ok, reasons := compareAnalyzerEvidence([]analyzerFamily{tc.fam},
				analyzerEvidence{Coverage: tc.head, Gaps: tc.headGap},
				analyzerEvidence{Coverage: tc.base, Gaps: tc.bsGap})
			if ok != tc.want {
				t.Fatalf("comparable = %v, want %v (reasons: %v)", ok, tc.want, reasons)
			}
			switch {
			case tc.want && len(reasons) != 0:
				t.Errorf("comparable family must produce no reason, got %v", reasons)
			case !tc.want && len(reasons) != 1:
				t.Errorf("want exactly one reason per unavailable family, got %v", reasons)
			case !tc.want && !strings.HasPrefix(reasons[0], tc.fam.name+": "):
				t.Errorf("reason %q must name the family %q", reasons[0], tc.fam.name)
			}
		})
	}

	t.Run("reasons are sorted and one per family", func(t *testing.T) {
		t.Parallel()
		fams := []analyzerFamily{
			{name: toolScip, tools: []string{toolScip}},
			{name: toolGoPackages, tools: []string{toolGoPackages}, primary: true},
			{name: toolJscpd, tools: []string{toolJscpd}},
		}
		head := analyzerEvidence{Coverage: []diagnostic.Coverage{
			covRow(toolScip, diagnostic.StatusOK),
			covRow(toolGoPackages, diagnostic.StatusOK),
			covRow(toolJscpd, diagnostic.StatusOK),
		}}
		base := analyzerEvidence{Coverage: []diagnostic.Coverage{
			covRow(toolScip, diagnostic.StatusPartial),
			covRow(toolGoPackages, diagnostic.StatusTimedOut),
			covRow(toolJscpd, diagnostic.StatusOK),
		}}
		delta := buildGitFindingDelta(gitDeltaInput{BaseRef: gitDeltaRef, Head: head, Base: base, Families: fams})
		if len(delta.ComparisonReasons) != 2 {
			t.Fatalf("comparison_reasons = %v, want one per unavailable family", delta.ComparisonReasons)
		}
		if !slices.IsSorted(delta.ComparisonReasons) {
			t.Errorf("comparison_reasons must be sorted: %v", delta.ComparisonReasons)
		}
	})
}

func testGitDeltaActiveFamilies(t *testing.T) {
	t.Parallel()
	names := func(fams []analyzerFamily) []string {
		out := make([]string, 0, len(fams))
		for _, f := range fams {
			out = append(out, f.name)
		}
		return out
	}
	primaries := names(analyzerFamilies(config.Config{}))
	if len(primaries) != len(languageRegistry) {
		t.Fatalf("a bare config must activate only the per-language primaries, got %v", primaries)
	}

	on := config.Analyzer{Enabled: view.ModeOn}
	timedOn := config.TimedAnalyzer{Enabled: view.ModeOn}
	tests := []struct {
		name string
		cfg  config.Config
		want []string
	}{
		{name: "rule patterns activate ast-grep", cfg: config.Config{
			Rules: []view.RuleDef{{ID: "r", Patterns: []view.PatternDef{{ID: "p", Lang: "go", Rule: "x"}}}},
		}, want: []string{toolAstGrep}},
		{name: "syntax activates ast-grep", cfg: config.Config{
			Analyzers: config.AnalyzersConfig{Syntax: on},
		}, want: []string{toolAstGrep}},
		{name: "scip activates both scip rows", cfg: config.Config{
			Analyzers: config.AnalyzersConfig{Scip: timedOn},
		}, want: []string{toolScip, toolScipSymbols}},
		{name: "clones activate jscpd", cfg: config.Config{
			Analyzers: config.AnalyzersConfig{Clones: timedOn},
		}, want: []string{toolJscpd}},
		{name: "cargo modules activate cargo-modules", cfg: config.Config{
			Analyzers: config.AnalyzersConfig{CargoModules: on},
		}, want: []string{toolCargoModules}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := names(analyzerFamilies(tc.cfg))
			for _, want := range tc.want {
				if !slices.Contains(got, want) {
					t.Errorf("family %q missing from %v", want, got)
				}
			}
			if len(got) != len(primaries)+len(tc.want) {
				t.Errorf("families = %v, want the primaries plus %v", got, tc.want)
			}
		})
	}
}

// testGitDeltaBaseFindingIDs pins the base projection: every lifecycle label
// except `fixed` is observed evidence, and the finding's kind is irrelevant —
// a base advisory still matches a head task promoted to gate kind, because the
// stable ID is all that crosses over.
func testGitDeltaBaseFindingIDs(t *testing.T) {
	t.Parallel()
	got := baseFindingIDs([]finding.Finding{
		{ID: "z", Kind: string(finding.KindAdvisory), Status: finding.StatusNew},
		{ID: "gone", Kind: string(finding.KindGate), Status: finding.StatusFixed},
		{ID: "a", Kind: string(finding.KindGate), Status: finding.StatusWaived},
		{ID: "m", Kind: string(finding.KindAdvisory), Status: finding.StatusExpiredWaiver},
	})
	if want := []string{"a", "m", "z"}; !slices.Equal(got, want) {
		t.Errorf("baseFindingIDs = %v, want %v (fixed dropped, sorted, kind ignored)", got, want)
	}
}

// testGitDeltaEffectiveConfig covers the base sub-run's config contract: it gets
// the caller's effective config (flag overrides included) through an independent
// module map, so the head pipeline's owner and deploy-unit backfill cannot leak
// head-tree evidence into the base measurement.
func testGitDeltaEffectiveConfig(t *testing.T) {
	t.Parallel()
	t.Run("module map is independent of the head config", func(t *testing.T) {
		t.Parallel()
		original := config.Config{Modules: map[string]module.ModuleDef{
			"a": {Paths: []string{"pkg/a/**"}},
		}}
		snapshot := withIndependentModules(original)
		// Stand in for runPipeline's owner backfill, which writes through the map.
		original.FillMissingOwners(map[string]string{"a": "head-tree-owner"})
		if def := snapshot.Modules["a"]; def.Owner != "" {
			t.Errorf("base config inherited a head-tree owner %q", def.Owner)
		}
		if def := original.Modules["a"]; def.Owner != "head-tree-owner" {
			t.Errorf("head config lost its own backfill: %+v", def)
		}
	})

	t.Run("analyzer overrides still reach the base run", func(t *testing.T) {
		t.Parallel()
		cfgPath := gitDeltaFixtureRepo(t, coupledModulesCfg)
		code, stdout, stderr := runArchfit(t, cmdAnalyze, flagBase, diffBaseRef, "--json", "--lang", "go", "-c", cfgPath)
		if code != 0 {
			t.Fatalf("analyze --base --lang go: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		var got gitDeltaJSON
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout)
		}
		if got.GitFindingDelta == nil {
			t.Fatalf("--base with a --lang override must still emit git_finding_delta\n%s", stdout)
		}
		// Both sides ran the same forced analyzer set, so nothing is unavailable.
		if len(got.GitFindingDelta.ComparisonReasons) != 0 {
			t.Errorf("comparison_reasons = %v, want none when both sides force the same analyzers",
				got.GitFindingDelta.ComparisonReasons)
		}
	})
}

// gitDeltaFixtureRepo builds a two-commit Go repo: the base commit holds only
// pkg/b, the head commit adds a pkg/a → pkg/b importer. The head run therefore
// carries a cross-module edge the base ref does not, and BOTH sides compile, so
// go/packages reports ok on both and the evidence is genuinely comparable.
func gitDeltaFixtureRepo(t *testing.T, cfgBody string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(markerGoMod, "module example.com/test\n\ngo 1.21\n")
	write("pkg/b/api/api.go", "package api\n\nfunc Secret() string { return \"s\" }\n")
	write(defaultConfigPath, cfgBody)
	gitInitFixtureRepo(t, dir)
	gitCommitAll(t, dir, "base: pkg/b only")

	write("pkg/a/a.go", "package a\n\nimport \"example.com/test/pkg/b/api\"\n\n"+
		"func UseSecret() string { return api.Secret() }\n")
	gitCommitAll(t, dir, "head: add the cross-module importer")
	return filepath.Join(dir, defaultConfigPath)
}

// gitDeltaJSON is the minimal decoder for the block under test.
type gitDeltaJSON struct {
	GitFindingDelta *struct {
		BaseRef           string   `json:"base_ref"`
		ComparisonStatus  string   `json:"comparison_status"`
		Introduced        []string `json:"introduced_finding_ids"`
		PreExisting       []string `json:"pre_existing_finding_ids"`
		UnknownOrigin     []string `json:"unknown_origin_finding_ids"`
		ComparisonReasons []string `json:"comparison_reasons"`
	} `json:"git_finding_delta"`
	AgentTasks []struct {
		FindingID string `json:"finding_id"`
	} `json:"agent_tasks"`
}

func testGitDeltaCheckBaseJSON(t *testing.T) {
	t.Parallel()
	const warnRule = `rules:
  - id: no-a-to-b
    type: forbidden_dependency
    gate: warn
    from: "pkg/a/**"
    to: "pkg/b/**"
`
	const failRule = `rules:
  - id: no-a-to-b
    type: forbidden_dependency
    gate: fail
    from: "pkg/a/**"
    to: "pkg/b/**"
`
	tests := []struct {
		name string
		// cfgBody selects the gate outcome; wantCode is the exit code the run
		// must produce with AND without --base (the delta is report-only).
		cfgBody string
		// wantIntroduced is the number of current repair tasks the base ref does
		// not carry. The head commit adds the only violating import, so a
		// blocking rule must attribute its task to this change.
		wantIntroduced int
		wantCode       int
	}{
		{name: "clean gate exits 0", cfgBody: coupledModulesCfg, wantCode: 0},
		{name: "blocking rule exits 1", cfgBody: coupledModulesCfg + failRule, wantIntroduced: 1, wantCode: 1},
		{name: "warning rule exits 2", cfgBody: coupledModulesCfg + warnRule, wantCode: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := gitDeltaFixtureRepo(t, tc.cfgBody)
			code, stdout, stderr := runArchfit(t, cmdCheck, flagBase, diffBaseRef, "--json", "-c", cfgPath)
			if code != tc.wantCode {
				t.Fatalf("check --base --json: exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
			}
			// Report-only: the same run without --base reaches the same verdict.
			if plain, _, _ := runArchfit(t, cmdCheck, "--json", "-c", cfgPath); plain != code {
				t.Errorf("--base changed the exit code: %d with, %d without", code, plain)
			}
			var got gitDeltaJSON
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, stdout)
			}
			if got.GitFindingDelta == nil {
				t.Fatalf("--base --json must emit git_finding_delta\n%s", stdout)
			}
			d := got.GitFindingDelta
			if d.BaseRef != diffBaseRef {
				t.Errorf("base_ref = %q, want %q", d.BaseRef, diffBaseRef)
			}
			if d.Introduced == nil || d.PreExisting == nil || d.UnknownOrigin == nil || d.ComparisonReasons == nil {
				t.Errorf("every git_finding_delta list must be a non-null array: %s", stdout)
			}
			if len(d.ComparisonReasons) != 0 {
				t.Errorf("both fixture sides compile; want no comparison_reasons, got %v", d.ComparisonReasons)
			}
			if len(d.Introduced) != tc.wantIntroduced {
				t.Errorf("introduced_finding_ids = %v, want %d entr(y|ies)", d.Introduced, tc.wantIntroduced)
			}
			// Every current repair task lands in exactly one origin bucket.
			total := len(d.Introduced) + len(d.PreExisting) + len(d.UnknownOrigin)
			if total != len(got.AgentTasks) {
				t.Errorf("origin buckets hold %d ids for %d agent_tasks", total, len(got.AgentTasks))
			}
			if len(d.UnknownOrigin) == 0 && d.ComparisonStatus != diagnostic.GitComparisonComparable {
				t.Errorf("comparison_status = %q with no unknown-origin task", d.ComparisonStatus)
			}
			if len(d.UnknownOrigin) > 0 && d.ComparisonStatus != diagnostic.GitComparisonUnknown {
				t.Errorf("comparison_status = %q with %d unknown-origin tasks", d.ComparisonStatus, len(d.UnknownOrigin))
			}
			// Isolation: the base worktree is deleted before output is read, so
			// no base-side path may appear anywhere in the head report.
			if strings.Contains(stdout, baseWorktreesDir(filepath.Dir(cfgPath))) {
				t.Errorf("head output leaked a base-worktree path: %s", stdout)
			}
		})
	}
}
