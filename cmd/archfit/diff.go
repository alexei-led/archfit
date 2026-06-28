// Package main — DiffCmd implementation.
//
// DiffCmd compares the scorecard at <base-ref> against the working-tree HEAD,
// printing a before/after delta table for every dimension plus the overall score.
// It creates a clean detached git worktree at <base-ref> in a temp directory,
// runs the full advisory pipeline on each side, synthesises both scorecards from
// the resulting diagnostics, then removes the worktree.
//
// Invariants:
//   - The user's working tree is never mutated.
//   - Cleanup runs even on error paths (deferred).
//   - Non-git directory or missing/bad ref → exit 3.
//   - Both sides use the current config (the one at --config) — this isolates
//     code drift from config drift; the base ref may predate the config file.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/score"
)

// gitEnvVars lists environment variables that redirect git's internal state.
// When archfit is invoked by a CI system or git hook that sets these, inheriting
// them into worktree add/remove commands would make git operate on the wrong
// repository. Scrub all of them from the subprocess environment.
var gitEnvVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_COMMON_DIR",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
}

// cleanGitEnv returns os.Environ() with git-redirect variables removed.
func cleanGitEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	scrub := make(map[string]bool, len(gitEnvVars))
	for _, k := range gitEnvVars {
		scrub[k] = true
	}
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if !scrub[key] {
			out = append(out, kv)
		}
	}
	return out
}

// DiffCmd emits a before/after scorecard delta table between <base-ref> and HEAD.
type DiffCmd struct {
	BaseRef  string `arg:"" help:"Git ref to compare against (e.g. main, HEAD~1, v1.2.3)."`
	Config   string `short:"c" help:"Config file." default:".archfit.yaml"`
	Root     string `help:"Repository root to analyze (default: git root)." type:"path"`
	Format   string `help:"Output format: text, json, markdown." enum:"text,json,markdown" default:"text"`
	NoConfig bool   `name:"no-config" help:"Skip the config file and use built-in defaults."`
}

func (*DiffCmd) Help() string {
	return `Compare the architecture scorecard between a git ref and the current working tree.

Creates a clean detached worktree at <base-ref>, scores both sides with the full
advisory pipeline, and prints a dimension-by-dimension delta table. The format is
report-only: exit 0 on success, exit 3 on config or git errors.

Common runs:
  archfit diff main
  archfit diff HEAD~1 --format json
  archfit diff v1.0.0 --root ./services/api --format markdown`
}

// diffResult is the structured output; also used by the json format.
type diffResult struct {
	BaseRef    string     `json:"base_ref"`
	BaseBand   string     `json:"base_band"`
	BaseScore  int        `json:"base_score"`
	HeadBand   string     `json:"head_band"`
	HeadScore  int        `json:"head_score"`
	Delta      int        `json:"delta"`
	Dimensions []dimDelta `json:"dimensions"`
}

type dimDelta struct {
	Name       string `json:"name"`
	BaseValue  int    `json:"base_value"`
	BaseBand   string `json:"base_band"`
	HeadValue  int    `json:"head_value"`
	HeadBand   string `json:"head_band"`
	Delta      int    `json:"delta"`
	Confidence string `json:"confidence"`
}

// Run implements the kong command interface.
func (c *DiffCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	// 1. Resolve the git root.
	//    We need a directory to anchor `git rev-parse --show-toplevel`.
	//    Use --root when given (absolutized); otherwise fall back to the config
	//    directory — both are inside the repo and both yield the same gitRoot.
	//    gitAnchor is NOT passed to runPipeline; it is only used for git ops.
	gitAnchor := c.Root
	if gitAnchor == "" {
		gitAnchor = c.configDir()
	}
	if abs, aerr := filepath.Abs(gitAnchor); aerr == nil {
		gitAnchor = abs
	}

	gitRoot, err := git.RepoRoot(ctx, gitAnchor, deps.Runner)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: diff requires a git repository: %v", err)}
	}

	// 2. Resolve headScanRoot — the analysis boundary for the HEAD side.
	//    When --root is omitted, headScanRoot = gitRoot so diff analyses the
	//    whole repo, matching check/score (which fall through resolveScanRoot
	//    to gitRoot when cfg.Root is empty). When --root is given, use it.
	//    This root is passed to runScoreSide for the HEAD side; passing ""
	//    to runPipeline would also work (it resolves identically), but keeping
	//    an explicit value here makes the base-side mapping unambiguous.
	headScanRoot := c.Root
	if headScanRoot == "" {
		headScanRoot = gitRoot
	} else {
		if abs, aerr := filepath.Abs(headScanRoot); aerr == nil {
			headScanRoot = abs
		}
	}
	// Canonicalize symlinks so filepath.Rel works on macOS (e.g. /var/... vs /private/var/...).
	if canon, cerr := filepath.EvalSymlinks(headScanRoot); cerr == nil {
		headScanRoot = canon
	}

	// 3. Create a temporary worktree at base-ref.
	//    Point at a non-existent subdirectory so git does not complain about
	//    an already-existing directory.
	tmpBase, err := os.MkdirTemp("", "archfit-diff-*")
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: create temp dir: %v", err)}
	}
	wtDir := filepath.Join(tmpBase, "wt")

	// Cleanup: remove worktree admin entry + temp dir even on error paths.
	defer func() {
		removeWorktree(gitRoot, wtDir)
		_ = os.RemoveAll(tmpBase)
	}()

	if err := addWorktree(gitRoot, wtDir, c.BaseRef); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: cannot create worktree for ref %q: %v", c.BaseRef, err)}
	}

	// Canonicalize: on macOS MkdirTemp returns /var/... but git resolves to
	// /private/var/... — EvalSymlinks aligns them so filepath.Rel works.
	wtCanon, err := filepath.EvalSymlinks(wtDir)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: eval worktree symlinks: %v", err)}
	}

	// 4. Derive base-side root for monorepo subtree support.
	//    Map headScanRoot into the worktree using the same relative offset.
	//    When --root is omitted headScanRoot==gitRoot → rel="." → baseRoot=wtCanon
	//    (whole worktree), mirroring the HEAD side's whole-repo scan.
	baseRoot, err := subtreeInWorktree(gitRoot, headScanRoot, wtCanon)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: map subtree into worktree: %v", err)}
	}

	// 5. Score both sides via the full advisory pipeline.
	//    HEAD side: pass c.Root (possibly "") — identical to how check/score call
	//    runPipeline, so resolveScanRoot produces the same ScanRoot as check/score.
	//    Base side: pass the explicit worktree path derived above; it must be
	//    non-empty so runPipeline does not escape to the live HEAD tree.
	baseSC, err := runScoreSide(ctx, deps, c.Config, baseRoot, c.NoConfig)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: score base (%s): %v", c.BaseRef, err)}
	}
	headSC, err := runScoreSide(ctx, deps, c.Config, c.Root, c.NoConfig)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: score HEAD: %v", err)}
	}

	// 6. Build and render the delta table.
	result := buildDiffResult(c.BaseRef, baseSC, headSC)
	return c.render(deps, result)
}

// configDir returns the directory that anchors git resolution (directory of
// --config, or cwd when the path is relative and resolution fails).
func (c *DiffCmd) configDir() string {
	if filepath.IsAbs(c.Config) {
		return filepath.Dir(c.Config)
	}
	abs, err := filepath.Abs(c.Config)
	if err != nil {
		return filepath.Dir(c.Config)
	}
	return filepath.Dir(abs)
}

// runScoreSide loads config, runs the full advisory pipeline on root, and
// returns the synthesised Scorecard. Both sides share the same config file to
// isolate code drift from config drift.
func runScoreSide(ctx context.Context, deps *appDeps, configPath, root string, noConfig bool) (score.Scorecard, error) {
	cfg, err := loadConfig(ctx, configPath, noConfig)
	if err != nil {
		return score.Scorecard{}, err
	}
	mode := engine.Mode{
		Full:       true,
		Advisory:   true,
		ReportOnly: true,
	}
	diag, err := runPipeline(ctx, deps, cfg, configPath, root, noConfig, mode, baseline.Baseline{})
	if err != nil {
		return score.Scorecard{}, err
	}
	return score.Synthesize(diag), nil
}

// subtreeInWorktree maps headRoot (absolute, inside gitRoot) to its mirror
// inside wtDir. When headRoot == gitRoot the result is wtDir itself.
// Returns an error when headRoot is not under gitRoot (e.g. a ../ path).
func subtreeInWorktree(gitRoot, headRoot, wtDir string) (string, error) {
	rel, err := filepath.Rel(gitRoot, headRoot)
	if err != nil {
		return "", fmt.Errorf("rel(%s, %s): %w", gitRoot, headRoot, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("--root %s is not under git root %s", headRoot, gitRoot)
	}
	if rel == "." {
		return wtDir, nil
	}
	return filepath.Join(wtDir, rel), nil
}

// addWorktree runs `git worktree add --detach <dir> <ref>` from gitRoot.
// The subprocess environment has GIT_DIR/GIT_WORK_TREE/GIT_INDEX_FILE and
// related redirect vars scrubbed so CI/hook-set vars do not redirect the
// worktree command to the wrong repository (C3).
func addWorktree(gitRoot, dir, ref string) error {
	cmd := exec.Command("git", "worktree", "add", "--detach", dir, ref) //nolint:gosec // exec.Command passes args without shell expansion; no injection risk
	cmd.Dir = gitRoot
	cmd.Env = cleanGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

// removeWorktree removes the worktree registration cleanly. All errors are
// ignored — best-effort; os.RemoveAll on the parent temp dir follows in the
// caller's defer.
func removeWorktree(gitRoot, dir string) {
	env := cleanGitEnv()
	cmd := exec.Command("git", "worktree", "remove", "--force", dir) //nolint:gosec // dir is an archfit-controlled temp path
	cmd.Dir = gitRoot
	cmd.Env = env
	_ = cmd.Run()
	// Prune in case RemoveAll ran before the remove command.
	prune := exec.Command("git", "worktree", "prune")
	prune.Dir = gitRoot
	prune.Env = env
	_ = prune.Run()
}

// buildDiffResult constructs the structured delta from two synthesised scorecards.
func buildDiffResult(baseRef string, base, head score.Scorecard) diffResult {
	res := diffResult{
		BaseRef:   baseRef,
		BaseBand:  string(base.OverallBand),
		BaseScore: base.Overall,
		HeadBand:  string(head.OverallBand),
		HeadScore: head.Overall,
		Delta:     head.Overall - base.Overall,
	}

	baseByName := make(map[string]score.Dimension, len(base.Dimensions))
	for _, d := range base.Dimensions {
		baseByName[d.Name] = d
	}

	for _, hd := range head.Dimensions {
		bd := baseByName[hd.Name]
		res.Dimensions = append(res.Dimensions, dimDelta{
			Name:       hd.Name,
			BaseValue:  bd.Value,
			BaseBand:   string(bd.Band),
			HeadValue:  hd.Value,
			HeadBand:   string(hd.Band),
			Delta:      hd.Value - bd.Value,
			Confidence: string(hd.Confidence),
		})
	}
	return res
}

// render writes the diff result in the requested format.
func (c *DiffCmd) render(deps *appDeps, res diffResult) error {
	switch c.Format {
	case formatJSON:
		enc := json.NewEncoder(deps.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	case formatMarkdown:
		return renderDiffMarkdown(deps, res)
	default: // formatText
		return renderDiffText(deps, res)
	}
}

// signedInt formats v with a leading "+" when positive.
func signedInt(v int) string {
	if v > 0 {
		return "+" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

func renderDiffText(deps *appDeps, res diffResult) error {
	// Write to a strings.Builder (never errors) then flush once to the real writer.
	var sb strings.Builder
	fmt.Fprintf(&sb, "archfit diff  base: %s → HEAD\n\n", res.BaseRef)
	fmt.Fprintf(&sb, "%-40s  %s → %s  %s\n", "Overall",
		fmt.Sprintf("%d (%s)", res.BaseScore, res.BaseBand),
		fmt.Sprintf("%d (%s)", res.HeadScore, res.HeadBand),
		signedInt(res.Delta))
	fmt.Fprintf(&sb, "\n%-40s  %-16s  %-16s  %-6s  %s\n",
		"Dimension", "Base", "Head", "Delta", "Confidence")
	fmt.Fprintf(&sb, "%-40s  %-16s  %-16s  %-6s  %s\n",
		strings.Repeat("-", 40), strings.Repeat("-", 16), strings.Repeat("-", 16), "------", "----------")
	for _, d := range res.Dimensions {
		fmt.Fprintf(&sb, "%-40s  %-16s  %-16s  %-6s  %s\n",
			d.Name,
			fmt.Sprintf("%d (%s)", d.BaseValue, d.BaseBand),
			fmt.Sprintf("%d (%s)", d.HeadValue, d.HeadBand),
			signedInt(d.Delta),
			d.Confidence)
	}
	_, err := io.WriteString(deps.Stdout, sb.String())
	return err
}

func renderDiffMarkdown(deps *appDeps, res diffResult) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# archfit diff: `%s` → HEAD\n\n", res.BaseRef)
	fmt.Fprintf(&sb, "**Overall:** %d/100 (%s) → %d/100 (%s) — **%s**\n\n",
		res.BaseScore, res.BaseBand, res.HeadScore, res.HeadBand, signedInt(res.Delta))
	fmt.Fprintf(&sb, "| Dimension | Base | Head | Δ | Confidence |\n")
	fmt.Fprintf(&sb, "|-----------|------|------|---|------------|\n")
	for _, d := range res.Dimensions {
		fmt.Fprintf(&sb, "| %s | %d (%s) | %d (%s) | %s | %s |\n",
			d.Name, d.BaseValue, d.BaseBand, d.HeadValue, d.HeadBand, signedInt(d.Delta), d.Confidence)
	}
	_, err := io.WriteString(deps.Stdout, sb.String())
	return err
}
