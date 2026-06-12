package main

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// lizardTimeout bounds the complexity tool run (generous for a first-run uvx fetch).
const lizardTimeout = 2 * time.Minute

// lizardExcludes keep tests, mocks, vendored, and generated trees out of the
// complexity scan (the same exclusions as the size-skew walk).
var lizardExcludes = []string{
	"*_test*", "*test_*", "*/tests/*", "*/__tests__/*", "*/test/*",
	"*/node_modules/*", "*/mocks/*", "*/vendor/*", "*/dist/*", "*.d.ts",
}

// complexityFuncs runs the external multi-language complexity tool (lizard) over
// root and returns per-function cyclomatic complexity. lizard is reused as-is
// (archfit is a meta-linter, not a reimplementation): it is found on PATH, else
// via `uvx lizard`. Best-effort: if neither is available or the run fails, returns
// nil so the complexity metric reports n/a.
func complexityFuncs(ctx context.Context, runner toolrun.Runner, root string) []signal.ComplexityFunc {
	name, pre := lizardCommand(ctx, runner)
	if name == "" {
		return nil
	}
	args := append(append([]string{}, pre...), root, "--csv")
	for _, x := range lizardExcludes {
		args = append(args, "-x", x)
	}
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: name, Args: args, WorkDir: root, Timeout: lizardTimeout})
	if err != nil || out.ExitCode != 0 {
		return nil
	}
	return parseLizardCSV(out.Stdout, root)
}

// lizardCommand resolves how to invoke lizard: the binary directly, or via uvx.
func lizardCommand(ctx context.Context, runner toolrun.Runner) (name string, prefix []string) {
	if _, ok := runner.Detect(ctx, "lizard"); ok {
		return "lizard", nil
	}
	if _, ok := runner.Detect(ctx, "uvx"); ok {
		return "uvx", []string{"lizard"}
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
