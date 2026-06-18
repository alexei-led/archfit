package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/calibrate"
	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/scope"
)

// CalibrateCmd compares AdditiveScorer and MultiplicativeScorer over real or
// synthetic coupling graphs and writes per-repo agreement reports to JSON.
// This is a development tool — it never affects the check gate.
type CalibrateCmd struct {
	Repo   []string `help:"Repo paths to analyse (repeatable). Skips paths that don't exist." short:"r"`
	Output string   `help:"Output file for JSON report array." default:"calibration-report.json" short:"o"`
}

// Run implements the kong command interface.
func (cmd *CalibrateCmd) Run(deps *appDeps) error {
	ctx := context.Background()
	var reports []calibrate.Report

	for _, repoPath := range cmd.Repo {
		if _, err := os.Stat(repoPath); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "calibrate: skipping %q: %v\n", repoPath, err)
			continue
		}

		absPath, err := filepath.Abs(repoPath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "calibrate: skipping %q: abs path: %v\n", repoPath, err)
			continue
		}

		report, err := calibrateRepo(ctx, deps, absPath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "calibrate: skipping %q: %v\n", absPath, err)
			continue
		}

		fmt.Printf("repo=%s edges=%d agree=%d rate=%.2f\n",
			absPath, report.EdgeCount, report.AgreeCount, report.BandAgreementRate())
		reports = append(reports, report)
	}

	if len(reports) == 0 {
		reports = []calibrate.Report{}
	}

	data, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return fmt.Errorf("calibrate: marshal: %w", err)
	}
	if err := os.WriteFile(cmd.Output, data, 0o644); err != nil { //nolint:gosec // calibration output file
		return fmt.Errorf("calibrate: write %q: %w", cmd.Output, err)
	}
	fmt.Printf("calibration report written to %s\n", cmd.Output)
	return nil
}

// calibrateRepo extracts facts from the Go packages in absPath, classifies
// them, and runs the two scorers to produce an agreement report.
func calibrateRepo(ctx context.Context, _ *appDeps, absPath string) (calibrate.Report, error) {
	cfgPath := filepath.Join(absPath, defaultConfigPath)
	cfg, err := loadConfig(ctx, cfgPath, false)
	if err != nil {
		// File missing is fine — loadConfig falls back to Default() only for
		// the process-cwd defaultConfigPath. For arbitrary repos we load
		// explicitly; fall back to Default() on any not-exist error.
		if os.IsNotExist(err) {
			cfg, err = loadConfig(ctx, cfgPath, true)
		}
		if err != nil {
			return calibrate.Report{}, err
		}
	}

	extractCfg := cfg.ForExtract("go")
	// Override Src so the Go extractor loads from the target repo, not cwd.
	extractCfg.Src = absPath

	ex := golang.New(extractCfg)
	sc := scope.Scope{Root: absPath}

	// Use NopSymbolResolver — no subprocess symbol resolution needed here.
	_ = ports.NopSymbolResolver{}

	facts, _, err := ex.Extract(ctx, sc)
	if err != nil {
		return calibrate.Report{}, fmt.Errorf("extract: %w", err)
	}

	g := graph.Build([]graph.Facts{facts})
	classifyCfg := cfg.ForClassify()
	idx := classify.Run(g, classifyCfg)

	return calibrate.Compare(
		absPath,
		idx,
		coupling.AdditiveScorer{},
		coupling.MultiplicativeScorer{},
	), nil
}
