package metrics_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// Evidence-map key constants — must match the category names used by the
// fitness package and by arch_fitness.go's display logic.
const (
	catArchTestFiles  = "arch_test_files"
	catImportLinter   = "import_linter_config"
	catArchLinterInCI = "arch_linter_in_ci"
)

// Display-label constants — must match the labels used by fitnessDisplay.
const (
	labelArchTests    = "arch_tests"
	labelImportLinter = "import_linter"
	labelCILinter     = "ci_linter"
)

// Band constants reused from the package-level test file.
const (
	bandInfoStr = "info"
)

// makeSignals builds a signal.Signals from explicit booleans.
// The EvidencePaths map is always non-nil (as Detect always initialises it),
// representing "scan ran" — even if no signals are present.
func makeSignals(archTests, importLinter, ciLinter bool) signal.Signals {
	ep := signal.EvidenceMap{
		catArchTestFiles:  nil,
		catImportLinter:   nil,
		catArchLinterInCI: nil,
	}
	if archTests {
		ep[catArchTestFiles] = []string{"internal/arch_test.go"}
	}
	if importLinter {
		ep[catImportLinter] = []string{".importlinter"}
	}
	if ciLinter {
		ep[catArchLinterInCI] = []string{".github/workflows/ci.yml"}
	}
	return signal.Signals{
		ArchTestFiles:      archTests,
		ImportLinterConfig: importLinter,
		ArchLinterInCI:     ciLinter,
		EvidencePaths:      ep,
	}
}

func TestArchitectureFitnessMetric_Calculate(t *testing.T) {
	m := metrics.ArchitectureFitnessMetric{}

	tests := []struct {
		name            string
		signals         signal.Signals
		wantBand        string
		wantNA          bool
		wantValueApprox float64  // expected value (within 0.01)
		wantDisplayHas  []string // substrings the display must contain
	}{
		{
			name:     "n/a when scan never ran (nil EvidencePaths)",
			signals:  signal.Signals{}, // zero value: EvidencePaths == nil
			wantNA:   true,
			wantBand: bandNAStr,
		},
		{
			name:            "no enforcement signals → low score (0/10)",
			signals:         makeSignals(false, false, false),
			wantBand:        bandInfoStr,
			wantValueApprox: 0.0,
			wantDisplayHas:  []string{"0.0/10", "0/3"},
		},
		{
			name:            "one signal → partial score (3.3/10)",
			signals:         makeSignals(true, false, false),
			wantBand:        bandInfoStr,
			wantValueApprox: 1.0 / 3.0,
			wantDisplayHas:  []string{"3.3/10", "1/3", labelArchTests},
		},
		{
			name:            "two signals → mid score (6.7/10)",
			signals:         makeSignals(true, true, false),
			wantBand:        bandInfoStr,
			wantValueApprox: 2.0 / 3.0,
			wantDisplayHas:  []string{"6.7/10", "2/3", labelArchTests, labelImportLinter},
		},
		{
			name:            "all three signals → full score (10.0/10)",
			signals:         makeSignals(true, true, true),
			wantBand:        bandInfoStr,
			wantValueApprox: 1.0,
			wantDisplayHas:  []string{"10.0/10", "3/3", labelArchTests, labelImportLinter, labelCILinter},
		},
		{
			name:            "ci only → partial score",
			signals:         makeSignals(false, false, true),
			wantBand:        bandInfoStr,
			wantValueApprox: 1.0 / 3.0,
			wantDisplayHas:  []string{"3.3/10", "1/3", labelCILinter},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := metrics.MetricInput{
				FitnessSignals: tc.signals,
			}
			result := m.Calculate(in)

			if result.Band != tc.wantBand {
				t.Errorf("Band = %q, want %q", result.Band, tc.wantBand)
			}
			if tc.wantNA {
				if result.Display != bandNAStr {
					t.Errorf("Display = %q, want %q", result.Display, bandNAStr)
				}
				if result.Delta != nil {
					t.Errorf("Delta should be nil for n/a result")
				}
				return
			}
			// Non-n/a cases.
			if diff := result.Value - tc.wantValueApprox; diff < -0.01 || diff > 0.01 {
				t.Errorf("Value = %.4f, want ~%.4f", result.Value, tc.wantValueApprox)
			}
			for _, sub := range tc.wantDisplayHas {
				if !strings.Contains(result.Display, sub) {
					t.Errorf("Display %q missing substring %q", result.Display, sub)
				}
			}
			if result.Name != "architecture_fitness" {
				t.Errorf("Name = %q, want %q", result.Name, "architecture_fitness")
			}
			if result.Version != "architecture_fitness.v1" {
				t.Errorf("Version = %q, want %q", result.Version, "architecture_fitness.v1")
			}
		})
	}
}

// TestArchitectureFitnessMetric_EvidencePathsInDisplay verifies that matched
// evidence paths appear in the display string.
func TestArchitectureFitnessMetric_EvidencePathsInDisplay(t *testing.T) {
	m := metrics.ArchitectureFitnessMetric{}

	ep := signal.EvidenceMap{
		catArchTestFiles:  []string{"internal/arch_test.go", "internal/layer_test.go"},
		catImportLinter:   nil,
		catArchLinterInCI: nil,
	}
	sig := signal.Signals{
		ArchTestFiles: true,
		EvidencePaths: ep,
	}
	result := m.Calculate(metrics.MetricInput{FitnessSignals: sig})

	if !strings.Contains(result.Display, "internal/arch_test.go") {
		t.Errorf("Display %q missing evidence path %q", result.Display, "internal/arch_test.go")
	}
	if !strings.Contains(result.Display, "internal/layer_test.go") {
		t.Errorf("Display %q missing evidence path %q", result.Display, "internal/layer_test.go")
	}
}

// TestArchitectureFitnessMetric_TruncatesLongEvidenceLists verifies that more
// than 3 evidence paths per category are truncated with a "+N more" suffix.
func TestArchitectureFitnessMetric_TruncatesLongEvidenceLists(t *testing.T) {
	m := metrics.ArchitectureFitnessMetric{}

	paths := []string{"a_test.go", "b_test.go", "c_test.go", "d_test.go", "e_test.go"}
	ep := signal.EvidenceMap{
		catArchTestFiles:  paths,
		catImportLinter:   nil,
		catArchLinterInCI: nil,
	}
	sig := signal.Signals{ArchTestFiles: true, EvidencePaths: ep}
	result := m.Calculate(metrics.MetricInput{FitnessSignals: sig})

	if !strings.Contains(result.Display, "+2 more") {
		t.Errorf("Display %q should contain '+2 more' for 5 paths (max 3)", result.Display)
	}
}
