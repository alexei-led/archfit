// coverage generates a gap-closure coverage table mapping each frozen inventory
// finding to one of: surfaced | llm-routed-by-design | agree | not-surfaced.
//
// Input: a directory of full.json files (either <repo>/full.json layout from
// gap-closure.sh, or flat <repo>-archfit.json from the eval harness).
// Output: markdown table on stdout.
//
// Usage:
//
//	go run ./scripts/eval/coverage --dir docs/archived/reports/eval/gap-closure > docs/archived/reports/eval/gap-closure/coverage.md
//	go run ./scripts/eval/coverage --dir ~/Workspace/archfit/docs/archived/reports/eval > docs/archived/reports/eval/gap-closure/coverage.md
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Status values for each finding in the coverage table.
const (
	statusSurfaced        = "surfaced"
	statusLLMRoutedDesign = "llm-routed-by-design"
	statusAgree           = "agree"
	statusNotSurfaced     = "not-surfaced"
)

// Repo name constants (used ≥3 times each in the inventory table).
const (
	repoArchfit   = "archfit"
	repoCcgram    = "ccgram"
	repoCodegraph = "codegraph"
	repoHerdr     = "herdr"
	repoPumba     = "pumba"
	repoYazi      = "yazi"
)

// Category constants used multiple times.
const (
	catSemanticIntent = "Semantic/Intent"
)

// Probe signal constants — metric names and syntax_fact kinds for future detectors.
// Each constant appears in both the inventory table and the test fixtures.
const (
	probeMetricUnsafeDensity = "unsafe_density"
	probeKindUnsafeOp        = "unsafe_op"
	probeMetricTestDensity   = "test_density"
	probeKindTestFn          = "test_fn"
)

// Finding ID constants that appear in both main and test files.
const (
	findingID61  = "6.1"
	findingID132 = "13.2"
)

// bandNA is the sentinel band value emitted when a metric is not applicable.
const bandNA = "n/a"

// Finding is one row in the frozen 20-row inventory table.
type Finding struct {
	ID       string
	Title    string
	Repo     string
	Category string
	// BaseStatus is the status from the frozen inventory (hardcoded for agree and
	// llm-routed; probeSignal/probeKind drive the surfaced/not-surfaced distinction).
	BaseStatus string
	// ProbeMetric is the metric name that will be present after the detector ships.
	// Empty means status is fully determined by BaseStatus.
	ProbeMetric string
	// ProbeKind is the syntax_fact kind that will be present after the detector ships.
	ProbeKind string
	// SurfaceWhenZero inverts the probe polarity for absence-signal findings.
	// Default (false): surfaced when metric value > 0 (detector found something).
	// True: surfaced when metric is present, band != "n/a", AND value == 0 (detector
	// ran and found NOTHING — the absence IS the finding, e.g. zero test density).
	// Tool-absent (metric missing or band == "n/a") is never surfaced for either polarity.
	SurfaceWhenZero bool
}

// inventory is the frozen 20-row ground truth from architect-only-inventory.md.
// BaseStatus is only set for agree and llm-routed-by-design findings.
// not-surfaced findings carry ProbeMetric/ProbeKind for the signal that will
// flip them to surfaced once the relevant task ships.
var inventory = []Finding{
	{
		ID:         "1.1",
		Title:      "score.go 1009-line god-file",
		Repo:       repoArchfit,
		Category:   "Single-File God-File",
		BaseStatus: statusLLMRoutedDesign,
	},
	{
		ID:         "1.2",
		Title:      "markdown.go 748-line renderer",
		Repo:       repoArchfit,
		Category:   "Single-File God-File",
		BaseStatus: statusLLMRoutedDesign,
	},
	{
		ID:         "1.4",
		Title:      "directory_callbacks._dispatch sub-router",
		Repo:       repoCcgram,
		Category:   "Single-File God-File + Semantic",
		BaseStatus: statusLLMRoutedDesign,
	},
	{
		ID:          "2.1",
		Title:       "AppState 106 pub fields",
		Repo:        repoHerdr,
		Category:    "God Struct by Field Count",
		ProbeMetric: "struct_field_max",
		ProbeKind:   "struct_field",
	},
	{
		ID:         "3.1",
		Title:      "Mock files in production binary",
		Repo:       repoPumba,
		Category:   "Test Code in Production",
		BaseStatus: statusAgree,
		// agree: test_in_production rule type exists in the engine.
		// The capability is shipped (internal/rules/rules.go); finding fires
		// when the config includes a test_in_production rule.
	},
	{
		ID:         "3.2",
		Title:      "Root-level mocks/ distant coupling",
		Repo:       repoPumba,
		Category:   "Test Code in Production",
		BaseStatus: statusLLMRoutedDesign,
	},
	{
		ID:         "4.1",
		Title:      "urfave/cli v1 EOL",
		Repo:       repoPumba,
		Category:   "Dependency Deprecation",
		BaseStatus: statusLLMRoutedDesign,
		// deprecated_dep_count tracks go.mod retract markers; EOL=live-registry fact, routed to LLM
	},
	{
		ID:          "5.1",
		Title:       "cliflags.V1.Ctx type leak",
		Repo:        repoPumba,
		Category:    "Public API Type Leak",
		ProbeMetric: "public_api_type_leak",
		ProbeKind:   "type_leak",
	},
	{
		ID:          findingID61,
		Title:       "Unsafe lifetime erasure (yazi)",
		Repo:        repoYazi,
		Category:    "Unsafe Code",
		ProbeMetric: probeMetricUnsafeDensity,
		ProbeKind:   probeKindUnsafeOp,
	},
	{
		ID:          "6.2",
		Title:       "FFI trampolines (herdr)",
		Repo:        repoHerdr,
		Category:    "Unsafe Code",
		ProbeMetric: probeMetricUnsafeDensity,
		ProbeKind:   probeKindUnsafeOp,
	},
	{
		ID:          "7.1",
		Title:       "15+ OnceLock/Atomic singletons",
		Repo:        repoHerdr,
		Category:    "Global Mutable State",
		ProbeMetric: "global_state_density",
		ProbeKind:   "global_state",
	},
	{
		ID:          "8.1",
		Title:       "551 production unwrap/expect/panic!",
		Repo:        repoHerdr,
		Category:    "Panic/Error-Handling",
		ProbeMetric: "panic_density",
		ProbeKind:   "panic_op",
	},
	{
		ID:        "9.1",
		Title:     "polling_coordinator lazy-import cycle",
		Repo:      repoCcgram,
		Category:  "Dynamic/Lazy Import",
		ProbeKind: "lazy_import",
	},
	{
		ID:         "10.1",
		Title:      "yazi-core → yazi-adapter layer violation",
		Repo:       repoYazi,
		Category:   "Layer Violation",
		BaseStatus: statusLLMRoutedDesign,
	},
	{
		ID:          "11.1",
		Title:       "extraction ↔ resolution bidirectional coupling",
		Repo:        repoCodegraph,
		Category:    "Non-Cycle Bidirectional",
		ProbeMetric: "file_mutual_import",
	},
	{
		ID:         "12.1",
		Title:      "Key-format fragmentation",
		Repo:       repoArchfit,
		Category:   catSemanticIntent,
		BaseStatus: statusLLMRoutedDesign,
	},
	{
		ID:         "12.2",
		Title:      "config Scorer field widens closure",
		Repo:       repoArchfit,
		Category:   catSemanticIntent,
		BaseStatus: statusLLMRoutedDesign,
	},
	{
		ID:         "12.3",
		Title:      "polling_types.py stdlib-purity",
		Repo:       repoCcgram,
		Category:   catSemanticIntent,
		BaseStatus: statusLLMRoutedDesign,
	},
	{
		ID:          "13.1",
		Title:       "Negligible unit coverage (yazi)",
		Repo:        repoYazi,
		Category:    "Test Coverage Blind Spot",
		ProbeMetric: probeMetricTestDensity,
		ProbeKind:   probeKindTestFn,
	},
	{
		ID:              findingID132,
		Title:           "No unit tests (herdr)",
		Repo:            repoHerdr,
		Category:        "Test Coverage Blind Spot",
		ProbeMetric:     probeMetricTestDensity,
		SurfaceWhenZero: true,
		// No ProbeKind: test_fn would surface when tests ARE found, inverting the signal.
		// Absence is signalled by test_density == 0 (detector ran, found nothing).
	},
}

// fullJSON is the top-level shape of archfit full.json output.
type fullJSON struct {
	Metrics     []metricEntry  `json:"metrics"`
	Findings    []findingEntry `json:"findings"`
	SyntaxFacts []syntaxFact   `json:"syntax_facts"`
}

type metricEntry struct {
	Name  string  `json:"name"`
	Band  string  `json:"band"`
	Value float64 `json:"value"`
}

type findingEntry struct {
	RuleID string `json:"rule_id"`
	Text   string `json:"text"`
}

type syntaxFact struct {
	Kind string `json:"kind"`
}

// repoData holds all parsed JSON for a single repo.
type repoData struct {
	// metricPosSignal: metric name → true when present, band != "n/a", value > 0.
	// Used by default (positive-polarity) probes.
	metricPosSignal map[string]bool
	// metricZeroSignal: metric name → true when present, band != "n/a", value == 0.
	// Used by absence-signal (SurfaceWhenZero) probes where zero IS the finding.
	// Distinct from tool-absent: a metric missing from the JSON is in neither map.
	metricZeroSignal map[string]bool
	factKinds        map[string]bool
}

func parseFullJSON(path string) (*repoData, error) {
	// #nosec G304 — path is constructed from a trusted directory argument, not user HTTP input.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "warn: close %s: %v\n", path, cerr)
		}
	}()

	var doc fullJSON
	if err := json.NewDecoder(f).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	rd := &repoData{
		metricPosSignal:  make(map[string]bool),
		metricZeroSignal: make(map[string]bool),
		factKinds:        make(map[string]bool),
	}
	for _, m := range doc.Metrics {
		if m.Band == "" || m.Band == bandNA {
			continue // tool absent or no signal — not surfaced for either polarity
		}
		if m.Value > 0 {
			rd.metricPosSignal[m.Name] = true
		} else {
			// value == 0: detector ran, found nothing — absence-signal probes check this.
			rd.metricZeroSignal[m.Name] = true
		}
	}
	for _, sf := range doc.SyntaxFacts {
		rd.factKinds[sf.Kind] = true
	}
	return rd, nil
}

// ClassifyFinding returns the coverage status for a single finding given the
// JSON data for its target repo. When rd is nil (repo not scanned), not-surfaced
// findings remain not-surfaced.
func ClassifyFinding(f Finding, rd *repoData) string {
	// agree and llm-routed are hardcoded — no probe needed.
	if f.BaseStatus == statusAgree || f.BaseStatus == statusLLMRoutedDesign {
		return f.BaseStatus
	}

	// No data for this repo: can't prove surfaced.
	if rd == nil {
		return statusNotSurfaced
	}

	// Check for the expected future signal. Polarity is per-probe:
	//   default (SurfaceWhenZero=false): metric present, band != "n/a", value > 0
	//   absence-signal (SurfaceWhenZero=true): metric present, band != "n/a", value == 0
	// Tool-absent (metric not in JSON or band == "n/a") is never surfaced.
	if f.ProbeMetric != "" {
		if f.SurfaceWhenZero {
			if rd.metricZeroSignal[f.ProbeMetric] {
				return statusSurfaced
			}
		} else {
			if rd.metricPosSignal[f.ProbeMetric] {
				return statusSurfaced
			}
		}
	}
	if f.ProbeKind != "" && rd.factKinds[f.ProbeKind] {
		return statusSurfaced
	}
	return statusNotSurfaced
}

// loadRepoData discovers full.json files from dir.
// Supports two layouts:
//   - <dir>/<repo>/full.json  (gap-closure.sh output)
//   - <dir>/<repo>-archfit.json  (flat eval harness output)
func loadRepoData(dir string) (map[string]*repoData, error) {
	result := make(map[string]*repoData)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", dir, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			// Layout: <dir>/<repo>/full.json
			jsonPath := filepath.Join(dir, e.Name(), "full.json")
			if _, statErr := os.Stat(jsonPath); statErr == nil {
				rd, parseErr := parseFullJSON(jsonPath)
				if parseErr != nil {
					fmt.Fprintf(os.Stderr, "warn: %v\n", parseErr)
					continue
				}
				result[e.Name()] = rd
			}
		} else {
			// Layout: <dir>/<repo>-archfit.json
			name := e.Name()
			if !strings.HasSuffix(name, "-archfit.json") {
				continue
			}
			repo := strings.TrimSuffix(name, "-archfit.json")
			rd, parseErr := parseFullJSON(filepath.Join(dir, name))
			if parseErr != nil {
				fmt.Fprintf(os.Stderr, "warn: %v\n", parseErr)
				continue
			}
			result[repo] = rd
		}
	}
	return result, nil
}

func run(dir string) error {
	repoMap, err := loadRepoData(dir)
	if err != nil {
		return err
	}

	// Collect repo names for header.
	repoNames := make([]string, 0, len(repoMap))
	for k := range repoMap {
		repoNames = append(repoNames, k)
	}

	// Tally per-status counts.
	counts := map[string]int{
		statusSurfaced:        0,
		statusLLMRoutedDesign: 0,
		statusAgree:           0,
		statusNotSurfaced:     0,
	}

	type row struct {
		finding Finding
		status  string
	}
	rows := make([]row, 0, len(inventory))
	for _, f := range inventory {
		rd := repoMap[f.Repo]
		status := ClassifyFinding(f, rd)
		counts[status]++
		rows = append(rows, row{f, status})
	}

	// Emit markdown table.
	fmt.Printf("# Gap-Closure Coverage Table\n\n")
	fmt.Printf("Generated from frozen architect-only inventory (20 findings).\n")
	fmt.Printf("Input directory: `%s`  \n", dir)
	fmt.Printf("Repos loaded: %s\n\n", strings.Join(repoNames, ", "))
	fmt.Printf("**Summary:** %d surfaced · %d agree · %d llm-routed-by-design · %d not-surfaced\n\n",
		counts[statusSurfaced], counts[statusAgree], counts[statusLLMRoutedDesign], counts[statusNotSurfaced])

	fmt.Printf("| ID | Title | Repo | Category | Status |\n")
	fmt.Printf("|----|-------|------|----------|--------|\n")
	for _, r := range rows {
		fmt.Printf("| %s | %s | %s | %s | %s |\n",
			r.finding.ID,
			r.finding.Title,
			r.finding.Repo,
			r.finding.Category,
			r.status,
		)
	}
	fmt.Println()

	return nil
}

func main() {
	dir := flag.String("dir", "docs/archived/reports/eval/gap-closure", "directory containing full.json files")
	flag.Parse()

	if err := run(*dir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
