package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
	reporttest "github.com/alexei-led/archfit/internal/testutil/report"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// depcruiseCrashRunner is a bunx launcher whose dependency-cruiser run exits 1
// with two kilobytes of multi-line stderr — storybook's Svelte 5 parse failure.
// Every other tool is absent, so the run is hermetic.
type depcruiseCrashRunner struct {
	real   toolrun.Runner
	stderr string
}

func (r depcruiseCrashRunner) Detect(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
	if tool == "bunx" {
		return toolrun.ToolInfo{Name: tool, Path: "/usr/local/bin/bunx"}, true
	}
	return toolrun.ToolInfo{}, false
}

func (r depcruiseCrashRunner) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	if cmd.Name != "bunx" {
		return r.real.Run(ctx, cmd)
	}
	if slices.Contains(cmd.Args, flagVersion) {
		return toolrun.Output{Stdout: []byte("16.10.4\n")}, nil
	}
	return toolrun.Output{ExitCode: 1, Stderr: []byte(r.stderr)}, nil
}

func (r depcruiseCrashRunner) Stream(ctx context.Context, cmd toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
	return r.real.Stream(ctx, cmd, consume)
}

const depcruiseCrashConfig = `version: 2
modules:
  web:
    paths: ["src/web/**"]
    owner: team-web
  core:
    paths: ["src/core/**"]
    owner: team-core
languages:
  typescript:
    enabled: %s
rules:
  - id: no-module-cycles
    type: module_cycle
    gate: fail
`

// TestCheck_AnalyzerStderrNeverBreaksTheStateContract is the storybook B
// repro: an auto-mode analyzer that exits non-zero with multi-line stderr. The
// published state passes the consumer's string rules — the coverage reason and
// the unevaluated-rule reason built from it are one bounded line each — and the
// raw output is on stderr.
func TestCheck_AnalyzerStderrNeverBreaksTheStateContract(t *testing.T) {
	t.Parallel()
	var raw strings.Builder
	raw.WriteString("ERROR: Extracting dependencies ran afoul of...\n\n  Cannot use `await` in a non-async context\n")
	for raw.Len() < 2048 {
		raw.WriteString("https://svelte.dev/e/experimental_async\n... in src/web/Async.svelte\n")
	}
	const firstLine = "dependency-cruiser exited 1: ERROR: Extracting dependencies ran afoul of... Cannot use `await`"
	for _, tc := range []struct{ mode, wantPrefix string }{
		{mode: "auto", wantPrefix: firstLine},                  // a coverage row the extractor built
		{mode: "true", wantPrefix: "extract/ts: " + firstLine}, // an extractor error acquisition turned into a row
	} {
		t.Run("typescript "+tc.mode, func(t *testing.T) {
			t.Parallel()
			cfgPath := writeRuleFixtureRepo(t, map[string]string{
				"package.json":      `{"name":"shop","private":true}` + "\n",
				"src/web/page.ts":   "import { price } from '../core/price';\nexport const page = price;\n",
				"src/core/price.ts": "export const price = 1;\n",
				defaultConfigPath:   fmt.Sprintf(depcruiseCrashConfig, tc.mode),
			})
			var stdout, stderr bytes.Buffer
			deps := &appDeps{Runner: depcruiseCrashRunner{real: toolrun.New(), stderr: raw.String()}, Stdout: &stdout, Stderr: &stderr}
			err := (&CheckCmd{Config: cfgPath, Root: filepath.Dir(cfgPath), JSON: true, Progress: "none"}).Run(deps)
			var exit *exitError
			if err != nil && (!errors.As(err, &exit) || exit.code == 3) {
				t.Fatalf("check: %v\nstderr:\n%s", err, stderr.String())
			}

			violations, decodeErr := reporttest.AppTextViolations(stdout.Bytes())
			if decodeErr != nil {
				t.Fatalf("%v\nstdout:\n%s", decodeErr, stdout.String())
			}
			if len(violations) > 0 {
				t.Errorf("published state breaks the consumer string rules:\n%s", strings.Join(violations, "\n"))
			}
			var st report.ArchitectureState
			if err := json.Unmarshal(stdout.Bytes(), &st); err != nil {
				t.Fatal(err)
			}
			var reason string
			for _, tool := range st.Coverage.Tools {
				if tool.Tool == "dependency-cruiser" {
					reason = tool.Reason
				}
			}
			if !strings.HasPrefix(reason, tc.wantPrefix) {
				t.Errorf("dependency-cruiser reason = %q, want prefix %q", reason, tc.wantPrefix)
			}
			if len(st.Decision.UnevaluatedRequiredRules) != 1 || !strings.Contains(st.Decision.UnevaluatedRequiredRules[0].Reason, "dependency-cruiser exited 1") {
				t.Errorf("unevaluated rules = %+v, want the module cycle rule citing the crashed analyzer", st.Decision.UnevaluatedRequiredRules)
			}
			if !strings.Contains(stderr.String(), strings.TrimRight(raw.String(), "\n")) {
				t.Errorf("stderr does not carry the raw analyzer output:\n%s", stderr.String())
			}
		})
	}
}
