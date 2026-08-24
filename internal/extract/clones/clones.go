// Package clones provides a clone-detection runner that identifies duplicated
// code blocks across files. It is opt-in and gated by analyzers.clones.enabled in
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

	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolName      = "jscpd"
	clonesTimeout = 3 * time.Minute

	// cacheAnalyzer is the fact-cache subdirectory (fact-cache.md D1) — the
	// analyzer name, not the tool binary name.
	cacheAnalyzer = "clones"

	// defaultTimeout is the per-analyzer outer watchdog applied when no
	// analyzers.clones.timeout is configured. It is intentionally generous (well
	// above the per-subprocess clonesTimeout) to guard only against pathological
	// hangs — not normal runs.
	defaultTimeout = 5 * time.Minute

	reportFile = "jscpd-report.json"
	flagOutput = "--output"
	flagIgnore = "--ignore"

	// Coverage reasons: why functional-candidate (clone) detection is n/a.
	// Static strings so a double-run stays byte-stable.
	// reasonDisabled is used when analyzers.clones.enabled is off — the tool may be
	// installed, but the user deliberately opted out. The action is to enable it
	// in config, NOT to install it. reasonNotInstalled is used when the tool is
	// genuinely absent from the environment.
	reasonDisabled     = "clone detection disabled by config — set `analyzers.clones.enabled: true` in .archfit.yaml to enable"
	reasonNotInstalled = "jscpd not found — install it (`npm install -g jscpd`) to enable clone detection"
	reasonRunFailed    = "jscpd run failed or its report was unreadable"
	reasonTimedOut     = "clone detection timed out — increase analyzers.clones.timeout or reduce the scope"
)

// effectiveTimeout returns configured when it is non-zero, else fallback.
// This lets a configured analyzers.clones.timeout extend or shorten the inner
// per-subprocess cap (not just the outer watchdog).
func effectiveTimeout(configured, fallback time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	return fallback
}

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
//
// cache is the extractor fact cache (nil = off). jscpd writes its report to a
// temp file, not stdout, so the seam is store-direct: the cached fact is the
// PARSED result (clusters + files-scanned). Only StatusOK results are cached —
// a timed-out or failed run must re-run next time (fact-cache.md D3).
func Run(ctx context.Context, runner toolrun.Runner, root string, enabled bool, timeout time.Duration, exclusions []string, cache *factcache.Store) ([]clone.Cluster, evidence.Coverage, error) {
	if !enabled {
		// Disabled by config — tool may or may not be installed. Report as
		// disabled (not absent) so the pipeline does not generate an "install"
		// coverage gap for a deliberate opt-out.
		return nil, evidence.Coverage{Tool: toolName, Status: evidence.StatusDisabled, Reason: reasonDisabled}, nil
	}

	if _, found := runner.Detect(ctx, toolName); !found {
		return nil, evidence.Coverage{Tool: toolName, Status: evidence.StatusAbsent, Reason: reasonNotInstalled}, nil
	}

	key := cacheKey(ctx, runner, root, exclusions, cache)
	if key != "" {
		if blob, ok := cache.Get(cacheAnalyzer, key); ok {
			var ce cacheEntry
			if json.Unmarshal(blob, &ce) == nil {
				return ce.Clusters, okCoverage(ce.FilesScanned), nil
			}
		}
	}

	// Apply per-analyzer watchdog. The outer timeout caps total clone-detection
	// time (including jscpd startup + scan). On deadline: return n/a (timed out)
	// and let the overall run continue.
	ctx, cancel := toolrun.WithWatchdog(ctx, timeout, defaultTimeout)
	defer cancel()

	tmp, err := os.MkdirTemp("", "archfit-clones-")
	if err != nil {
		return nil, evidence.Coverage{Tool: toolName, Status: evidence.StatusAbsent, Reason: reasonRunFailed}, nil
	}
	defer os.RemoveAll(tmp) //nolint:errcheck

	partial := evidence.Coverage{Tool: toolName, Status: evidence.StatusPartial, Reason: reasonRunFailed}

	// Build jscpd args: --reporters json --output <tmp> [--ignore "<globs>"] <root>
	// --ignore accepts a comma-separated list of glob patterns. Only added when
	// exclusions are configured — no exclusions → byte-identical to before.
	args := []string{"--reporters", "json", flagOutput, tmp}
	if len(exclusions) > 0 {
		args = append(args, flagIgnore, strings.Join(exclusions, ","))
	}
	args = append(args, root)

	_, err = runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolName,
		Args:    args,
		WorkDir: root,
		Timeout: effectiveTimeout(timeout, clonesTimeout),
	})
	// Check both the inner per-subprocess deadline (err) and the outer watchdog
	// (ctx.Err()). When the inner timeout fires, runner.Run returns
	// context.DeadlineExceeded as err but ctx.Err() is nil.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, evidence.Coverage{Tool: toolName, Status: evidence.StatusTimedOut, Reason: reasonTimedOut}, nil
	}
	// jscpd (npm ≤3.x) exits 1 when it finds duplicates — a non-zero exit is NOT
	// a fatal failure. Always try to read the report from disk; only treat as
	// partial when the report is missing or unparseable. Hard errors (exec
	// failures, deadline) are handled above. We ignore ExitCode intentionally.
	if err != nil {
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

	if key != "" {
		if blob, merr := json.Marshal(cacheEntry{Clusters: clusters, FilesScanned: filesScanned}); merr == nil {
			cache.Put(cacheAnalyzer, key, blob)
		}
	}
	return clusters, okCoverage(filesScanned), nil
}

// okCoverage builds the StatusOK coverage record shared by the live and
// cached paths — the two must stay byte-identical.
// FilesSeen/FilesApplicable are the source files jscpd scanned (its
// statistics.total.sources), not the clone-pair count — a repo with 200
// files and 4 clone pairs covered 200 files, not 4.
func okCoverage(filesScanned int) evidence.Coverage {
	return evidence.Coverage{
		Tool:            toolName,
		FilesSeen:       filesScanned,
		FilesApplicable: filesScanned,
		Status:          evidence.StatusOK,
	}
}

// cacheEntry is the stored fact-cache envelope: the parsed jscpd result.
type cacheEntry struct {
	Clusters     []clone.Cluster `json:"clusters,omitempty"`
	FilesScanned int             `json:"files_scanned"`
}

// cacheKey derives the fact-cache key for one jscpd scan, or "" when the
// cache is off or key material cannot be derived (never an error). Key
// inputs: jscpd version probe, the scan root + exclusion set (the report may
// embed tree-specific paths, and --ignore changes the scan), and the content
// hash of every non-excluded file under root — jscpd is multi-language, so
// the input scope is the whole tree.
func cacheKey(ctx context.Context, runner toolrun.Runner, root string, exclusions []string, cache *factcache.Store) string {
	if cache == nil {
		return ""
	}
	cfgHash, err := factcache.HashJSON(struct {
		Root       string
		Exclusions []string
	}{root, exclusions})
	if err != nil {
		return ""
	}
	files := factcache.ListInputs(root, factcache.MatchAll, exclusions)
	treeHash, err := factcache.HashTree(root, files)
	if err != nil {
		return ""
	}
	return factcache.Key(cacheAnalyzer, jscpdVersion(ctx, runner), cfgHash, treeHash)
}

// jscpdVersion probes `jscpd --version`. Best-effort: "" on any failure.
func jscpdVersion(ctx context.Context, runner toolrun.Runner) string {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolName,
		Args:    []string{"--version"},
		Timeout: 30 * time.Second,
	})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(out.Stdout))
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

// jscpdFile mirrors one side (firstFile/secondFile) of a jscpd duplicate entry.
// Start/End are the 1-based line numbers bounding the duplicated fragment
// within this file, as reported by jscpd's JSON reporter.
type jscpdFile struct {
	Name  string `json:"name"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// parseJscpdReport parses jscpd JSON report data into Cluster values and the
// number of source files jscpd scanned (statistics.total.sources). Each duplicate
// entry becomes one Cluster with two files, their line-location ranges, and the
// duplicated line count.
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
			Locations: []clone.LineRange{
				{StartLine: d.FirstFile.Start, EndLine: d.FirstFile.End},
				{StartLine: d.SecondFile.Start, EndLine: d.SecondFile.End},
			},
		})
	}
	return clusters, report.Statistics.Total.Sources, nil
}
