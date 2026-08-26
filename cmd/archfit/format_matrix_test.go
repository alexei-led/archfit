package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

// Committed pre-migration fixture names, one per renderer. baseline.json (the
// json format) is owned by byteidentical_test.go; the remaining four are
// captured here so the architecture-state cutover in later tasks has a frozen
// "before" for every format, not only for JSON.
//
// They live under cmd/archfit/testdata/, NOT beside the analysed fixture: a
// baseline written into the fixture tree becomes an input to the next run that
// copies that tree, and four parallel subtests bootstrapping into a shared
// source directory race over what each one analyses.
const (
	formatMatrixBaselineDir = "testdata/format-matrix"

	baselineText      = "text.txt"
	baselineMarkdown  = "markdown.md"
	baselineSarif     = "sarif.json"
	baselineScorecard = "scorecard.txt"
)

// formatMatrix is the compatibility matrix frozen by Task 1 of the
// architecture-state migration (docs/design/architecture-state-reporting.md).
// Every renderer archfit ships appears exactly once; adding a format without
// adding its row leaves the cutover unwitnessed for that format.
var formatMatrix = []struct {
	format   string // --format value
	baseline string // committed fixture under formatMatrixBaselineDir
	jsonOut  bool   // renderer emits JSON, so the baseline is canonicalised
}{
	{format: "text", baseline: baselineText},
	{format: "markdown", baseline: baselineMarkdown},
	{format: "sarif", baseline: baselineSarif, jsonOut: true},
	{format: "scorecard", baseline: baselineScorecard},
}

// TestFormatMatrix_PreStateBaselines pins the rendered output of every non-JSON
// format against the machine-independent single-module fixture. It is the
// pre-cutover half of the migration compatibility matrix: Task 1 must not move
// any of these bytes, and the task that does cut over must move them
// deliberately, in the same commit that updates the contract.
func TestFormatMatrix_PreStateBaselines(t *testing.T) {
	t.Parallel()
	requireHealthyExtraction(t)
	for _, tc := range formatMatrix {
		t.Run(tc.format, func(t *testing.T) {
			t.Parallel()
			_, root := materializeFixtureRepo(t, fixtureSingleModule)
			got := runAnalyzeFormat(t, root, tc.format, tc.jsonOut)
			assertMatchesBaseline(t, filepath.Join(formatMatrixBaselineDir, tc.baseline), got)
		})
	}
}

// TestFormatMatrix_DoubleRunIsStable asserts each renderer is deterministic on
// an unchanged tree. A format that differs between two identical runs cannot
// carry a byte-comparable migration baseline at all.
func TestFormatMatrix_DoubleRunIsStable(t *testing.T) {
	t.Parallel()
	for _, tc := range formatMatrix {
		t.Run(tc.format, func(t *testing.T) {
			t.Parallel()
			_, root := materializeFixtureRepo(t, fixtureSingleModule)
			first := runAnalyzeFormat(t, root, tc.format, tc.jsonOut)
			second := runAnalyzeFormat(t, root, tc.format, tc.jsonOut)
			if !bytes.Equal(first, second) {
				t.Fatalf("%s renderer is not deterministic:\n%s", tc.format,
					firstDiffLine(string(first), string(second)))
			}
		})
	}
}

// TestFormatMatrix_ExitCodesUnchanged pins the process exit contract this
// migration must preserve, at the level cmd owns. The executable authority for
// the full 0/1/2/3 table remains scripts/tests/cli_exit_contract_test.sh; this
// table is the in-process regression net for the paths the state cutover
// touches: analyze is report-only for every format, and check maps a gate
// violation to 1 and a bad config path to 3.
func TestFormatMatrix_ExitCodesUnchanged(t *testing.T) {
	t.Parallel()

	t.Run("analyze is report-only in every format", func(t *testing.T) {
		t.Parallel()
		cfgPath := writeViolatingRepo(t)
		for _, tc := range formatMatrix {
			code, stdout, stderr := runArchfit(t, cmdAnalyze, "-c", cfgPath, "--format="+tc.format)
			if code != 0 {
				t.Errorf("analyze --format=%s on a violated policy: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s",
					tc.format, code, stdout, stderr)
			}
		}
	})

	t.Run("check maps gate state to the frozen exit codes", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			cfg  func(*testing.T) string
			want int
		}{
			// V1 reports complexity, testability, and operations partial by
			// contract, and any partial dimension is needs_attention — so a
			// fixture with nothing to violate is 2, not 0. Fabricating healthy
			// here is the implicit green result the contract prevents.
			{name: "clean", cfg: func(t *testing.T) string { return writeCheckFixtureRepo(t, "golang") }, want: 2},
			{name: "violated", cfg: writeViolatingRepo, want: 1},
			{name: "missing config", cfg: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nope.yaml")
			}, want: 3},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				code, stdout, stderr := runArchfit(t, cmdCheck, "-c", tc.cfg(t))
				if code != tc.want {
					t.Fatalf("check %s: exit = %d, want %d\nstdout:\n%s\nstderr:\n%s",
						tc.name, code, tc.want, stdout, stderr)
				}
			})
		}
	})
}

// requireHealthyExtraction fails fast when the environment — not the code —
// degraded Go extraction, so an environment problem cannot be misread as an
// output regression and "fixed" by re-recording the baselines.
//
// The failure mode is real and has already cost one capture: a sandbox that
// denies writes to the Go module cache makes packages.Load fail per package,
// which raises Coverage.Unresolved, which drops the coverage metric to low
// confidence, which caps its band from strong to mixed. Only the scorecard
// carries that band, so the JSON envelope stays byte-identical while every
// scorecard-bearing renderer moves. The baselines here are captured with Go
// extraction healthy; run these tests with module-cache writes permitted.
func requireHealthyExtraction(t *testing.T) {
	t.Helper()

	_, root := materializeFixtureRepo(t, fixtureSingleModule)
	var doc struct {
		ToolCoverage []struct {
			Tool       string `json:"tool"`
			Status     string `json:"status"`
			Unresolved int    `json:"unresolved"`
		} `json:"tool_coverage"`
	}
	if err := json.Unmarshal(runAnalyzeFormat(t, root, formatLegacyJSON, true), &doc); err != nil {
		t.Fatalf("decode tool coverage: %v", err)
	}
	for _, c := range doc.ToolCoverage {
		if c.Tool != toolGoPackages {
			continue
		}
		if c.Status != string(evidence.StatusOK) || c.Unresolved != 0 {
			t.Fatalf("Go extraction is degraded in this environment (%s: status=%q unresolved=%d) — "+
				"the format baselines are not comparable here; do not re-record them",
				c.Tool, c.Status, c.Unresolved)
		}
		return
	}
	t.Fatalf("no %s coverage row in the fixture run: %+v", toolGoPackages, doc.ToolCoverage)
}

// runAnalyzeFormat runs `archfit analyze --format <format>` in-process against
// the materialized repo and normalises the temp root out of the result. JSON
// renderers additionally round-trip through normalizeArchfitJSON for canonical
// key ordering; text renderers are compared as written.
func runAnalyzeFormat(t *testing.T, root, format string, jsonOut bool) []byte {
	t.Helper()

	cfgPath := filepath.Join(root, ".archfit.yaml")
	var buf bytes.Buffer
	code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, "--format=" + format}, &buf)
	if code != 0 && code != 1 {
		t.Fatalf("archfit analyze --format=%s exited %d (want 0 or 1):\n%s", format, code, buf.String())
	}
	if jsonOut {
		got, err := normalizeArchfitJSON(buf.Bytes(), root)
		if err != nil {
			t.Fatalf("normalise %s output: %v", format, err)
		}
		return got
	}
	return []byte(normalizeRoot(buf.String(), root))
}

// normalizeRoot replaces the temp scan root (in both native and forward-slash
// spelling) with <ROOT> so rendered paths are stable across runs.
func normalizeRoot(s, root string) string {
	out := strings.ReplaceAll(s, root, "<ROOT>")
	if fwd := filepath.ToSlash(root); fwd != root {
		out = strings.ReplaceAll(out, fwd, "<ROOT>")
	}
	return out
}

// assertMatchesBaseline compares got against the committed fixture at path,
// bootstrapping the fixture on first run so a new format's baseline can be
// reviewed and committed. Never delete a committed baseline to get green: that
// re-records whatever the code now emits and makes the gate vacuous.
func assertMatchesBaseline(t *testing.T, path string, got []byte) {
	t.Helper()

	want, err := os.ReadFile(path) //nolint:gosec // path is a compile-time testdata constant
	if os.IsNotExist(err) {
		// Regeneration is deliberate, matching TestModelSurfaceNoDrift and the
		// config-schema golden. Bootstrapping silently turned a deleted or
		// unshipped fixture into a self-recording no-op that reported green.
		if os.Getenv("ARCHFIT_UPDATE_FORMATS") == "" {
			t.Fatalf("baseline %s is missing; regenerate deliberately with ARCHFIT_UPDATE_FORMATS=1 and inspect the diff", path)
		}
		if writeErr := os.WriteFile(path, got, 0o600); writeErr != nil {
			t.Fatalf("write baseline %s: %v", path, writeErr)
		}
		t.Logf("baseline written to %s — review and commit", path)
		return
	}
	if err != nil {
		t.Fatalf("read baseline %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("output differs from %s:\n%s", filepath.Base(path), firstDiffLine(string(want), string(got)))
	}
}

// TestFormatMatrix_CrossFormatParity is the migration's honesty check: every
// primary format must report the SAME decision, the same nine dimensions with
// the same statuses, the same counts, and the same finding IDs. A format that
// quietly disagrees lets a reader pick the answer they prefer.
//
// SARIF is exempt from layout parity, not fact parity: it carries the state in
// run properties, which TestFormatMatrix_SarifCarriesTheState checks.
func TestFormatMatrix_CrossFormatParity(t *testing.T) {
	t.Parallel()
	requireHealthyExtraction(t)

	cfgPath := writeViolatingRepo(t)
	stateJSON := runFormatOutput(t, cfgPath, formatJSON)

	var state struct {
		Verdict  string `json:"verdict"`
		Decision struct {
			HardGates           string `json:"hard_gates"`
			ActiveBlockers      int    `json:"active_blockers"`
			AttentionDimensions int    `json:"attention_dimensions"`
		} `json:"decision"`
		Dimensions map[string]struct {
			Status string `json:"status"`
			Gate   string `json:"gate"`
		} `json:"dimensions"`
		Coverage struct {
			Measured   int `json:"measured"`
			Partial    int `json:"partial"`
			Unmeasured int `json:"unmeasured"`
		} `json:"coverage"`
		Findings []struct {
			ID     string `json:"id"`
			RuleID string `json:"rule_id"`
			Status string `json:"status"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		t.Fatalf("decode architecture state: %v", err)
	}
	if len(state.Findings) == 0 {
		t.Fatal("the parity fixture produced no findings — the finding-sequence assertion below would be vacuous")
	}

	verdictLabel := strings.ToUpper(strings.ReplaceAll(state.Verdict, "_", " "))
	coverage := []string{
		strconv.Itoa(state.Coverage.Measured), strconv.Itoa(state.Coverage.Partial), strconv.Itoa(state.Coverage.Unmeasured),
	}
	canonical := make([]string, 0, len(state.Findings))
	for _, f := range state.Findings {
		canonical = append(canonical, f.ID)
	}

	for _, format := range []string{formatText, formatMarkdown, formatScorecard} {
		t.Run(format, func(t *testing.T) {
			out := string(runFormatOutput(t, cfgPath, format))

			if !strings.Contains(out, verdictLabel) {
				t.Errorf("%s does not report the verdict %q:\n%s", format, verdictLabel, out)
			}
			if !strings.Contains(out, state.Decision.HardGates) {
				t.Errorf("%s does not report hard gates %q:\n%s", format, state.Decision.HardGates, out)
			}
			for name, dim := range state.Dimensions {
				// Status and gate must sit on the DIMENSION'S OWN line. A
				// document-global Contains can never fail here: nine
				// dimensions share three status words and the coverage
				// headline prints all three, so a format that drops a row
				// still finds the word somewhere else. Every human format
				// renders name, status, and gate together.
				lines := dimensionLines(out, name)
				if len(lines) == 0 {
					t.Errorf("%s omits dimension %q:\n%s", format, name, out)
					continue
				}
				if !anyLineHas(lines, dim.Status, dim.Gate) {
					t.Errorf("%s reports no %s line carrying status %q and gate %q; candidates: %q",
						format, name, dim.Status, dim.Gate, lines)
				}
			}
			// The coverage triple must be present as three numbers on one line,
			// so a format cannot report a different split than JSON did.
			if !containsCoverageTriple(out, coverage) {
				t.Errorf("%s does not report the coverage split %v:\n%s", format, coverage, out)
			}
			for _, f := range state.Findings {
				if !strings.Contains(out, f.RuleID) {
					t.Errorf("%s omits the rule %q behind finding %s:\n%s", format, f.RuleID, f.ID, out)
				}
				if !strings.Contains(out, f.Status) {
					t.Errorf("%s omits the status %q of finding %s:\n%s", format, f.Status, f.ID, out)
				}
			}
			// A human format may lead with the actionable findings and cap that
			// list, but it must still append every finding ID in the document's
			// canonical order. Without the appendix an abbreviated render and a
			// genuinely shorter run are indistinguishable.
			assertCanonicalFindingSequence(t, format, out, canonical)
		})
	}
}

// assertCanonicalFindingSequence checks that every canonical finding ID appears
// in out, and that the LAST occurrence of each one runs in canonical order — the
// appendix. Earlier mentions in an actionable section are free to reorder by
// severity; the appendix is what fixes the sequence across formats.
func assertCanonicalFindingSequence(t *testing.T, format, out string, canonical []string) {
	t.Helper()

	prev := -1
	for _, id := range canonical {
		at := strings.LastIndex(out, id)
		if at < 0 {
			t.Errorf("%s omits finding %s from its finding index:\n%s", format, id, out)
			return
		}
		if at <= prev {
			t.Errorf("%s lists finding %s out of canonical order (at %d, previous at %d)", format, id, at, prev)
			return
		}
		prev = at
	}
}

// containsCoverageTriple reports whether any line carries all three coverage
// counts in order. Checking the numbers on ONE line rather than anywhere in the
// document keeps an unrelated count from satisfying the assertion.
func containsCoverageTriple(out string, counts []string) bool {
	m := coverageTripleRe.FindStringSubmatch(out)
	if m == nil {
		return false
	}
	return m[1] == counts[0] && m[2] == counts[1] && m[3] == counts[2]
}

// coverageTripleRe parses the coverage headline off its own labels. Testing
// whether three numbers appear in order on a line mentioning "measured" is not
// an assertion: a dimension denominator like "63/70" supplies a 6, a 3, and a 0.
var coverageTripleRe = regexp.MustCompile(
	`(\d+)\s*measured\D+?(\d+)\s*partial\D+?(\d+)\s*unmeasured`)

// dimensionLines returns every rendered line naming dim as a whole word. The
// word boundary matters: the scorecard prints a `local_coupling_modules` metric
// before the `coupling` heading, and a substring match would grade the wrong
// line. Text and scorecard render a heading, Markdown a table row; all three put
// the name, status, and gate together on one of these lines.
func dimensionLines(out, dim string) []string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(dim) + `\b`)
	var hits []string
	for _, line := range strings.Split(out, "\n") {
		if re.MatchString(line) {
			hits = append(hits, line)
		}
	}
	return hits
}

// anyLineHas reports whether one of lines carries every token.
func anyLineHas(lines []string, tokens ...string) bool {
	for _, line := range lines {
		ok := true
		for _, tok := range tokens {
			if !strings.Contains(line, tok) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// runFormatOutput renders one format over the fixture and returns stdout.
func runFormatOutput(t *testing.T, cfgPath, format string) []byte {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := RunWithStderr([]string{cmdAnalyze, "-c", cfgPath, "--progress=none", "--format=" + format}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("analyze --format=%s: exit=%d\nstdout:\n%s\nstderr:\n%s", format, code, stdout.String(), stderr.String())
	}
	return stdout.Bytes()
}

// TestFormatMatrix_SarifCarriesTheState is SARIF's half of the parity rule: it
// is exempt from human layout, not from facts. The run properties must report
// the same verdict, decision, nine dimensions, and coverage split as
// --format json, and every finding must keep its rule ID and fingerprint.
func TestFormatMatrix_SarifCarriesTheState(t *testing.T) {
	t.Parallel()
	requireHealthyExtraction(t)

	cfgPath := writeViolatingRepo(t)

	var state struct {
		Verdict  string `json:"verdict"`
		Decision struct {
			HardGates      string `json:"hard_gates"`
			ActiveBlockers int    `json:"active_blockers"`
		} `json:"decision"`
		Dimensions map[string]struct {
			Status     string `json:"status"`
			Gate       string `json:"gate"`
			Confidence string `json:"confidence"`
		} `json:"dimensions"`
		Coverage struct {
			Measured   int `json:"measured"`
			Partial    int `json:"partial"`
			Unmeasured int `json:"unmeasured"`
		} `json:"coverage"`
		Findings []struct {
			ID     string `json:"id"`
			RuleID string `json:"rule_id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(runFormatOutput(t, cfgPath, formatJSON), &state); err != nil {
		t.Fatalf("decode architecture state: %v", err)
	}

	var log struct {
		Runs []struct {
			Properties struct {
				Verdict  string `json:"verdict"`
				Decision struct {
					HardGates      string `json:"hard_gates"`
					ActiveBlockers int    `json:"active_blockers"`
				} `json:"decision"`
				Dimensions []struct {
					Name       string `json:"name"`
					Status     string `json:"status"`
					Gate       string `json:"gate"`
					Confidence string `json:"confidence"`
				} `json:"dimensions"`
				Coverage struct {
					Measured   int `json:"measured"`
					Partial    int `json:"partial"`
					Unmeasured int `json:"unmeasured"`
				} `json:"coverage"`
			} `json:"properties"`
			Results []struct {
				RuleID       string            `json:"ruleId"`
				Fingerprints map[string]string `json:"fingerprints"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(runFormatOutput(t, cfgPath, formatSarif), &log); err != nil {
		t.Fatalf("decode SARIF: %v", err)
	}
	if len(log.Runs) != 1 {
		t.Fatalf("SARIF runs = %d, want 1", len(log.Runs))
	}
	props := log.Runs[0].Properties

	if props.Verdict != state.Verdict {
		t.Errorf("SARIF verdict = %q, JSON says %q", props.Verdict, state.Verdict)
	}
	if props.Decision != state.Decision {
		t.Errorf("SARIF decision = %+v, JSON says %+v", props.Decision, state.Decision)
	}
	if props.Coverage != state.Coverage {
		t.Errorf("SARIF coverage = %+v, JSON says %+v", props.Coverage, state.Coverage)
	}
	if len(props.Dimensions) != stateDimensionCount {
		t.Errorf("SARIF dimensions = %d, want %d", len(props.Dimensions), stateDimensionCount)
	}
	// SARIF is exempt from human LAYOUT parity, not from FACT parity. Counting
	// the envelopes lets SARIF report every dimension measured while JSON
	// reports three partial, so compare the facts by name.
	for _, dim := range props.Dimensions {
		want, ok := state.Dimensions[dim.Name]
		if !ok {
			t.Errorf("SARIF carries dimension %q, which the state does not", dim.Name)
			continue
		}
		if dim.Status != want.Status || dim.Gate != want.Gate || dim.Confidence != want.Confidence {
			t.Errorf("SARIF %s = {%s %s %s}, JSON says {%s %s %s}",
				dim.Name, dim.Status, dim.Gate, dim.Confidence, want.Status, want.Gate, want.Confidence)
		}
	}

	// Finding identity is untouched by the cutover: an existing code-scanning
	// consumer must keep resolving the same alerts.
	fingerprints := map[string]string{}
	for _, r := range log.Runs[0].Results {
		fingerprints[r.Fingerprints["archfit/v1"]] = r.RuleID
	}
	for _, f := range state.Findings {
		rule, present := fingerprints[f.ID]
		if !present {
			t.Errorf("SARIF dropped finding %s (%s)", f.ID, f.RuleID)
			continue
		}
		if rule != f.RuleID {
			t.Errorf("SARIF ruleId for %s = %q, JSON says %q", f.ID, rule, f.RuleID)
		}
	}
}
