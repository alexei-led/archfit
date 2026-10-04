package main

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/state"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	ruleIDDomainNoClock = "domain_no_clock"
	fileDomainService   = "internal/domain/service.go"
	toolSG              = "sg"
)

// clockFixtureFiles is a Go repository whose domain module reads the wall clock
// in production code (service.go:6), in a test, and outside the rule's scope.
func clockFixtureFiles(cfg string) map[string]string {
	return map[string]string{
		markerGoMod: fixtureShopGoMod,
		fileDomainService: "package domain\n\nimport \"time\"\n\n" +
			"func Stamp() int64 {\n\treturn time.Now().Unix()\n}\n",
		"internal/domain/service_test.go": "package domain\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\n" +
			"func TestStamp(t *testing.T) { _ = time.Now() }\n",
		"internal/app/handler.go": "package app\n\nimport \"time\"\n\n" +
			"func Now() time.Time {\n\treturn time.Now()\n}\n",
		defaultConfigPath: cfg,
	}
}

const clockFixtureConfig = `version: 2
modules:
  domain:
    paths: ["internal/domain/**"]
    owner: team-d
    subdomain: core
  app:
    paths: ["internal/app/**"]
    owner: team-a
    subdomain: supporting
rules:
  - id: domain_no_clock
    type: forbidden_pattern
    from: "internal/domain/**"
    gate: fail
    patterns:
      - id: clock
        lang: go
        rule: "time.Now()"
`

// sgPatternFakeRunner answers ast-grep pattern runs with canned matches, so the
// forbidden_pattern pipeline is proven without an ast-grep binary (CI runs the
// unit suite before it installs one). Every other command reaches the real
// runner.
type sgPatternFakeRunner struct {
	real    toolrun.Runner
	matches []byte
}

func (r sgPatternFakeRunner) Detect(ctx context.Context, tool string) (toolrun.ToolInfo, bool) {
	if tool == toolSG {
		return toolrun.ToolInfo{Name: toolSG}, true
	}
	return r.real.Detect(ctx, tool)
}

func (r sgPatternFakeRunner) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	if cmd.Name != toolSG {
		return r.real.Run(ctx, cmd)
	}
	switch {
	case slices.Contains(cmd.Args, flagVersion):
		return toolrun.Output{Stdout: []byte("ast-grep 0.44.0\n")}, nil
	case slices.Contains(cmd.Args, "--pattern"):
		return toolrun.Output{Stdout: r.matches}, nil
	}
	return toolrun.Output{ExitCode: 2, Stderr: []byte("unexpected sg call")}, nil
}

func (r sgPatternFakeRunner) Stream(ctx context.Context, cmd toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
	if cmd.Name == toolSG {
		return toolrun.Output{}, consume(strings.NewReader(""))
	}
	return r.real.Stream(ctx, cmd, consume)
}

// sgMatchJSON renders ast-grep's --json shape for one time.Now() match (0-based line).
func sgMatchJSON(file string, line0 int) map[string]any {
	return map[string]any{
		"text": "time.Now()", "file": file, "language": "Go",
		"range": map[string]any{"start": map[string]any{"line": line0, "column": 8}, "end": map[string]any{"line": line0, "column": 18}},
	}
}

// TestPipeline_ForbiddenPatternBlocksAProductionHit is the deterministic proof
// that a pattern hit fails the gate: the domain's production clock read blocks
// at service.go:6, while the same construct in a test file and outside from:
// does not, and the rule counts as evaluated.
func TestPipeline_ForbiddenPatternBlocksAProductionHit(t *testing.T) {
	t.Parallel()
	cfgPath := writeRuleFixtureRepo(t, clockFixtureFiles(clockFixtureConfig))
	root := filepath.Dir(cfgPath)
	matches, err := json.Marshal([]map[string]any{
		sgMatchJSON("internal/app/handler.go", 5),
		sgMatchJSON(fileDomainService, 5),
		sgMatchJSON("internal/domain/service_test.go", 7),
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(context.Background(), cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	deps := &appDeps{Runner: sgPatternFakeRunner{real: toolrun.New(), matches: matches}, Stdout: io.Discard, Stderr: io.Discard}
	diag, _, err := runPipeline(context.Background(), deps, cfg, cfgPath, root)
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	if diag.State.Decision.HardGates != state.HardGateFail {
		t.Errorf("hard gates = %q, want fail", diag.State.Decision.HardGates)
	}
	for _, rule := range diag.State.Decision.UnevaluatedRequiredRules {
		if rule.RuleID == ruleIDDomainNoClock {
			t.Errorf("forbidden_pattern listed unevaluated: %s", rule.Reason)
		}
	}
	var hits []string
	for _, f := range diag.Findings {
		if f.RuleID != ruleIDDomainNoClock {
			continue
		}
		for _, loc := range f.Locations {
			hits = append(hits, loc.File+":"+strconv.Itoa(loc.Line)+":"+f.Kind+":"+string(f.Status))
		}
		if f.Edge.From.Module != "domain" || f.Edge.Kind != "pattern_match" {
			t.Errorf("finding edge = %+v, want the domain file with kind pattern_match", f.Edge)
		}
	}
	if want := []string{fileDomainService + ":6:gate:new"}; !slices.Equal(hits, want) {
		t.Errorf("pattern hits = %v, want %v", hits, want)
	}
}

// TestRun_Check_ForbiddenPatternFiresWithRealAstGrep runs the same fixture
// through the real ast-grep binary when one is installed.
func TestRun_Check_ForbiddenPatternFiresWithRealAstGrep(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess integration test — skipped with -short")
	}
	sgPath, err := exec.LookPath(toolSG)
	if err != nil {
		t.Skip("sg not found on PATH")
	}
	if out, err := exec.Command(sgPath, flagVersion).CombinedOutput(); err != nil || !strings.Contains(string(out), "ast-grep") { //nolint:gosec // path from LookPath
		t.Skipf("sg at %s is not ast-grep", sgPath)
	}
	t.Parallel()
	code, state := checkStateOf(t, writeRuleFixtureRepo(t, clockFixtureFiles(clockFixtureConfig)))
	if code != 1 {
		t.Fatalf("check exit = %d, want 1 (production clock read in the domain)", code)
	}
	got := findingsOf(state, ruleIDDomainNoClock)
	if len(got) != 1 || len(got[0].Locations) != 1 || got[0].Locations[0].File != fileDomainService || got[0].Locations[0].Line != 6 {
		t.Fatalf("forbidden_pattern findings = %+v, want exactly %s:6", got, fileDomainService)
	}
}

// TestRun_Check_WarnsOnPatternsOutsideForbiddenPattern pins the v2.4 posture
// end to end: a config whose patterns sit on another rule type still loads and
// runs, and check says on stderr that those matches can never fire.
func TestRun_Check_WarnsOnPatternsOutsideForbiddenPattern(t *testing.T) {
	t.Parallel()
	cfg := strings.Replace(clockFixtureConfig, "type: forbidden_pattern\n", "type: forbidden_dependency\n    to: \"internal/app/**\"\n", 1)
	code, _, stderr := runArchfit(t, cmdCheck, "-c", writeRuleFixtureRepo(t, clockFixtureFiles(cfg)))
	if code == 3 {
		t.Fatalf("check exit 3: patterns on another rule type must still load\nstderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, `warning: rule "domain_no_clock" (type forbidden_dependency) declares patterns`) {
		t.Errorf("stderr lacks the patterns warning:\n%s", stderr)
	}
}

// TestPipeline_ForbiddenPatternIgnoresOutOfScopeFiles pins forbidden_pattern to
// the declared analysis scope. The LOC walk and the ast-grep scan both ignore
// exclude:, so a hit in an excluded tree or in a switched-off language reaches
// the rule as a classified production file; rule scope already drops those
// files, and the rule must not fire on them either.
func TestPipeline_ForbiddenPatternIgnoresOutOfScopeFiles(t *testing.T) {
	const (
		ruleID     = "no_clock"
		legacyFile = "legacy/old.go"
		ruleBlock  = `rules:
  - id: no_clock
    type: forbidden_pattern
    from: "**"
    gate: fail
    patterns:
      - id: clock
        lang: go
        rule: "time.Now()"
`
	)
	clockRead := "package legacy\n\nimport \"time\"\n\nfunc Stamp() int64 {\n\treturn time.Now().Unix()\n}\n"
	tests := []struct {
		name    string
		config  string
		files   map[string]string
		hitFile string
	}{
		{
			name:   "excluded tree",
			config: "version: 2\nexclude: [\"legacy/**\"]\n" + ruleBlock,
			files: map[string]string{
				markerGoMod:       fixtureShopGoMod,
				legacyFile:        clockRead,
				fileDomainService: "package domain\n\nfunc Answer() int { return 42 }\n",
			},
			hitFile: legacyFile,
		},
		{
			name:   "switched-off language",
			config: "version: 2\nlanguages:\n  go:\n    enabled: false\n" + ruleBlock,
			files: map[string]string{
				markerGoMod:  fixtureShopGoMod,
				legacyFile:   clockRead,
				"web/app.ts": "export const answer = 42;\n",
			},
			hitFile: legacyFile,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := tc.files
			files[defaultConfigPath] = tc.config
			cfgPath := writeRuleFixtureRepo(t, files)
			matches, err := json.Marshal([]map[string]any{sgMatchJSON(tc.hitFile, 5)})
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := loadConfig(context.Background(), cfgPath)
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			deps := &appDeps{Runner: sgPatternFakeRunner{real: toolrun.New(), matches: matches}, Stdout: io.Discard, Stderr: io.Discard}
			diag, _, err := runPipeline(context.Background(), deps, cfg, cfgPath, filepath.Dir(cfgPath))
			if err != nil {
				t.Fatalf("pipeline: %v", err)
			}
			for _, f := range diag.Findings {
				if f.RuleID == ruleID {
					t.Errorf("forbidden_pattern fired outside the declared scope: %+v", f.Locations)
				}
			}
			for _, rule := range diag.State.Decision.UnevaluatedRequiredRules {
				if rule.RuleID == ruleID {
					t.Errorf("forbidden_pattern listed unevaluated: %s", rule.Reason)
				}
			}
		})
	}
}
