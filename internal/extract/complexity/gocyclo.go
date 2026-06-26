package complexity

import (
	"bufio"
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	gocycloTool    = "gocyclo"
	gocycloTimeout = 2 * time.Minute
)

// runGocyclo invokes gocyclo over root and returns per-function CCN records.
// Returns (nil, false) when gocyclo is not on PATH or the run fails.
// gocyclo's exit code is the number of functions above the threshold — ignored.
func runGocyclo(ctx context.Context, runner toolrun.Runner, root string) ([]signal.ComplexityFunc, bool) {
	if _, ok := runner.Detect(ctx, gocycloTool); !ok {
		return nil, false
	}
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gocycloTool,
		Args:    []string{"-over", "0", root},
		WorkDir: root,
		Timeout: gocycloTimeout,
	})
	if err != nil {
		// Binary present but run failed — treat as absent; don't propagate.
		return nil, true
	}
	return parseGocycloOutput(out.Stdout, root), true
}

// parseGocycloOutput parses gocyclo output lines into ComplexityFunc records.
// Each line: <ccn> <pkg> <func> <file>:<line>:<col>
// NLOC is 0 — gocyclo has no NLOC; the pure-Go loc signal covers size.
func parseGocycloOutput(data []byte, root string) []signal.ComplexityFunc {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var out []signal.ComplexityFunc
	for scanner.Scan() {
		if f, ok := parseGocycloLine(scanner.Text(), root); ok {
			out = append(out, f)
		}
	}
	return out
}

// parseGocycloLine parses one gocyclo output line.
// Format: <ccn> <pkg> <func> <file>:<line>:<col>
// Method names appear as "(*Receiver).MethodName".
// The file path is the third-to-last element: strip last two ":N" suffixes.
func parseGocycloLine(line, root string) (signal.ComplexityFunc, bool) {
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return signal.ComplexityFunc{}, false
	}
	ccn, err := strconv.Atoi(parts[0])
	if err != nil {
		return signal.ComplexityFunc{}, false
	}
	funcName := parts[2]
	pos := parts[len(parts)-1] // <file>:<line>:<col>

	// Strip column: "<file>:<line>:<col>" → "<file>:<line>"
	lastColon := strings.LastIndex(pos, ":")
	if lastColon < 0 {
		return signal.ComplexityFunc{}, false
	}
	fileAndLine := pos[:lastColon]

	// Strip line: "<file>:<line>" → "<file>"
	secondColon := strings.LastIndex(fileAndLine, ":")
	if secondColon < 0 {
		return signal.ComplexityFunc{}, false
	}
	filePath := fileAndLine[:secondColon]
	lineStr := fileAndLine[secondColon+1:]
	lineNum, err := strconv.Atoi(lineStr)
	if err != nil {
		return signal.ComplexityFunc{}, false
	}

	if rel, rerr := filepath.Rel(root, filePath); rerr == nil {
		filePath = rel
	}
	return signal.ComplexityFunc{
		File: filepath.ToSlash(filePath),
		Name: funcName,
		CCN:  ccn,
		NLOC: 0,
		Line: lineNum,
	}, true
}
