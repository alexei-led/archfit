package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/labels/labelsio"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// Erosion gates (CI), behavioural half. The structural half — no_scalar_decision
// and no_dead_archfit_rule — lives in internal/erosion_test.go, which also
// carries the name-to-owner table for all six checks.
//
// Each check here runs the real command over a fixture repository, because what
// it protects is what a user receives: the emitted state, the emitted comparison
// fingerprints, the accepted labels, and the written baseline.

// erosionState runs `archfit analyze --format json` over the fixture and
// decodes the architecture state a consumer would receive.
func erosionState(t *testing.T, cfgPath string) report.ArchitectureState {
	t.Helper()

	var buf bytes.Buffer
	if code := Run([]string{cmdAnalyze, "-c", cfgPath, "--format=" + formatJSON}, &buf); code != 0 {
		t.Fatalf("analyze: exit = %d\noutput:\n%s", code, buf.String())
	}
	var state report.ArchitectureState
	if err := json.Unmarshal(buf.Bytes(), &state); err != nil {
		t.Fatalf("decode architecture state: %v\noutput:\n%s", err, buf.String())
	}
	return state
}

// TestErosion_DimensionStatusRequired (dimension_status_required) asserts every
// run publishes all nine envelopes, each with an owner, a measurement status,
// and a confidence — and that a measured envelope actually says what it counted.
//
// A dimension with no status reads as an empty list, and an empty list reads as
// healthy. That is the exact substitution the state contract exists to prevent:
// "we found no coupling problems" and "nothing looked at coupling" must never
// serialise the same way.
func TestErosion_DimensionStatusRequired(t *testing.T) {
	t.Parallel()
	state := erosionState(t, writeCoupledRepo(t, coupledModulesCfg))

	dimensions := state.Dimensions.All()
	if len(dimensions) != report.DimensionCount {
		t.Fatalf("dimensions = %d, want %d", len(dimensions), report.DimensionCount)
	}
	for _, dim := range dimensions {
		for _, problem := range dimensionStatusProblems(dim) {
			t.Errorf("dimension %q: %s", dim.Name, problem)
		}
	}

	measured, partial, unmeasured := state.Dimensions.CountStatuses()
	if measured+partial+unmeasured != report.DimensionCount {
		t.Errorf("coverage counts %d/%d/%d do not sum to %d",
			measured, partial, unmeasured, report.DimensionCount)
	}
	if state.Coverage.Measured != measured || state.Coverage.Partial != partial ||
		state.Coverage.Unmeasured != unmeasured {
		t.Errorf("coverage block %+v disagrees with the envelopes (%d/%d/%d)",
			state.Coverage, measured, partial, unmeasured)
	}
}

// TestErosion_DimensionStatusRequiredFiresOnABlankEnvelope proves the predicate
// above reports the shapes it claims to reject.
func TestErosion_DimensionStatusRequiredFiresOnABlankEnvelope(t *testing.T) {
	t.Parallel()
	cases := map[string]report.DimensionState{
		"no status":                {Name: characterizationScoreDimension, Owner: report.OwnerCoupling, Confidence: report.ConfidenceUnrated},
		"no owner":                 {Name: characterizationScoreDimension, Status: report.MeasurementMeasured, Confidence: report.ConfidenceHigh},
		"no confidence":            {Name: characterizationScoreDimension, Owner: report.OwnerCoupling, Status: report.MeasurementMeasured},
		"measured without a basis": {Name: characterizationScoreDimension, Owner: report.OwnerCoupling, Status: report.MeasurementMeasured, Confidence: report.ConfidenceHigh},
		"unrated while measured": {Name: characterizationScoreDimension, Owner: report.OwnerCoupling, Status: report.MeasurementMeasured,
			Confidence: report.ConfidenceUnrated, Coverage: report.DimensionCoverage{Basis: "edges"}},
		"unmeasured without a reason": {Name: characterizationScoreDimension, Owner: report.OwnerCoupling,
			Status: report.MeasurementUnmeasured, Confidence: report.ConfidenceUnrated},
	}
	for name, dim := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if problems := dimensionStatusProblems(dim); len(problems) == 0 {
				t.Errorf("dimensionStatusProblems(%+v) = none, want the defect reported", dim)
			}
		})
	}

	healthy := report.DimensionState{
		Name: characterizationScoreDimension, Owner: report.OwnerCoupling, Status: report.MeasurementMeasured,
		Confidence: report.ConfidenceHigh, Coverage: report.DimensionCoverage{Basis: "edges scored", Observed: 3, Total: 4},
	}
	if problems := dimensionStatusProblems(healthy); len(problems) != 0 {
		t.Errorf("dimensionStatusProblems(measured envelope) = %v, want none", problems)
	}
}

// dimensionStatusProblems lists what one envelope fails to state. It is the
// single predicate behind both the real-run check and its fixtures, so the two
// cannot drift apart.
func dimensionStatusProblems(d report.DimensionState) []string {
	var out []string
	if d.Owner == "" {
		out = append(out, "no evidence owner: an envelope nobody owns cannot be measured by anyone")
	}
	switch d.Status {
	case report.MeasurementMeasured, report.MeasurementPartial:
		if d.Coverage.Basis == "" {
			out = append(out, "measured with no denominator basis: the numbers name no population")
		}
		if d.Confidence == report.ConfidenceUnrated {
			out = append(out, "measured but unrated: evidence was gathered, so it supports some confidence")
		}
	case report.MeasurementUnmeasured:
		if len(d.Unknown) == 0 {
			out = append(out, "unmeasured with no unknown fact: indistinguishable from an envelope nobody explained")
		}
	default:
		out = append(out, "no measurement status: an envelope with no status reads as an empty, healthy result")
	}
	if d.Confidence == "" {
		out = append(out, "no confidence: unrated is the abstention, an empty string is an omission")
	}
	return out
}

// TestErosion_ConfigHashRequired (config_hash_required) asserts every run
// publishes the fingerprints a later run needs to decide comparability.
//
// Without them nothing downstream can refuse a bogus comparison: a delta taken
// across a config edit would attribute a policy change to the code, which is
// the failure mode the strict four-fingerprint rule exists to make impossible.
func TestErosion_ConfigHashRequired(t *testing.T) {
	t.Parallel()
	state := erosionState(t, writeCoupledRepo(t, coupledModulesCfg))

	for _, problem := range comparisonFingerprintProblems(state.Comparison) {
		t.Errorf("comparison block: %s", problem)
	}
}

// TestErosion_ConfigHashRequiredFiresOnAMissingFingerprint proves the predicate
// reports an unqualifiable comparison.
func TestErosion_ConfigHashRequiredFiresOnAMissingFingerprint(t *testing.T) {
	t.Parallel()
	complete := report.StateComparison{
		Status: report.ComparisonNotRequested, ConfigHash: "cfg", ModelHash: "model-hash",
		RubricVersion: report.ScoreVersion,
	}
	if problems := comparisonFingerprintProblems(complete); len(problems) != 0 {
		t.Errorf("comparisonFingerprintProblems(complete) = %v, want none", problems)
	}

	for _, blank := range []report.StateComparison{
		{Status: complete.Status, ModelHash: complete.ModelHash, RubricVersion: complete.RubricVersion},
		{Status: complete.Status, ConfigHash: complete.ConfigHash, RubricVersion: complete.RubricVersion},
		{Status: complete.Status, ConfigHash: complete.ConfigHash, ModelHash: complete.ModelHash},
		{ConfigHash: complete.ConfigHash, ModelHash: complete.ModelHash, RubricVersion: complete.RubricVersion},
	} {
		if problems := comparisonFingerprintProblems(blank); len(problems) == 0 {
			t.Errorf("comparisonFingerprintProblems(%+v) = none, want the missing fingerprint reported", blank)
		}
	}
}

// comparisonFingerprintProblems lists the comparability facts a state fails to
// publish.
//
// labels_hash is deliberately absent: it is empty exactly when no label is
// approved, and empty compares equal to empty, so a repository with no labels
// is comparable against another with none. A malformed labels file never
// reaches here — it is exit 3 before any report exists.
func comparisonFingerprintProblems(c report.StateComparison) []string {
	var out []string
	if c.ConfigHash == "" {
		out = append(out, "no config_hash: a later run cannot tell whether the policy moved under it")
	}
	if c.ModelHash == "" {
		out = append(out, "no model_hash: a module rename would read as one resolved seam plus one new seam")
	}
	if c.RubricVersion == "" {
		out = append(out, "no rubric_version: scores from two different formulas would subtract cleanly")
	}
	if c.Status == "" {
		out = append(out, "no comparison status: the run does not say whether it compared anything")
	}
	return out
}

// TestErosion_LabelEvidenceRequired (label_evidence_required) asserts every
// approved label in this repository's own label file carries the evidence its
// approval rests on.
//
// An approved label overrides measured integration strength, which moves seam
// severity and distributed-monolith qualification. One with no evidence hash can
// never go stale — the freshness check is skipped for it — so it silences a seam
// permanently, and nothing in the report says a human decision is doing it.
func TestErosion_LabelEvidenceRequired(t *testing.T) {
	t.Parallel()
	for i, problem := range labelEvidenceProblems(loadRepoLabels(t)) {
		t.Errorf("label %d: %s", i, problem)
	}
}

// TestErosion_LabelEvidenceRequiredFiresOnAnUnevidencedApproval proves the
// predicate rejects the entry it exists to reject, and accepts a complete one.
func TestErosion_LabelEvidenceRequiredFiresOnAnUnevidencedApproval(t *testing.T) {
	t.Parallel()
	complete := labels.Label{
		From: "a", To: "b", Strength: "contract", Status: labels.StatusApproved,
		EvidenceHash: "hash", Rationale: "reviewed the four imports; all cross the declared public surface",
		Provenance: labels.ProvenanceHuman, Confidence: labels.ConfidenceHigh,
	}
	if problems := labelEvidenceProblems([]labels.Label{complete}); len(problems) != 0 {
		t.Errorf("labelEvidenceProblems(complete approval) = %v, want none", problems)
	}

	// A draft is inert: it changes nothing measurable, so it owes no evidence.
	draft := complete
	draft.Status, draft.EvidenceHash, draft.Rationale = labels.StatusDraft, "", ""
	if problems := labelEvidenceProblems([]labels.Label{draft}); len(problems) != 0 {
		t.Errorf("labelEvidenceProblems(draft) = %v, want none — a draft overrides nothing", problems)
	}

	for name, mutate := range map[string]func(*labels.Label){
		"no evidence hash": func(l *labels.Label) { l.EvidenceHash = "" },
		"no rationale":     func(l *labels.Label) { l.Rationale = "" },
		"no provenance":    func(l *labels.Label) { l.Provenance = "" },
		"no confidence":    func(l *labels.Label) { l.Confidence = "" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			broken := complete
			mutate(&broken)
			if problems := labelEvidenceProblems([]labels.Label{broken}); len(problems) == 0 {
				t.Errorf("labelEvidenceProblems(%+v) = none, want the missing evidence reported", broken)
			}
		})
	}
}

// labelEvidenceProblems lists what each approved label fails to record. Draft
// entries are skipped: the gate never consumes them.
func labelEvidenceProblems(lbls []labels.Label) []string {
	var out []string
	for _, l := range lbls {
		if l.Status != labels.StatusApproved {
			continue
		}
		pair := l.From + " -> " + l.To
		if l.EvidenceHash == "" {
			out = append(out, pair+": no evidence_hash — the override can never go stale, so it silences the pair permanently")
		}
		if strings.TrimSpace(l.Rationale) == "" {
			out = append(out, pair+": no rationale — an approval with no stated reason cannot be re-reviewed")
		}
		if l.Provenance == "" {
			out = append(out, pair+": no provenance — human, tool, and llm judgments carry different precedence and confidence")
		}
		if l.Confidence == "" {
			out = append(out, pair+": no confidence — the coupling envelope cannot lower its own rating for a weak override")
		}
	}
	return out
}

// loadRepoLabels reads this repository's own label file through the real
// loader, so the check runs against exactly what a run would consume.
func loadRepoLabels(t *testing.T) []labels.Label {
	t.Helper()

	path := filepath.Join("..", "..", ".archfit-labels.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	lbls, err := labelsio.Load(path)
	if err != nil {
		t.Fatalf("load %s: %v — a malformed label file is exit 3, never a silent partial read", path, err)
	}
	return lbls
}

// TestErosion_BaselineIdempotent (baseline_idempotent) asserts a capture over an
// unchanged tree writes the same bytes twice.
//
// A capture that reads the file it is about to overwrite is self-referential:
// accepting a rolled-up advisory splits its group on the next run and exposes
// the siblings as new representatives, so the file never settles and every CI
// run reports drift that is not there.
func TestErosion_BaselineIdempotent(t *testing.T) {
	TestRun_Baseline_IsIdempotent(t)
	baselineCaptureIgnoresItsOwnFile(t)
}

// baselineCaptureIgnoresItsOwnFile is this gate's paired violating-input
// fixture, and it drives the defect directly rather than the symptom.
//
// TestRun_Baseline_IsIdempotent compares two captures from the SAME starting
// state, so it has two vacuous passes available to it: a capture that accepts
// nothing writes two identical empty files, and a capture that is stable only
// because nothing perturbed it proves nothing about self-reference. This
// fixture removes both. It asserts the capture is non-vacuous, then perturbs
// exactly the input the pre-fix code read — the baseline file itself — and
// requires the same bytes back. Pre-fix, dropping an accepted representative
// split its rolled-up group and exposed the siblings, so this input produced a
// different file; post-fix the capture cannot see it at all.
func baselineCaptureIgnoresItsOwnFile(t *testing.T) {
	t.Helper()
	cfgPath := writeCoupledRepo(t, distributedMonolithCfg)
	path := filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath)

	capture := func() []byte {
		t.Helper()
		var buf bytes.Buffer
		if code := Run([]string{cmdBaseline, "-c", cfgPath}, &buf); code != 0 {
			t.Fatalf("baseline: exit = %d\noutput:\n%s", code, buf.String())
		}
		data, err := os.ReadFile(path) //nolint:gosec // path derives from t.TempDir()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	clean := capture()
	stored, err := baseline.Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Accepted) == 0 {
		t.Fatal("fixture regression: the capture accepted nothing, so two empty files would compare equal and the gate would pass vacuously")
	}

	stored.Accepted = stored.Accepted[1:]
	if err := baseline.Save(context.Background(), path, stored); err != nil {
		t.Fatal(err)
	}
	perturbed, err := os.ReadFile(path) //nolint:gosec // path derives from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(perturbed, clean) {
		t.Fatal("fixture regression: dropping an accepted entry did not change the file, so the perturbation tests nothing")
	}

	if got := capture(); !bytes.Equal(got, clean) {
		t.Errorf("capture over an unchanged tree depends on the baseline it overwrites\nwant:\n%s\ngot:\n%s", clean, got)
	}
}
