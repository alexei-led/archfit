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
	"sort"
	"time"

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
)

// Cluster represents a group of files that share a duplicated code block.
type Cluster struct {
	// Files contains the repo-relative (or absolute) paths involved in the clone.
	Files []string
	// Lines is the number of duplicated lines in the block.
	Lines int
}

// ModulePairs converts a slice of Clusters to canonical cross-module pairs
// using an injected key function. Same-module pairs are skipped. Each pair is
// returned in sorted order ([a,b] where a <= b) and the result slice is deduped
// and sorted for determinism.
//
// The key function is typically fileToModuleKey(file, lang) from the metrics
// package, injected at the call site so this package stays free of that import.
func ModulePairs(clusters []Cluster, key func(string) string) [][2]string {
	seen := make(map[[2]string]struct{})
	for _, c := range clusters {
		// Collect distinct module keys across all files in this cluster.
		mods := make(map[string]struct{})
		for _, f := range c.Files {
			if k := key(f); k != "" {
				mods[k] = struct{}{}
			}
		}
		// Emit all cross-module pairs from this cluster.
		keys := make([]string, 0, len(mods))
		for k := range mods {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				pair := [2]string{keys[i], keys[j]}
				seen[pair] = struct{}{}
			}
		}
	}

	out := make([][2]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// Run invokes jscpd over root and returns detected clone clusters.
// When enabled is false, or the tool is absent, or any non-fatal failure
// occurs, it returns an empty slice with an absent/partial coverage record
// and a nil error.
func Run(ctx context.Context, runner toolrun.Runner, root string, enabled bool) ([]Cluster, diagnostic.Coverage, error) {
	absent := diagnostic.Coverage{Tool: toolName, Status: statusAbsent}
	if !enabled {
		return nil, absent, nil
	}

	if _, found := runner.Detect(ctx, toolName); !found {
		return nil, absent, nil
	}

	tmp, err := os.MkdirTemp("", "archfit-clones-")
	if err != nil {
		return nil, absent, nil
	}
	defer os.RemoveAll(tmp) //nolint:errcheck

	partial := diagnostic.Coverage{Tool: toolName, Status: statusPartial}

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
func parseJscpdReport(data []byte) ([]Cluster, error) {
	var report jscpdReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	clusters := make([]Cluster, 0, len(report.Duplicates))
	for _, d := range report.Duplicates {
		if d.FirstFile.Name == "" && d.SecondFile.Name == "" {
			continue
		}
		clusters = append(clusters, Cluster{
			Files: []string{d.FirstFile.Name, d.SecondFile.Name},
			Lines: d.Lines,
		})
	}
	return clusters, nil
}
