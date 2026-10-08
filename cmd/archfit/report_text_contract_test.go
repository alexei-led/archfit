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
	"unicode/utf8"

	"github.com/alexei-led/archfit/v3/internal/model/report"
	reporttest "github.com/alexei-led/archfit/v3/internal/testutil/report"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
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

// multilineRuleConfig declares a rule whose rationale is a YAML block scalar
// (embedded newlines, a U+2028, a control character, 700 runes), plus
// alternatives and docs written the same way.
func multilineRuleConfig() string {
	long := strings.Repeat("Because modules own their data. ", 22)
	return `version: 2
modules:
  billing:
    paths: ["internal/billing/**"]
    owner: team-b
    subdomain: core
  shipping:
    paths: ["internal/shipping/**"]
    owner: team-c
    subdomain: core
rules:
  - id: no_billing_from_shipping
    type: forbidden_dependency
    from: "internal/shipping/**"
    to: "internal/billing/**"
    gate: fail
    rationale: |
      First line of the rationale.

      Second paragraph` + " " + `with a separator` + "\x07" + `and a bell.
      ` + long + `
    alternatives:
      - |
        Publish an event
        instead of calling billing.
      - "Ask billing through an API"
    docs: |
      docs/adr/0001.md
      section 2
`
}

// TestCheck_MultilineRuleTextNeverBreaksTheStateContract is the v2.5.0 App
// breaker: a block-scalar rationale, alternatives and docs reach the why, the
// task constraints, the allowed alternatives, and SARIF. Every format must hold
// the consumer's one-line, bounded-length rule.
func TestCheck_MultilineRuleTextNeverBreaksTheStateContract(t *testing.T) {
	t.Parallel()
	cfgPath := writeRuleFixtureRepo(t, map[string]string{
		markerGoMod:                        fixtureShopGoMod,
		"internal/billing/domain/total.go": "package domain\n\nfunc Total() int { return 7 }\n",
		fileShippingAPIGo: "package api\n\nimport \"example.com/shop/internal/billing/domain\"\n\n" +
			"func Quote() int { return domain.Total() }\n",
		defaultConfigPath: multilineRuleConfig(),
	})
	for _, format := range []string{formatJSON, formatSarif, formatAgent, formatMarkdown, formatText} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := runArchfit(t, cmdCheck, "-c", cfgPath, "--format="+format)
			if code != 1 {
				t.Fatalf("check exit = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
			if format == formatJSON {
				violations, err := reporttest.AppTextViolations([]byte(stdout))
				if err != nil {
					t.Fatal(err)
				}
				if len(violations) > 0 {
					t.Errorf("state breaks the consumer string rules:\n%s", strings.Join(violations, "\n"))
				}
				if !strings.Contains(stdout, "First line of the rationale. Second paragraph") {
					t.Errorf("state lost the rationale text:\n%s", stdout)
				}
			}
			if format == formatSarif || format == formatAgent {
				var doc any
				if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
					t.Fatal(err)
				}
				assertSingleLineStrings(t, doc, "")
			}
		})
	}
}

func assertSingleLineStrings(t *testing.T, v any, path string) {
	t.Helper()
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			assertSingleLineStrings(t, c, path+"."+k)
		}
	case []any:
		for i, c := range x {
			assertSingleLineStrings(t, c, fmt.Sprintf("%s[%d]", path, i))
		}
	case string:
		if strings.ContainsFunc(x, func(r rune) bool { return r == '\n' || r == '\r' || r == ' ' || r == ' ' || r == '\x07' }) {
			t.Errorf("%s holds a line break or control character: %q", path, x)
		}
	}
}

// TestPolicyCanImport_MultilineRuleTextIsOneBoundedLine: the can-import answer
// carries the same rule text but never passes through the report projection.
func TestPolicyCanImport_MultilineRuleTextIsOneBoundedLine(t *testing.T) {
	t.Parallel()
	cfgPath := writeRuleFixtureRepo(t, map[string]string{
		markerGoMod:                        fixtureShopGoMod,
		"internal/billing/domain/total.go": "package domain\n\nfunc Total() int { return 7 }\n",
		fileShippingAPIGo: "package api\n\nimport \"example.com/shop/internal/billing/domain\"\n\n" +
			"func Quote() int { return domain.Total() }\n",
		defaultConfigPath: multilineRuleConfig(),
	})
	code, stdout, stderr := runArchfit(t, "policy", "can-import", "-c", cfgPath, fileShippingAPIGo, pkgBillingDom)
	if code != 1 {
		t.Fatalf("can-import exit = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var doc any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	assertSingleLineStrings(t, doc, "")
	if !strings.Contains(stdout, "First line of the rationale. Second paragraph") {
		t.Errorf("answer lost the rationale text:\n%s", stdout)
	}
	var parsed struct {
		Answers []struct {
			Denials []struct {
				Why string `json:"why"`
			} `json:"denials"`
		} `json:"answers"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatal(err)
	}
	if n := utf8.RuneCountInString(parsed.Answers[0].Denials[0].Why); n > 400 {
		t.Errorf("why is %d runes, want at most 400", n)
	}
}
