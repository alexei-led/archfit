package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// gitResolver adapts internal/history/git to scope. The concrete git dependency
// lives here in the composition root; scope itself stays free of process and
// tool dependencies.
type gitResolver struct {
	workDir string
	runner  toolrun.Runner
}

func (g gitResolver) RepoRoot(ctx context.Context) (string, error) {
	return git.RepoRoot(ctx, g.workDir, g.runner)
}
func (g gitResolver) HeadRef(ctx context.Context) (string, error) {
	return git.HeadRef(ctx, g.workDir, g.runner)
}
func (g gitResolver) Changed(ctx context.Context, base, head string) ([]string, error) {
	cs, err := git.Changed(ctx, g.workDir, base, head, "", g.runner)
	if err != nil {
		return nil, err
	}
	return cs.Files, nil
}

// RunContext carries the per-run path and time inputs of one pipeline run.
type RunContext struct {
	ConfigSource string
	BundleDir    string
	ScanRoot     string
	EvaluatedAt  time.Time
}

// NewRunContext builds a normal single-config run context.
func NewRunContext(configPath, root string) RunContext {
	return RunContext{ConfigSource: configPath, BundleDir: filepath.Dir(configPath), ScanRoot: root}
}

// BaseRunContext derives the base-tree context from a head run.
func BaseRunContext(head RunContext, baseRoot string) RunContext {
	head.ScanRoot = baseRoot
	return head
}
func baseRunContext(head RunContext, baseRoot string) RunContext {
	return BaseRunContext(head, baseRoot)
}

// EvaluatedAtValue returns the configured instant or samples the current time.
func (rc RunContext) EvaluatedAtValue() time.Time {
	if rc.EvaluatedAt.IsZero() {
		return time.Now()
	}
	return rc.EvaluatedAt
}
func (rc RunContext) evaluatedAt() time.Time { return rc.EvaluatedAtValue() }

// ScanDir returns the scope/git resolution anchor.
func (rc RunContext) ScanDir() string {
	if rc.ScanRoot != "" {
		return rc.ScanRoot
	}
	return rc.BundleDir
}
func (rc RunContext) scanDir() string { return rc.ScanDir() }

// OnDiskWithin returns the PathResolver's onDisk callback and rejects paths
// that escape the analyzed root before touching the filesystem.
func OnDiskWithin(root string) func(string) bool {
	return func(rel string) bool {
		osRel := filepath.FromSlash(rel)
		if !filepath.IsLocal(osRel) {
			return false
		}
		_, err := os.Stat(filepath.Join(root, osRel))
		return err == nil
	}
}

// ValidationCommand returns the shell command agent tasks should run to verify fixes.
func ValidationCommand(configPath, root string) string {
	args := []string{"archfit", "check", "-c", configPath}
	if root != "" {
		args = append(args, "--root", root)
	}
	for i := range args {
		args[i] = shellQuoteArg(args[i])
	}
	return strings.Join(args, " ")
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if !strings.ContainsAny(arg, " \t\n'\"\\$`!#&;()*<>?[\\]^{|}~") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}

const ruleIDBCCouplingGate = "bc/coupling_gate"

// RuleIDBCCouplingGate is the synthetic coupling-gate summary rule ID.
const RuleIDBCCouplingGate = ruleIDBCCouplingGate
const findingIDCouplingGate = "coupling-gate"

// FindingIDCouplingGate is the fixed synthetic coupling-gate finding ID.
const FindingIDCouplingGate = findingIDCouplingGate

// PolicyCouplingGateView adapts policy declarations to score inputs.
func PolicyCouplingGateView(snapshot policy.PolicySnapshot) score.CouplingGate {
	g := snapshot.Gates.Coupling
	return score.CouplingGate{Enabled: g.Enabled, MinBand: score.Band(g.MinBand), MaxDrop: g.MaxDrop}
}

// ApplyCouplingGate escalates a measured coupling score and promotes active
// coupling advisories into gate findings. It is a pure assessment finalizer.
func ApplyCouplingGate(diag *result.Result, card score.Scorecard, gate score.CouplingGate, base baseline.Baseline) {
	trip := score.EvaluateCouplingGate(card, gate, base.CouplingScore())
	if !trip.Tripped {
		return
	}
	diag.Verdict = result.VerdictFail
	promoted := 0
	for i := range diag.Findings {
		f := &diag.Findings[i]
		if f.RuleID != RuleIDBCImbalanced || f.Kind != finding.KindAdvisory || !score.IsActiveGateFinding(*f) {
			continue
		}
		f.Kind = finding.KindGate
		promoted++
	}
	diag.Summary.GateFindings += promoted
	diag.Summary.Warnings = max(0, diag.Summary.Warnings-promoted)
	if promoted == 0 {
		diag.Findings = append(diag.Findings, finding.Finding{ID: findingIDCouplingGate, Kind: finding.KindGate,
			RuleID: ruleIDBCCouplingGate, Status: finding.StatusNew, Severity: finding.SeverityHigh,
			Why: strings.Join(trip.Reasons, "; ")})
		diag.Summary.GateFindings++
	}
}
