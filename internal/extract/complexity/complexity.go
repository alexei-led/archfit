// Package complexity provides a cyclomatic-complexity runner that collects
// per-function CCN values via the external lizard tool. It is opt-in and gated
// by tools.complexity.enabled in the archfit config.
//
// Supported backend: lizard — a multi-language cyclomatic complexity analyser
// that supports Go, Python, JavaScript/TypeScript, and many other languages.
// lizard is invoked directly when on PATH, or via `uvx lizard` otherwise.
//
// When the tool is absent, disabled, or any non-fatal failure occurs, the
// runner returns an empty result with absent/disabled coverage — never an error.
package complexity

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolName      = "lizard"
	statusOK      = "ok"
	statusAbsent  = "absent"
	lizardTimeout = 2 * time.Minute
)

// lizardExcludes keep tests, mocks, vendored, and generated trees out of the
// complexity scan (the same exclusions as the size-skew walk).
var lizardExcludes = []string{
	"*_test*", "*test_*", "*/tests/*", "*/__tests__/*", "*/test/*",
	"*/node_modules/*", "*/mocks/*", "*/vendor/*", "*/dist/*", "*.d.ts",
}

// Run invokes lizard over root and returns per-function complexity records.
// When enabled is false or the tool is absent, it returns an empty slice with
// an absent coverage record and a nil error, mirroring the clones.Run contract.
// Coverage carries zero file counts (FilesSeen/FilesApplicable/Unresolved all 0)
// so the coverage metric value does not shift between runs with/without the tool.
func Run(ctx context.Context, runner toolrun.Runner, root string, enabled bool) ([]signal.ComplexityFunc, diagnostic.Coverage, error) {
	absent := diagnostic.Coverage{Tool: toolName, Status: statusAbsent}
	if !enabled {
		return nil, absent, nil
	}

	name, pre := lizardCommand(ctx, runner)
	if name == "" {
		return nil, absent, nil
	}

	args := append(append([]string{}, pre...), root, "--csv")
	for _, x := range lizardExcludes {
		args = append(args, "-x", x)
	}
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: name, Args: args, WorkDir: root, Timeout: lizardTimeout})
	if err != nil || out.ExitCode != 0 {
		return nil, absent, nil
	}

	funcs := parseLizardCSV(out.Stdout, root)
	cov := diagnostic.Coverage{Tool: toolName, Status: statusOK}
	return funcs, cov, nil
}

// lizardCommand resolves how to invoke lizard: the binary directly, or via uvx.
func lizardCommand(ctx context.Context, runner toolrun.Runner) (name string, prefix []string) {
	if _, ok := runner.Detect(ctx, toolName); ok {
		return toolName, nil
	}
	if _, ok := runner.Detect(ctx, "uvx"); ok {
		return "uvx", []string{toolName}
	}
	return "", nil
}

// parseLizardCSV parses lizard --csv output into ComplexityFunc records. Columns:
// nloc, ccn, token, param, length, location, file, function, long_name, start, end.
func parseLizardCSV(data []byte, root string) []signal.ComplexityFunc {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1
	var out []signal.ComplexityFunc
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || len(rec) < 10 {
			continue
		}
		ccn, err := strconv.Atoi(strings.TrimSpace(rec[1]))
		if err != nil {
			continue
		}
		nloc, _ := strconv.Atoi(strings.TrimSpace(rec[0]))
		line, _ := strconv.Atoi(strings.TrimSpace(rec[9]))
		file := rec[6]
		if rel, rerr := filepath.Rel(root, file); rerr == nil {
			file = rel
		}
		out = append(out, signal.ComplexityFunc{
			File: filepath.ToSlash(file), Name: rec[7], CCN: ccn, NLOC: nloc, Line: line,
		})
	}
	return out
}
