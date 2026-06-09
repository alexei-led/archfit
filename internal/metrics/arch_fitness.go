package metrics

import (
	"fmt"
	"strings"

	"github.com/alexei-led/archfit/internal/fitness"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// architectureFitnessDefinition is the human-readable definition string.
const architectureFitnessDefinition = "fraction of architecture-enforcement signals present " +
	"(arch tests, import-linter config, arch-linter in CI)"

// archFitnessSignalCount is the total number of enforcement signal categories.
const archFitnessSignalCount = 3

// ArchitectureFitnessMetric reports whether architecture intent is actively enforced
// in the repository. It scans for three categories of enforcement evidence:
//
//  1. Architecture test files (files naming arch rules, or importing arch-test libs).
//  2. Import-linter configuration (.importlinter, setup.cfg, pyproject.toml).
//  3. Arch-linter in CI (CI workflow referencing archfit/import-linter/deptry/etc.).
//
// Scoring: each present signal contributes 1/3 to the raw fraction (0–1); the
// display score scales to 10. Band is always "info" (report-only; never gates).
// n/a is reported only when FitnessSignals.EvidencePaths is nil, meaning the
// scan was never run (e.g. zero-value MetricInput from a cmd that skipped it).
type ArchitectureFitnessMetric struct{}

// Name returns "architecture_fitness".
func (m ArchitectureFitnessMetric) Name() string { return "architecture_fitness" }

// Version returns "architecture_fitness.v1".
func (m ArchitectureFitnessMetric) Version() string { return "architecture_fitness.v1" }

// Calculate scores the enforcement level from the FitnessSignals in MetricInput.
//
// n/a when EvidencePaths is nil (scan was never run).
// 0/10 when no signals are present but the scan did run (honest zero).
// 10/10 when all three signal categories are present.
func (m ArchitectureFitnessMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	// nil EvidencePaths = scan never ran → n/a, not a false zero.
	if in.FitnessSignals.EvidencePaths == nil {
		return naCount(m.Name(), m.Version(), architectureFitnessDefinition)
	}

	sig := in.FitnessSignals
	present := countSignals(sig)
	value := float64(present) / float64(archFitnessSignalCount)
	score := value * 10.0

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    fitnessDisplay(score, present, sig),
		Band:       bandInformational,
		Confidence: confidenceHigh,
		Version:    m.Version(),
		Mode:       modeRatio,
		Definition: architectureFitnessDefinition,
	}
}

// countSignals returns the number of enforcement signal categories that are present.
func countSignals(sig fitness.Signals) int {
	n := 0
	if sig.ArchTestFiles {
		n++
	}
	if sig.ImportLinterConfig {
		n++
	}
	if sig.ArchLinterInCI {
		n++
	}
	return n
}

// fitnessDisplay builds the human-readable display string.
// It includes the score, signal count, and matched evidence paths for explainability.
func fitnessDisplay(score float64, present int, sig fitness.Signals) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%.1f/10 (%d/%d signals)", score, present, archFitnessSignalCount)

	// Append evidence paths grouped by category, in a stable order.
	type catEvidence struct {
		label string
		paths []string
	}
	cats := []catEvidence{
		{"arch_tests", sig.EvidencePaths["arch_test_files"]},
		{"import_linter", sig.EvidencePaths["import_linter_config"]},
		{"ci_linter", sig.EvidencePaths["arch_linter_in_ci"]},
	}
	for _, c := range cats {
		if len(c.paths) == 0 {
			continue
		}
		b.WriteString("; ")
		b.WriteString(c.label)
		b.WriteString(": ")
		// Show up to 3 paths per category; truncate the rest.
		const maxPaths = 3
		shown := c.paths
		extra := 0
		if len(shown) > maxPaths {
			extra = len(shown) - maxPaths
			shown = shown[:maxPaths]
		}
		b.WriteString(strings.Join(shown, ", "))
		if extra > 0 {
			fmt.Fprintf(&b, " (+%d more)", extra)
		}
	}
	return b.String()
}
