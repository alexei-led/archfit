// Package clones provides a clone-detection runner that identifies duplicated
// code blocks across files. It is opt-in and gated by tools.clones.enabled in
// the archfit config.
//
// Supported detector: jscpd — a multi-language clone detector that supports
// Go, JS/TS, Python, Java, and many other languages. PMD CPD is documented as
// an alternative but jscpd is preferred because it ships a JSON report format
// that is easier to parse deterministically.
//
// When the tool is absent, disabled, or any non-fatal failure occurs, the
// runner returns an empty result with absent/partial coverage — never an error.
// Downstream callers (Task 12) map clusters to module pairs via an injected
// key function so this package does not import internal/metrics.
package clones

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolName      = "jscpd"
	statusOK      = "ok"
	statusPartial = "partial"
	statusAbsent  = "absent"
	clonesTimeout = 3 * time.Minute
	reportFile    = "jscpd-report.json"
	flagOutput    = "--output"

	// Absent/partial-coverage reasons: why functional-candidate (clone) detection
	// is n/a and the enable step. Static strings so a double-run stays byte-stable.
	reasonDisabled     = "clone detection is opt-in — set `tools.clones.enabled: true` in .archfit.yaml"
	reasonNotInstalled = "jscpd not found — install it (`npm install -g jscpd`) to enable clone detection"
	reasonRunFailed    = "jscpd run failed or its report was unreadable"
)

// Run invokes jscpd over root and returns detected clone clusters.
// When enabled is false, or the tool is absent, or any non-fatal failure
// occurs, it returns an empty slice with an absent/partial coverage record
// and a nil error.
func Run(ctx context.Context, runner toolrun.Runner, root string, enabled bool) ([]clone.Cluster, diagnostic.Coverage, error) {
	if !enabled {
		return nil, diagnostic.Coverage{Tool: toolName, Status: statusAbsent, Reason: reasonDisabled}, nil
	}

	if _, found := runner.Detect(ctx, toolName); !found {
		return nil, diagnostic.Coverage{Tool: toolName, Status: statusAbsent, Reason: reasonNotInstalled}, nil
	}

	tmp, err := os.MkdirTemp("", "archfit-clones-")
	if err != nil {
		return nil, diagnostic.Coverage{Tool: toolName, Status: statusAbsent, Reason: reasonRunFailed}, nil
	}
	defer os.RemoveAll(tmp) //nolint:errcheck

	partial := diagnostic.Coverage{Tool: toolName, Status: statusPartial, Reason: reasonRunFailed}

	// jscpd --reporters json --output <tmp> <root>
	// The JSON reporter writes jscpd-report.json into the output directory.
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolName,
		Args:    []string{"--reporters", "json", flagOutput, tmp, root},
		WorkDir: root,
		Timeout: clonesTimeout,
	})
	if err != nil || out.ExitCode != 0 {
		return nil, partial, nil
	}

	reportPath := filepath.Join(tmp, reportFile)
	data, err := os.ReadFile(reportPath) //nolint:gosec
	if err != nil {
		return nil, partial, nil
	}

	clusters, perr := parseJscpdReport(data)
	if perr != nil {
		return nil, partial, nil
	}

	cov := diagnostic.Coverage{
		Tool:            toolName,
		FilesSeen:       len(clusters),
		FilesApplicable: len(clusters),
		Status:          statusOK,
	}
	return clusters, cov, nil
}

// jscpdReport mirrors the JSON structure emitted by jscpd --reporters json.
// jscpd writes a "duplicates" array where each entry is a pairwise match.
type jscpdReport struct {
	Duplicates []jscpdDuplicate `json:"duplicates"`
}

type jscpdDuplicate struct {
	FirstFile  jscpdFile `json:"firstFile"`
	SecondFile jscpdFile `json:"secondFile"`
	Lines      int       `json:"lines"`
}

type jscpdFile struct {
	Name string `json:"name"`
}

// parseJscpdReport parses jscpd JSON report data into Cluster values.
// Each duplicate entry becomes one Cluster with two files and the line count.
func parseJscpdReport(data []byte) ([]clone.Cluster, error) {
	var report jscpdReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	clusters := make([]clone.Cluster, 0, len(report.Duplicates))
	for _, d := range report.Duplicates {
		if d.FirstFile.Name == "" && d.SecondFile.Name == "" {
			continue
		}
		clusters = append(clusters, clone.Cluster{
			Files: []string{d.FirstFile.Name, d.SecondFile.Name},
			Lines: d.Lines,
		})
	}
	return clusters, nil
}
