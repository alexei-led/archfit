// Package main — `archfit config compare <candidate>`.
//
// Two configurations, one source tree, two full measurements, one report-only
// difference. The pure comparison lives in internal/decision (CompareConfigs);
// this file owns the composition (two pipeline runs) and the two output shapes.
//
// Invariants:
//   - Report-only. Exit 0 after a successful comparison, exit 3 on an input or
//     runtime error. Findings never move the exit code.
//   - Both sides run with an EMPTY accepted baseline: the comparison is about raw
//     measurement, and .archfit-baseline.json records findings accepted under the
//     CURRENT config only — applying it would silence the candidate's findings by
//     the current config's history.
//   - Both sides share the current config's bundle directory (pinned labels, fact
//     cache) and one evaluation instant. Only ConfigSource differs, so a candidate
//     file living in another directory can never move the analysis boundary.
//   - Nothing is written: config, baseline, labels, candidate, and policy files
//     stay byte-identical. Normal content-addressed fact-cache reads and writes
//     are the only filesystem effect.
//   - No output states that the candidate configuration is better. A higher score
//     under a configuration that measured less of the tree is a measurement loss,
//     which is what the warnings block is for.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/decision"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/score"
)

// configCompareSchemaVersion versions the `config compare --json` document.
const configCompareSchemaVersion = "archfit.config-compare.v1"

// Stderr warning labels. Both sides run the same pipeline over one stderr
// stream, so every degradation notice says which configuration produced it.
const (
	compareLabelCurrent   = "[current] "
	compareLabelCandidate = "[candidate] "
)

// scoreUnmeasured is how an unmeasured (band n/a) side renders in the text
// report. Never a number: coupling_balance abstains rather than invent one.
const scoreUnmeasured = "n/a"

// CompareCmd measures one source tree under two configurations and reports the
// difference. It is report-only and never writes.
type CompareCmd struct {
	Candidate string `arg:"" help:"Candidate config file to measure against the current one."`
	Config    string `short:"c" help:"Current config file path." default:".archfit.yaml"`
	Root      string `help:"Repository root to analyze (default: directory of --config)." type:"path"`
	JSON      bool   `name:"json" help:"Emit the comparison as a JSON document."`
}

func (c *CompareCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	// Load and validate BOTH configurations before any pipeline runs, so an
	// unreadable or invalid candidate costs no analysis time.
	curCfg, err := loadAnalysisConfig(ctx, c.Config)
	if err != nil {
		return configLoadError(err)
	}
	candCfg, err := loadCandidateConfig(ctx, c.Candidate)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// One run context for both sides. The candidate swaps ONLY ConfigSource:
	// BundleDir and ScanRoot stay the current run's, so both sides measure the
	// same tree with the same labels and fact cache even when the candidate file
	// lives elsewhere. Sharing EvaluatedAt keeps waiver expiry and staleness from
	// differing just because the second pipeline started later.
	curRC := runContext{
		ConfigSource: c.Config,
		BundleDir:    filepath.Dir(c.Config),
		ScanRoot:     c.Root,
		EvaluatedAt:  time.Now(),
	}
	candRC := curRC
	candRC.ConfigSource = c.Candidate

	current, err := runCompareSide(ctx, deps, curCfg, curRC, compareLabelCurrent)
	if err != nil {
		return err
	}
	candidate, err := runCompareSide(ctx, deps, candCfg, candRC, compareLabelCandidate)
	if err != nil {
		return err
	}

	result := decision.CompareConfigs(decision.ConfigCompareInput{Current: current, Candidate: candidate})
	if c.JSON {
		return writeConfigCompareJSON(deps.Stdout, buildConfigCompareDoc(current, candidate, result))
	}
	renderConfigCompareText(deps.Stdout, c.Config, c.Candidate, current, candidate, result)
	return nil
}

// loadCandidateConfig loads and validates the candidate configuration. It never
// suggests `archfit config init`: a missing candidate is a bad argument, not an
// unconfigured repository.
func loadCandidateConfig(ctx context.Context, path string) (config.Config, error) {
	cfg, err := config.Load(ctx, path)
	if err != nil {
		return config.Config{}, fmt.Errorf("candidate config %s: %w", path, err)
	}
	if err := validateConfigRules(cfg); err != nil {
		return config.Config{}, fmt.Errorf("candidate config %s: %w", path, err)
	}
	return cfg, nil
}

// runCompareSide runs one full advisory pipeline and projects it to a comparison
// side. Progress reporting is off (two runs would overflow one phase counter,
// see worktree.go) and warnings are labelled so a candidate-side degradation is
// never read as a current-side one.
func runCompareSide(ctx context.Context, deps *appDeps, cfg config.Config, rc runContext, label string) (decision.ConfigCompareSide, error) {
	quiet := *deps
	quiet.progress = nil
	quiet.warnLabel = label
	mode := engine.Mode{Full: true, Advisory: true, ReportOnly: true}
	// Empty baseline by design: raw measurement, no accepted-finding bias.
	diag, sc, err := runPipeline(ctx, &quiet, cfg, rc, mode, baseline.Baseline{})
	if err != nil {
		return decision.ConfigCompareSide{}, &exitError{
			code: 3,
			msg:  fmt.Sprintf("error: measuring %sconfig %s: %v", label, rc.ConfigSource, err),
		}
	}
	return decision.ConfigCompareSide{Diag: diag, Score: sc}, nil
}

// ---------------------------------------------------------------------------
// JSON document
// ---------------------------------------------------------------------------

// configCompareDoc is the `--json` contract (archfit.config-compare.v1). The
// decision types carry no JSON tags on purpose — the wire shape is owned here,
// in the composition root, so a pure-comparison refactor cannot silently rename
// a published key.
type configCompareDoc struct {
	SchemaVersion string               `json:"schema_version"`
	Current       configCompareSideDoc `json:"current"`
	Candidate     configCompareSideDoc `json:"candidate"`
	// ScoreDelta is candidate overall − current overall, null when either side
	// is unmeasured. Null is "unknown", which 0 ("no change") would misreport.
	ScoreDelta *int                     `json:"score_delta"`
	Findings   configCompareFindingsDoc `json:"findings"`
	Coverage   configCompareCoverageDoc `json:"coverage"`
	Warnings   []configCompareWarnDoc   `json:"warnings"`
}

// configCompareSideDoc is one configuration's complete measurement.
type configCompareSideDoc struct {
	ConfigHash string          `json:"config_hash"`
	Scorecard  score.Scorecard `json:"scorecard"`
	// ClassifiedEdges is null when classification produced no summary at all.
	ClassifiedEdges *diagnostic.ClassifiedEdgeSummary `json:"classified_edges"`
	// FindingCount is the number of DISTINCT observed finding IDs on this side,
	// with fixed entries excluded — the same population the ID buckets split, so
	// the two can never disagree. It is not len(findings).
	FindingCount int `json:"finding_count"`
}

type configCompareFindingsDoc struct {
	CurrentOnlyIDs   []string `json:"current_only_ids"`
	CandidateOnlyIDs []string `json:"candidate_only_ids"`
	BothIDs          []string `json:"both_ids"`
}

type configCompareCoverageDoc struct {
	Status  string                         `json:"status"`
	Details []configCompareCoverageRowsDoc `json:"details"`
}

type configCompareCoverageRowsDoc struct {
	Tool      string `json:"tool"`
	Current   string `json:"current"`
	Candidate string `json:"candidate"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
}

type configCompareWarnDoc struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

// buildConfigCompareDoc projects the two sides and the pure result onto the wire
// shape. Every list is a non-null array; only the two genuinely-unknown values
// (score delta, absent edge summary) are ever null.
func buildConfigCompareDoc(current, candidate decision.ConfigCompareSide, res decision.ConfigCompareResult) configCompareDoc {
	curCount := len(res.Findings.CurrentOnlyIDs) + len(res.Findings.BothIDs)
	candCount := len(res.Findings.CandidateOnlyIDs) + len(res.Findings.BothIDs)
	details := make([]configCompareCoverageRowsDoc, 0, len(res.Coverage.Details))
	for _, d := range res.Coverage.Details {
		details = append(details, configCompareCoverageRowsDoc{
			Tool:      d.Tool,
			Current:   d.Current,
			Candidate: d.Candidate,
			Status:    string(d.Status),
			Reason:    d.Reason,
		})
	}
	warnings := make([]configCompareWarnDoc, 0, len(res.Warnings))
	for _, w := range res.Warnings {
		warnings = append(warnings, configCompareWarnDoc{Code: w.Code, Text: w.Text})
	}
	return configCompareDoc{
		SchemaVersion: configCompareSchemaVersion,
		Current:       compareSideDoc(current, curCount),
		Candidate:     compareSideDoc(candidate, candCount),
		ScoreDelta:    res.ScoreDelta,
		Findings: configCompareFindingsDoc{
			CurrentOnlyIDs:   res.Findings.CurrentOnlyIDs,
			CandidateOnlyIDs: res.Findings.CandidateOnlyIDs,
			BothIDs:          res.Findings.BothIDs,
		},
		Coverage: configCompareCoverageDoc{Status: string(res.Coverage.Status), Details: details},
		Warnings: warnings,
	}
}

func compareSideDoc(side decision.ConfigCompareSide, findingCount int) configCompareSideDoc {
	return configCompareSideDoc{
		ConfigHash:      side.Diag.ConfigHash,
		Scorecard:       side.Score,
		ClassifiedEdges: side.Diag.ClassifiedEdges,
		FindingCount:    findingCount,
	}
}

func writeConfigCompareJSON(w io.Writer, doc configCompareDoc) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: encoding config comparison: %v", err)}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Text report
// ---------------------------------------------------------------------------

// configCompareIdentityLine is the exact line emitted when every measurement
// configCompareDiffLines compares came out the same.
//
// It names those measurements rather than claiming identity in general. The
// broad claim was false: the differences section reads the overall score, the
// one-sided finding IDs, the four edge counters and the classification mix, and
// nothing else in the two diagnostics. A future measurement added without a diff
// line would silently widen a claim written as "no differences"; it cannot widen
// this one.
const configCompareIdentityLine = "No change in score, findings, edge counts, or classification mix."

// configCompareFooter states the one thing a reader must not conclude from a
// score that moved.
const configCompareFooter = "Report only: two measurements of one source tree. " +
	"A higher candidate score is not evidence that the candidate configuration is better."

// renderConfigCompareText writes the short report: the coverage grade in its own
// section (it qualifies everything below it, including an identity result), then
// only the measurements that changed, then every warning.
func renderConfigCompareText(
	w io.Writer,
	currentPath, candidatePath string,
	current, candidate decision.ConfigCompareSide,
	res decision.ConfigCompareResult,
) {
	_, _ = fmt.Fprintf(w, "config compare\n  current:   %s\n  candidate: %s\n\n", currentPath, candidatePath)

	_, _ = fmt.Fprintf(w, "coverage evidence: %s\n", res.Coverage.Status)
	for _, d := range res.Coverage.Details {
		_, _ = fmt.Fprintf(w, "  - %s: current %s, candidate %s — %s\n", d.Tool, d.Current, d.Candidate, d.Reason)
	}
	_, _ = fmt.Fprintln(w)

	diffs := configCompareDiffLines(current, candidate, res)
	if len(diffs) == 0 {
		_, _ = fmt.Fprintln(w, configCompareIdentityLine)
	} else {
		_, _ = fmt.Fprintln(w, "measurement differences:")
		for _, line := range diffs {
			_, _ = fmt.Fprintf(w, "  %s\n", line)
		}
	}

	if len(res.Warnings) > 0 {
		_, _ = fmt.Fprintln(w, "\nmeasurement loss:")
		for _, warn := range res.Warnings {
			_, _ = fmt.Fprintf(w, "  - %s: %s\n", warn.Code, warn.Text)
		}
	}
	_, _ = fmt.Fprintf(w, "\n%s\n", configCompareFooter)
}

// configCompareDiffLines returns one line per changed measurement, in a fixed
// order. An unchanged measurement produces no line at all — the default report
// shows differences, not a full scorecard dump.
func configCompareDiffLines(current, candidate decision.ConfigCompareSide, res decision.ConfigCompareResult) []string {
	var lines []string
	if line, changed := scoreCompareLine(current.Score, candidate.Score, res.ScoreDelta); changed {
		lines = append(lines, line)
	}
	if ids := res.Findings.CurrentOnlyIDs; len(ids) > 0 {
		lines = append(lines, fmt.Sprintf("findings only under the current config (%d): %s", len(ids), summariseIDs(ids)))
	}
	if ids := res.Findings.CandidateOnlyIDs; len(ids) > 0 {
		lines = append(lines, fmt.Sprintf("findings only under the candidate config (%d): %s", len(ids), summariseIDs(ids)))
	}
	if line, changed := edgeCompareLine(current.Diag.ClassifiedEdges, candidate.Diag.ClassifiedEdges); changed {
		lines = append(lines, line)
	}
	return append(lines, classificationMixLines(current.Diag.ClassifiedEdges, candidate.Diag.ClassifiedEdges)...)
}

// classificationMixLines renders every cross-boundary classification histogram
// that moved.
//
// Nothing above can see these. An `owner:` or `deploy_unit:` edit moves edges
// between distance rungs without changing how many were scored, so the four edge
// counters hold; and with balance = max(|S−D|, 10−V) the 10−V term dominates for
// every low-volatility target, so the rounded score holds too. A proposed module
// split — the first use case this command documents — is exactly that shape, and
// it used to render as an identity result.
//
// The list is the complete set of classification histograms the diagnostic
// carries, so the identity line's claim covers the whole mix rather than a
// chosen subset of it.
func classificationMixLines(current, candidate *diagnostic.ClassifiedEdgeSummary) []string {
	mixes := []struct {
		label string
		pick  func(*diagnostic.ClassifiedEdgeSummary) map[string]int
	}{
		{"strength mix", func(s *diagnostic.ClassifiedEdgeSummary) map[string]int { return s.ByStrength }},
		{"distance mix", func(s *diagnostic.ClassifiedEdgeSummary) map[string]int { return s.ByDistance }},
		{"distance basis mix", func(s *diagnostic.ClassifiedEdgeSummary) map[string]int { return s.ByDistanceBasis }},
		{"volatility mix", func(s *diagnostic.ClassifiedEdgeSummary) map[string]int { return s.ByVolatility }},
		{"severity mix", func(s *diagnostic.ClassifiedEdgeSummary) map[string]int { return s.BySeverity }},
		{"volatility provenance (modules)", volatilityProvenanceCounts},
	}
	var lines []string
	for _, m := range mixes {
		if line, changed := histogramCompareLine(m.label, pickHistogram(current, m.pick), pickHistogram(candidate, m.pick)); changed {
			lines = append(lines, line)
		}
	}
	return lines
}

// pickHistogram reads one histogram from a side, treating a side that never
// classified anything as an empty one.
func pickHistogram(
	s *diagnostic.ClassifiedEdgeSummary,
	pick func(*diagnostic.ClassifiedEdgeSummary) map[string]int,
) map[string]int {
	if s == nil {
		return nil
	}
	return pick(s)
}

// volatilityProvenanceCounts flattens the provenance struct to the same
// bucket-count shape as the other histograms. These count MODULES, not edges —
// hence the label.
func volatilityProvenanceCounts(s *diagnostic.ClassifiedEdgeSummary) map[string]int {
	p := s.VolatilityProvenance
	if p == nil {
		return nil
	}
	return map[string]int{
		"declared":   p.Declared,
		"inherited":  p.Inherited,
		"cascade":    p.Cascade,
		"undeclared": p.Undeclared,
	}
}

// histogramCompareLine renders only the buckets that moved, sorted by bucket so
// a double-run stays byte-identical. A bucket present on one side and absent on
// the other reads as 0 there, which is what an absent key means.
func histogramCompareLine(label string, cur, cand map[string]int) (string, bool) {
	keys := make([]string, 0, len(cur)+len(cand))
	seen := make(map[string]struct{}, len(cur)+len(cand))
	for _, m := range []map[string]int{cur, cand} {
		for k := range m {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	moved := make([]string, 0, len(keys))
	for _, k := range keys {
		if cur[k] == cand[k] {
			continue
		}
		moved = append(moved, fmt.Sprintf("%s %d → %d", k, cur[k], cand[k]))
	}
	if len(moved) == 0 {
		return "", false
	}
	return fmt.Sprintf("%s: %s", label, strings.Join(moved, ", ")), true
}

// edgeCompareLine renders the cross-boundary edge footprint when it moved. The
// counts change without the rounded score or the finding IDs changing at all —
// a candidate that scores the same edges differently, or measures more of the
// tree, is still a changed measurement, and the identity line must not claim
// otherwise.
func edgeCompareLine(current, candidate *diagnostic.ClassifiedEdgeSummary) (string, bool) {
	if current == nil && candidate == nil {
		return "", false
	}
	cur, cand := edgeCountsText(current), edgeCountsText(candidate)
	if cur == cand {
		return "", false
	}
	return fmt.Sprintf("classified edges: %s → %s", cur, cand), true
}

func edgeCountsText(s *diagnostic.ClassifiedEdgeSummary) string {
	if s == nil {
		return scoreUnmeasured
	}
	return fmt.Sprintf("%d total / %d scored / %d abstained / %d external",
		s.Total, s.Scored, s.Abstained, s.External)
}

// compareIDPreview caps how many finding IDs the text report spells out. A
// config change can move dozens of findings at once, and a 30-fingerprint line
// is unreadable; the count is already on the line and --json carries them all.
const compareIDPreview = 5

func summariseIDs(ids []string) string {
	if len(ids) <= compareIDPreview {
		return strings.Join(ids, ", ")
	}
	return fmt.Sprintf("%s, … %d more (see --json)",
		strings.Join(ids[:compareIDPreview], ", "), len(ids)-compareIDPreview)
}

// scoreCompareLine renders the overall-score row and reports whether it is a
// difference at all.
//
// The nil-delta cases are deliberately split. Unmeasured on BOTH sides is not a
// difference — neither configuration produced a number, and saying so as a
// "change" would invent one. Unmeasured on exactly ONE side IS a difference, and
// an important one: a configuration that stopped measuring coupling entirely is
// exactly what this command exists to expose.
func scoreCompareLine(current, candidate score.Scorecard, delta *int) (string, bool) {
	switch {
	case delta != nil && *delta != 0:
		return fmt.Sprintf("score: %s → %s (%+d)", scoreText(current), scoreText(candidate), *delta), true
	case delta != nil:
		return "", false
	case current.OverallBand.Unmeasured() && candidate.OverallBand.Unmeasured():
		return "", false
	default:
		return fmt.Sprintf("score: %s → %s (delta unknown: one side is unmeasured)",
			scoreText(current), scoreText(candidate)), true
	}
}

func scoreText(sc score.Scorecard) string {
	if sc.OverallBand.Unmeasured() {
		return scoreUnmeasured
	}
	return fmt.Sprintf("%d (%s)", sc.Overall, sc.OverallBand)
}
