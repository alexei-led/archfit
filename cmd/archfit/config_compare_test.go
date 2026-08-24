package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
)

// TestConfigCompare covers `archfit config compare`: the identity result, a
// candidate that measures less of the same tree, isolation from the accepted
// baseline, gate-vs-advisory finding identity, the protected-file contract, and
// the exit codes.
//
// One exported test function by design — cmd/archfit sits at its public_api_max
// ceiling, so new coverage arrives as subtests, never as new exported names.
func TestConfigCompare(t *testing.T) {
	t.Parallel()
	t.Run("identity", testCompareIdentity)
	t.Run("candidate_outside_tree", testCompareCandidateOutsideTree)
	t.Run("measurement_loss", testCompareMeasurementLoss)
	t.Run("baseline_isolation", testCompareBaselineIsolation)
	t.Run("advisory_promotion", testCompareAdvisoryPromotion)
	t.Run("protected_files", testCompareProtectedFiles)
	t.Run("exit_codes", testCompareExitCodes)
	t.Run("stderr_side_labels", testCompareStderrSideLabels)
	t.Run("score_line", testCompareScoreLine)
	t.Run("id_preview", testCompareIDPreview)
	t.Run("classification_mix", testCompareClassificationMix)
}

// testCompareClassificationMix pins that a classification shift is reported.
//
// It is the shape `config compare` exists to evaluate and the one it used to
// call identical: an `owner:` or `deploy_unit:` edit moves edges between
// distance rungs without changing how many were scored, and with
// balance = max(|S−D|, 10−V) the 10−V term dominates for every low-volatility
// target, so neither the four edge counters nor the rounded score moves.
func testCompareClassificationMix(t *testing.T) {
	t.Parallel()
	summary := func(mutate func(*result.ClassifiedEdgeSummary)) *result.ClassifiedEdgeSummary {
		s := &result.ClassifiedEdgeSummary{
			Total: 1, Scored: 1,
			ByStrength:      map[string]int{enrichFunctional: 1},
			ByDistance:      map[string]int{"cross_module_different_owner": 1},
			ByDistanceBasis: map[string]int{"ownership": 1},
			ByVolatility:    map[string]int{volatilityLow: 1},
			VolatilityProvenance: &result.VolatilityProvenance{
				Declared: 2, Undeclared: 0,
			},
		}
		mutate(s)
		return s
	}
	unchanged := func(*result.ClassifiedEdgeSummary) {}

	tests := []struct {
		name       string
		mutate     func(*result.ClassifiedEdgeSummary)
		wantSubstr string
	}{
		{name: "identical mix reports nothing", mutate: unchanged},
		{
			name: "owner edit moves the distance rung",
			mutate: func(s *result.ClassifiedEdgeSummary) {
				s.ByDistance = map[string]int{"cross_module_same_owner": 1}
			},
			wantSubstr: "distance mix: cross_module_different_owner 1 → 0, cross_module_same_owner 0 → 1",
		},
		{
			name: "volatility provenance moves",
			mutate: func(s *result.ClassifiedEdgeSummary) {
				s.VolatilityProvenance = &result.VolatilityProvenance{Declared: 1, Undeclared: 1}
			},
			wantSubstr: "volatility provenance (modules): declared 2 → 1, undeclared 0 → 1",
		},
		{
			name: "strength mix moves",
			mutate: func(s *result.ClassifiedEdgeSummary) {
				s.ByStrength = map[string]int{"contract": 1}
			},
			wantSubstr: "strength mix: contract 0 → 1, functional 1 → 0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lines := classificationMixLines(summary(unchanged), summary(tc.mutate))
			if tc.wantSubstr == "" {
				if len(lines) != 0 {
					t.Fatalf("an unchanged mix must render no line, got %v", lines)
				}
				return
			}
			if len(lines) == 0 {
				t.Fatal("a moved classification mix must render a line")
			}
			if !slices.Contains(lines, tc.wantSubstr) {
				t.Errorf("lines %v do not carry %q", lines, tc.wantSubstr)
			}
		})
	}

	t.Run("nil summaries report nothing", func(t *testing.T) {
		t.Parallel()
		if lines := classificationMixLines(nil, nil); len(lines) != 0 {
			t.Errorf("two unclassified sides must render no line, got %v", lines)
		}
	})
}

// testCompareIDPreview pins the text-report cap: short lists print in full, long
// ones are truncated with a pointer to the complete JSON list.
func testCompareIDPreview(t *testing.T) {
	t.Parallel()
	ids := make([]string, 0, compareIDPreview+3)
	for i := 0; i < cap(ids); i++ {
		ids = append(ids, fmt.Sprintf("id%02d", i))
	}
	full := summariseIDs(ids[:compareIDPreview])
	if strings.Contains(full, "more") || !strings.Contains(full, ids[compareIDPreview-1]) {
		t.Errorf("a list at the cap must print in full, got %q", full)
	}
	capped := summariseIDs(ids)
	if !strings.Contains(capped, "3 more") {
		t.Errorf("a list over the cap must name the remainder, got %q", capped)
	}
	if strings.Contains(capped, ids[compareIDPreview]) {
		t.Errorf("a capped list must not print the truncated ids, got %q", capped)
	}
}

const cmdCompare = "compare"

// narrowModuleCfg declares only module a, so the fixture's pkg/a → pkg/b edge
// leaves the declared module set entirely: it becomes an external edge, excluded
// from coupling_balance. Measuring less of the same tree is what the
// measurement-loss warnings exist to expose.
const narrowModuleCfg = `version: 1
modules:
  a:
    paths: ["pkg/a/**"]
    owner: team-a
`

// compareDoc decodes the archfit.config-compare.v1 document. Pointer fields
// distinguish JSON null (unknown) from a zero value.
type compareDoc struct {
	SchemaVersion string          `json:"schema_version"`
	Current       compareSideJSON `json:"current"`
	Candidate     compareSideJSON `json:"candidate"`
	ScoreDelta    *int            `json:"score_delta"`
	Findings      struct {
		CurrentOnly   []string `json:"current_only_ids"`
		CandidateOnly []string `json:"candidate_only_ids"`
		Both          []string `json:"both_ids"`
	} `json:"findings"`
	Coverage struct {
		Status  string `json:"status"`
		Details []struct {
			Tool      string `json:"tool"`
			Current   string `json:"current"`
			Candidate string `json:"candidate"`
			Status    string `json:"status"`
			Reason    string `json:"reason"`
		} `json:"details"`
	} `json:"coverage"`
	Warnings []struct {
		Code string `json:"code"`
		Text string `json:"text"`
	} `json:"warnings"`
}

type compareSideJSON struct {
	ConfigHash string `json:"config_hash"`
	Scorecard  struct {
		Overall     int    `json:"overall"`
		OverallBand string `json:"overall_band"`
	} `json:"scorecard"`
	ClassifiedEdges *struct {
		Total     int `json:"total"`
		Scored    int `json:"scored"`
		Abstained int `json:"abstained"`
		External  int `json:"external"`
	} `json:"classified_edges"`
	FindingCount int `json:"finding_count"`
}

func (d compareDoc) warningCodes() []string {
	codes := make([]string, 0, len(d.Warnings))
	for _, w := range d.Warnings {
		codes = append(codes, w.Code)
	}
	return codes
}

// runCompareJSON runs `config compare <candidate> --json -c <current>` and
// decodes stdout. It fails the test on any non-zero exit — a comparison that
// succeeded must exit 0 no matter how large the difference is.
func runCompareJSON(t *testing.T, currentCfg, candidateCfg string) compareDoc {
	t.Helper()
	code, stdout, stderr := runArchfit(t, cmdConfig, cmdCompare, candidateCfg, flagJSON, "-c", currentCfg)
	if code != 0 {
		t.Fatalf("config compare --json: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var doc compareDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	return doc
}

// writeCandidateCfg writes a candidate config into its own directory, outside
// the analyzed repo, and returns its path.
func writeCandidateCfg(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "candidate.archfit.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testCompareIdentity(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)

	code, stdout, stderr := runArchfit(t, cmdConfig, cmdCompare, cfgPath, "-c", cfgPath)
	if code != 0 {
		t.Fatalf("config compare: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, configCompareIdentityLine) {
		t.Errorf("identity run must say %q:\n%s", configCompareIdentityLine, stdout)
	}
	if !strings.Contains(stdout, "coverage evidence: ") {
		t.Errorf("text report must carry a coverage-evidence grade:\n%s", stdout)
	}
	// The coverage grade must stay in its own section: it qualifies everything
	// below it, and an identity run can legitimately grade below `comparable`
	// (an analyzer absent or disabled on both sides) while still reporting no
	// measurement differences. The two must not share a line, or they read as a
	// contradiction.
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, configCompareIdentityLine) && strings.Contains(line, "coverage") {
			t.Errorf("coverage grade must not share the identity line: %q", line)
		}
	}

	doc := runCompareJSON(t, cfgPath, cfgPath)
	if doc.SchemaVersion != configCompareSchemaVersion {
		t.Errorf("schema_version = %q, want %q", doc.SchemaVersion, configCompareSchemaVersion)
	}
	if len(doc.Findings.CurrentOnly) != 0 || len(doc.Findings.CandidateOnly) != 0 {
		t.Errorf("identity run reported one-sided findings: %+v", doc.Findings)
	}
	if doc.Findings.CurrentOnly == nil || doc.Findings.CandidateOnly == nil ||
		doc.Findings.Both == nil || doc.Warnings == nil || doc.Coverage.Details == nil {
		t.Errorf("every list must be a non-null array: %+v", doc)
	}
	// Equal coverage gaps on both sides are shared blindness, not measurement
	// loss: an identity run may grade comparable_with_gaps and must still raise
	// no warning.
	if len(doc.Warnings) != 0 {
		t.Errorf("identity run must raise no measurement-loss warning, got %v", doc.warningCodes())
	}
	if doc.Current.ConfigHash == "" || doc.Current.ConfigHash != doc.Candidate.ConfigHash {
		t.Errorf("same config file must hash identically on both sides: %q vs %q",
			doc.Current.ConfigHash, doc.Candidate.ConfigHash)
	}
	if doc.ScoreDelta == nil || *doc.ScoreDelta != 0 {
		t.Errorf("score_delta = %v, want 0 for a measured identity run", doc.ScoreDelta)
	}
	// finding_count is the distinct observed population, i.e. the buckets. The
	// fixture must actually produce shared findings, or an empty BothIDs would
	// satisfy the two count checks below with nothing measured.
	if len(doc.Findings.Both) == 0 {
		t.Fatalf("fixture regression: an identity run must report shared findings: %+v", doc.Findings)
	}
	if want := len(doc.Findings.CurrentOnly) + len(doc.Findings.Both); doc.Current.FindingCount != want {
		t.Errorf("current.finding_count = %d, want %d", doc.Current.FindingCount, want)
	}
	if want := len(doc.Findings.CandidateOnly) + len(doc.Findings.Both); doc.Candidate.FindingCount != want {
		t.Errorf("candidate.finding_count = %d, want %d", doc.Candidate.FindingCount, want)
	}
}

// testCompareCandidateOutsideTree pins the wiring rule that a same-directory
// identity run cannot catch: only ConfigSource differs between the two sides.
// If the candidate's own directory became the bundle directory, runContext would
// fall back to it as the scan root and the candidate side would measure an empty
// tree — with nothing in the output to reveal it.
func testCompareCandidateOutsideTree(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)
	candidatePath := writeCandidateCfg(t, coupledModulesCfg)

	doc := runCompareJSON(t, cfgPath, candidatePath)
	if doc.Current.ClassifiedEdges == nil || doc.Candidate.ClassifiedEdges == nil {
		t.Fatalf("both sides must classify edges: %+v", doc)
	}
	if doc.Current.ClassifiedEdges.Total == 0 {
		t.Fatalf("fixture produced no classified edges: %+v", doc.Current.ClassifiedEdges)
	}
	if doc.Current.ClassifiedEdges.Total != doc.Candidate.ClassifiedEdges.Total {
		t.Errorf("a candidate outside the repo must measure the SAME tree: current total %d, candidate total %d",
			doc.Current.ClassifiedEdges.Total, doc.Candidate.ClassifiedEdges.Total)
	}
	if len(doc.Findings.CurrentOnly) != 0 || len(doc.Findings.CandidateOnly) != 0 {
		t.Errorf("identical config bodies must produce identical findings: %+v", doc.Findings)
	}
	if doc.Current.ConfigHash != doc.Candidate.ConfigHash {
		t.Errorf("identical config bytes must hash identically: %q vs %q",
			doc.Current.ConfigHash, doc.Candidate.ConfigHash)
	}
}

func testCompareMeasurementLoss(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)
	candidatePath := writeCandidateCfg(t, narrowModuleCfg)

	doc := runCompareJSON(t, cfgPath, candidatePath)
	if doc.Candidate.ClassifiedEdges == nil || doc.Current.ClassifiedEdges == nil {
		t.Fatalf("both sides must classify edges: %+v", doc)
	}
	if doc.Candidate.ClassifiedEdges.External <= doc.Current.ClassifiedEdges.External {
		t.Fatalf("dropping a module must push its edges out to external: current %d, candidate %d",
			doc.Current.ClassifiedEdges.External, doc.Candidate.ClassifiedEdges.External)
	}
	if !slices.Contains(doc.warningCodes(), decision.WarnExternalEdgesRose) {
		t.Errorf("measuring less must warn %q, got %v", decision.WarnExternalEdgesRose, doc.warningCodes())
	}

	// The text report must show the difference and every warning, and must never
	// call the candidate better.
	code, stdout, stderr := runArchfit(t, cmdConfig, cmdCompare, candidatePath, "-c", cfgPath)
	if code != 0 {
		t.Fatalf("a comparison with differences still exits 0: exit = %d\nstderr:\n%s", code, stderr)
	}
	if strings.Contains(stdout, configCompareIdentityLine) {
		t.Errorf("differing configs must not report the identity line:\n%s", stdout)
	}
	if !strings.Contains(stdout, decision.WarnExternalEdgesRose) {
		t.Errorf("text report must list the measurement-loss warning:\n%s", stdout)
	}
	// The no-ranking guardrail is the footer itself: it is the only line that
	// tells a reader a higher candidate score is not a better configuration.
	if !strings.Contains(stdout, configCompareFooter) {
		t.Errorf("the report must carry the no-ranking footer:\n%s", stdout)
	}
}

// testCompareBaselineIsolation pins the guardrail that `config compare` reads
// raw measurement: an accepted baseline that demonstrably changes the gate must
// not change the comparison document at all.
func testCompareBaselineIsolation(t *testing.T) {
	t.Parallel()
	const failRule = `rules:
  - id: no-a-to-b
    type: forbidden_dependency
    gate: fail
    from: "pkg/a/**"
    to: "pkg/b/**"
`
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+failRule)

	// Both compare runs must genuinely succeed: two failed runs would both print
	// nothing and make the equality check below pass vacuously.
	code, before, stderr := runArchfit(t, cmdConfig, cmdCompare, cfgPath, flagJSON, "-c", cfgPath)
	if code != 0 || strings.TrimSpace(before) == "" {
		t.Fatalf("config compare before baselining: exit = %d, output %q\nstderr:\n%s", code, before, stderr)
	}

	if code, _, stderr := runArchfit(t, cmdCheck, "-c", cfgPath); code != 1 {
		t.Fatalf("fixture must fail the gate before baselining: exit = %d\nstderr:\n%s", code, stderr)
	}
	if code, stdout, stderr := runArchfit(t, cmdBaseline, "-c", cfgPath); code != 0 {
		t.Fatalf("baseline: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	// The baseline is load-bearing: it silences the violation for the gate.
	if code, _, stderr := runArchfit(t, cmdCheck, "-c", cfgPath); code != 0 {
		t.Fatalf("baseline did not accept the finding: check exit = %d\nstderr:\n%s", code, stderr)
	}

	afterCode, after, afterErr := runArchfit(t, cmdConfig, cmdCompare, cfgPath, flagJSON, "-c", cfgPath)
	if afterCode != 0 {
		t.Fatalf("config compare after baselining: exit = %d\nstderr:\n%s", afterCode, afterErr)
	}
	if after != before {
		t.Errorf("the accepted baseline changed the comparison document\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// testCompareAdvisoryPromotion pins the finding rule: the coupling gate promotes
// Balanced-Coupling advisories to gate kind, but the ID does not move, so the
// same seam must land in both_ids rather than split across the one-sided buckets.
func testCompareAdvisoryPromotion(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+"coupling:\n  gate:\n    min_band: strong\n")
	candidatePath := writeCandidateCfg(t, coupledModulesCfg)

	// The premise is load-bearing and invisible in the compare document, which
	// carries no finding kinds: if the gate stopped tripping, this test would
	// silently degrade into a duplicate of testCompareCandidateOutsideTree.
	if code, _, stderr := runArchfit(t, cmdCheck, "-c", cfgPath); code != 1 {
		t.Fatalf("fixture regression: the coupling gate must trip and promote the advisory: check exit = %d\nstderr:\n%s", code, stderr)
	}

	doc := runCompareJSON(t, cfgPath, candidatePath)
	if len(doc.Findings.Both) == 0 {
		t.Fatalf("fixture produced no shared findings: %+v", doc.Findings)
	}
	if len(doc.Findings.CurrentOnly) != 0 || len(doc.Findings.CandidateOnly) != 0 {
		t.Errorf("gate/advisory promotion must not split a finding by kind: %+v", doc.Findings)
	}
	if doc.ScoreDelta == nil || *doc.ScoreDelta != 0 {
		t.Errorf("a gate knob must not move the measured score: score_delta = %v", doc.ScoreDelta)
	}
}

// testCompareStderrSideLabels pins the file-header invariant: both sides run
// the same pipeline over one stderr stream, so every degradation notice must
// say which configuration produced it. The fixture puts the config bundle in a
// subdirectory of --root, which makes both sides emit the output-inside-root
// warning — one warning, twice, distinguishable only by its label.
func testCompareStderrSideLabels(t *testing.T) {
	t.Parallel()
	repoCfg := writeCoupledRepo(t, coupledModulesCfg)
	// The scope resolver canonicalises the scan root, so the test must hand it a
	// canonical path too — on macOS t.TempDir() is a symlinked /var/folders path
	// and the bundle would compute as outside the root.
	repoDir, err := filepath.EvalSymlinks(filepath.Dir(repoCfg))
	if err != nil {
		t.Fatal(err)
	}
	repoCfg = filepath.Join(repoDir, defaultConfigPath)
	bundleDir := filepath.Join(repoDir, "cfg")
	if err := os.MkdirAll(bundleDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(bundleDir, defaultConfigPath)
	if err := os.Rename(repoCfg, cfgPath); err != nil {
		t.Fatal(err)
	}
	candidatePath := writeCandidateCfg(t, coupledModulesCfg)

	code, _, stderr := runArchfit(t, cmdConfig, cmdCompare, candidatePath, "-c", cfgPath, "--root", repoDir)
	if code != 0 {
		t.Fatalf("config compare: exit = %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "output written inside analyzed root") {
		t.Fatalf("fixture regression: the bundle directory must sit inside --root\nstderr:\n%s", stderr)
	}
	for _, label := range []string{compareLabelCurrent, compareLabelCandidate} {
		if !strings.Contains(stderr, "warning: "+label) {
			t.Errorf("stderr carries no %q-labelled warning:\n%s", label, stderr)
		}
	}
}

// testCompareProtectedFiles pins the report-only contract: every file in the
// analyzed tree, plus the candidate outside it, stays byte-identical. The fact
// cache under .archfit-cache is the only permitted write.
func testCompareProtectedFiles(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)
	repoDir := filepath.Dir(cfgPath)
	candidatePath := writeCandidateCfg(t, narrowModuleCfg)

	// Baseline and labels files must be present to be provably untouched.
	baselinePath := filepath.Join(repoDir, defaultBaselinePath)
	if code, _, stderr := runArchfit(t, cmdBaseline, "-c", cfgPath); code != 0 {
		t.Fatalf("baseline: exit = %d\nstderr:\n%s", code, stderr)
	}
	labelsPath := filepath.Join(repoDir, defaultLabelsPath)
	if err := os.WriteFile(labelsPath, []byte("version: 1\nlabels: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The candidate's own directory is checked by ENTRY, not just by file
	// content: if the bundle directory were ever derived from the candidate
	// path, a .archfit-cache or a baseline would appear beside it — and
	// snapshotTree deliberately skips the cache, so only a directory listing
	// catches that.
	candidateDir := filepath.Dir(candidatePath)
	beforeEntries := dirEntryNames(t, candidateDir)

	before := snapshotTree(t, repoDir)
	before[candidatePath] = readFileForCompare(t, candidatePath)

	if code, _, stderr := runArchfit(t, cmdConfig, cmdCompare, candidatePath, "-c", cfgPath); code != 0 {
		t.Fatalf("config compare: exit = %d\nstderr:\n%s", code, stderr)
	}

	if got := dirEntryNames(t, candidateDir); !slices.Equal(got, beforeEntries) {
		t.Errorf("config compare wrote beside the candidate: %v, want %v", got, beforeEntries)
	}
	after := snapshotTree(t, repoDir)
	after[candidatePath] = readFileForCompare(t, candidatePath)
	for _, protected := range []string{cfgPath, baselinePath, labelsPath, candidatePath} {
		if before[protected] == "" {
			t.Fatalf("protected file %s was missing before the run", protected)
		}
		if after[protected] != before[protected] {
			t.Errorf("config compare modified %s", protected)
		}
	}
	for path, content := range before {
		if after[path] != content {
			t.Errorf("config compare modified %s", path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("config compare created %s outside the fact cache", path)
		}
	}
}

// dirEntryNames lists dir's immediate entries, sorted. Unlike snapshotTree it
// hides nothing, so a stray .archfit-cache is visible.
func dirEntryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// snapshotTree reads every file under dir except the git directory and the fact
// cache, which the run is allowed to write.
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == ".archfit-cache" {
				return filepath.SkipDir
			}
			return nil
		}
		out[path] = readFileForCompare(t, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

func readFileForCompare(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //#nosec G304 -- test-owned temp path
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func testCompareExitCodes(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)
	missing := filepath.Join(t.TempDir(), "absent.archfit.yaml")
	invalid := writeCandidateCfg(t, "version: 1\nrules:\n  - id: bogus\n    type: not_a_rule_type\n")

	tests := []struct {
		name      string
		current   string
		candidate string
		wantCode  int
	}{
		{name: "difference still exits 0", current: cfgPath, candidate: writeCandidateCfg(t, narrowModuleCfg), wantCode: 0},
		{name: "missing candidate", current: cfgPath, candidate: missing, wantCode: 3},
		{name: "missing current config", current: missing, candidate: cfgPath, wantCode: 3},
		{name: "invalid candidate", current: cfgPath, candidate: invalid, wantCode: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := runArchfit(t, cmdConfig, cmdCompare, tc.candidate, "-c", tc.current)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
			}
			if tc.wantCode != 3 {
				return
			}
			if !strings.Contains(stderr, "error: ") {
				t.Errorf("an input error must be explained on stderr:\n%s", stderr)
			}
			if tc.candidate == missing && strings.Contains(stderr, "config init") {
				t.Errorf("a missing candidate must not suggest scaffolding a config:\n%s", stderr)
			}
		})
	}
}

// testCompareScoreLine pins the nil-delta split: unmeasured on both sides is not
// a difference, unmeasured on exactly one side is.
func testCompareScoreLine(t *testing.T) {
	t.Parallel()
	measured := func(v int) score.Scorecard {
		return score.Scorecard{Overall: v, OverallBand: score.BandMixed}
	}
	unmeasured := score.Scorecard{OverallBand: score.BandNA}
	delta := func(v int) *int { return &v }

	tests := []struct {
		name        string
		current     score.Scorecard
		candidate   score.Scorecard
		delta       *int
		wantChanged bool
		wantSubstr  string
	}{
		{name: "equal measured scores", current: measured(71), candidate: measured(71), delta: delta(0)},
		{name: "score moved", current: measured(71), candidate: measured(64), delta: delta(-7), wantChanged: true, wantSubstr: "-7"},
		{name: "candidate unmeasured", current: measured(71), candidate: unmeasured, wantChanged: true, wantSubstr: "delta unknown"},
		{name: "current unmeasured", current: unmeasured, candidate: measured(71), wantChanged: true, wantSubstr: scoreUnmeasured},
		{name: "both unmeasured", current: unmeasured, candidate: unmeasured},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			line, changed := scoreCompareLine(tc.current, tc.candidate, tc.delta)
			if changed != tc.wantChanged {
				t.Fatalf("changed = %v, want %v (line %q)", changed, tc.wantChanged, line)
			}
			if !changed {
				if line != "" {
					t.Errorf("an unchanged score must render no line, got %q", line)
				}
				return
			}
			if !strings.Contains(line, tc.wantSubstr) {
				t.Errorf("line %q does not carry %q", line, tc.wantSubstr)
			}
		})
	}
}
