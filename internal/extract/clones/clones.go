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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolName      = "jscpd"
	clonesTimeout = 3 * time.Minute

	// defaultTimeout is the per-analyzer outer watchdog applied when no
	// tools.clones.timeout is configured. It is intentionally generous (well
	// above the per-subprocess clonesTimeout) to guard only against pathological
	// hangs — not normal runs.
	defaultTimeout = 5 * time.Minute

	reportFile = "jscpd-report.json"
	flagOutput = "--output"
	flagIgnore = "--ignore"

	// Coverage reasons: why functional-candidate (clone) detection is n/a.
	// Static strings so a double-run stays byte-stable.
	// reasonDisabled is used when tools.clones.enabled is off — the tool may be
	// installed, but the user deliberately opted out. The action is to enable it
	// in config, NOT to install it. reasonNotInstalled is used when the tool is
	// genuinely absent from the environment.
	reasonDisabled     = "clone detection disabled by config — set `tools.clones.enabled: on` in .archfit.yaml to enable"
	reasonNotInstalled = "jscpd not found — install it (`npm install -g jscpd`) to enable clone detection"
	reasonRunFailed    = "jscpd run failed or its report was unreadable"
	reasonTimedOut     = "clone detection timed out — increase tools.clones.timeout or reduce the scope"
)

// Run invokes jscpd over root and returns detected clone clusters.
// exclusions is the effective set of glob patterns (scope.MergeExclusions result)
// that jscpd should skip via --ignore; empty means no --ignore flag is added
// (byte-identical to before this change). Best-effort: jscpd-only. scip-go
// cannot honor file-level exclusions because it indexes via the Go build system,
// not a file list — Task 12's per-analyzer timeout is its guard.
// timeout is the per-analyzer outer watchdog; 0 uses defaultTimeout. When
// enabled is false, or the tool is absent, or any non-fatal failure occurs, it
// returns an empty slice with an absent/partial coverage record and a nil error.
// On timeout it returns StatusTimedOut coverage and a nil error — the run continues.
func Run(ctx context.Context, runner toolrun.Runner, root string, enabled bool, timeout time.Duration, exclusions []string) ([]clone.Cluster, diagnostic.Coverage, error) {
	if !enabled {
		// Disabled by config — tool may or may not be installed. Report as
		// disabled (not absent) so the pipeline does not generate an "install"
		// coverage gap for a deliberate opt-out.
		return nil, diagnostic.Coverage{Tool: toolName, Status: diagnostic.StatusDisabled, Reason: reasonDisabled}, nil
	}

	if _, found := runner.Detect(ctx, toolName); !found {
		return nil, diagnostic.Coverage{Tool: toolName, Status: diagnostic.StatusAbsent, Reason: reasonNotInstalled}, nil
	}

	// Apply per-analyzer watchdog. The outer timeout caps total clone-detection
	// time (including jscpd startup + scan). On deadline: return n/a (timed out)
	// and let the overall run continue.
	to := timeout
	if to <= 0 {
		to = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	tmp, err := os.MkdirTemp("", "archfit-clones-")
	if err != nil {
		return nil, diagnostic.Coverage{Tool: toolName, Status: diagnostic.StatusAbsent, Reason: reasonRunFailed}, nil
	}
	defer os.RemoveAll(tmp) //nolint:errcheck

	partial := diagnostic.Coverage{Tool: toolName, Status: diagnostic.StatusPartial, Reason: reasonRunFailed}

	// Build jscpd args: --reporters json --output <tmp> [--ignore "<globs>"] <root>
	// --ignore accepts a comma-separated list of glob patterns. Only added when
	// exclusions are configured — no exclusions → byte-identical to before.
	args := []string{"--reporters", "json", flagOutput, tmp}
	if len(exclusions) > 0 {
		args = append(args, flagIgnore, strings.Join(exclusions, ","))
	}
	args = append(args, root)

	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolName,
		Args:    args,
		WorkDir: root,
		Timeout: clonesTimeout,
	})
	if err != nil || out.ExitCode != 0 {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, diagnostic.Coverage{Tool: toolName, Status: diagnostic.StatusTimedOut, Reason: reasonTimedOut}, nil
		}
		return nil, partial, nil
	}

	reportPath := filepath.Join(tmp, reportFile)
	data, err := os.ReadFile(reportPath) //nolint:gosec
	if err != nil {
		return nil, partial, nil
	}

	clusters, filesScanned, perr := parseJscpdReport(data)
	if perr != nil {
		return nil, partial, nil
	}

	// FilesSeen/FilesApplicable are the source files jscpd scanned (its
	// statistics.total.sources), not the clone-pair count — a repo with 200
	// files and 4 clone pairs covered 200 files, not 4.
	cov := diagnostic.Coverage{
		Tool:            toolName,
		FilesSeen:       filesScanned,
		FilesApplicable: filesScanned,
		Status:          diagnostic.StatusOK,
	}
	return clusters, cov, nil
}

// jscpdReport mirrors the JSON structure emitted by jscpd --reporters json.
// jscpd writes a "duplicates" array where each entry is a pairwise match, plus a
// "statistics" block whose total.sources is the number of files scanned.
type jscpdReport struct {
	Duplicates []jscpdDuplicate `json:"duplicates"`
	Statistics struct {
		Total struct {
			Sources int `json:"sources"`
		} `json:"total"`
	} `json:"statistics"`
}

type jscpdDuplicate struct {
	FirstFile  jscpdFile `json:"firstFile"`
	SecondFile jscpdFile `json:"secondFile"`
	Lines      int       `json:"lines"`
}

type jscpdFile struct {
	Name string `json:"name"`
}

// parseJscpdReport parses jscpd JSON report data into Cluster values and the
// number of source files jscpd scanned (statistics.total.sources). Each duplicate
// entry becomes one Cluster with two files and the line count.
func parseJscpdReport(data []byte) ([]clone.Cluster, int, error) {
	var report jscpdReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, 0, err
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
	return clusters, report.Statistics.Total.Sources, nil
}
