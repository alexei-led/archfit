// Package main is a standalone scorer-calibration tool.
// It compares coupling.AdditiveScorer and coupling.MultiplicativeScorer over
// real or synthetic repos and writes a per-repo agreement report to JSON.
// This binary is never shipped to users; it is a scorer-development tool.
//
// Usage:
//
//	calibrate --repo /path/to/repo [--repo /other] --output calibration-report.json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/calibrate"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/scope"
)

const defaultConfigName = ".archfit.yaml"

// repoFlag is a repeatable string flag (--repo can appear multiple times).
type repoFlag []string

func (r *repoFlag) String() string { return fmt.Sprint([]string(*r)) }
func (r *repoFlag) Set(v string) error {
	*r = append(*r, v)
	return nil
}

func main() {
	var repos repoFlag
	output := flag.String("output", "calibration-report.json", "Output file for JSON report array.")
	flag.Var(&repos, "repo", "Repo path to analyse (repeatable). Skips paths that don't exist.")
	flag.Parse()

	ctx := context.Background()
	var reports []calibrate.Report

	for _, repoPath := range repos {
		if _, err := os.Stat(repoPath); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "calibrate: skipping %q: %v\n", repoPath, err)
			continue
		}

		absPath, err := filepath.Abs(repoPath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "calibrate: skipping %q: abs path: %v\n", repoPath, err)
			continue
		}

		report, err := calibrateRepo(ctx, absPath)
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
		_, _ = fmt.Fprintf(os.Stderr, "calibrate: marshal: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil { //nolint:gosec // calibration output file
		_, _ = fmt.Fprintf(os.Stderr, "calibrate: write %q: %v\n", *output, err)
		os.Exit(1)
	}
	fmt.Printf("calibration report written to %s\n", *output)
}

// calibrateRepo extracts facts from the Go packages in absPath, classifies
// them, and runs the two scorers to produce an agreement report.
func calibrateRepo(ctx context.Context, absPath string) (calibrate.Report, error) {
	cfgPath := filepath.Join(absPath, defaultConfigName)
	cfg, err := loadConfig(ctx, cfgPath)
	if err != nil {
		return calibrate.Report{}, err
	}

	extractCfg := cfg.ForExtract("go")
	// Override Src so the Go extractor loads from the target repo, not cwd.
	extractCfg.Src = absPath

	ex := golang.New(extractCfg)
	sc := scope.Scope{Root: absPath}

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

// loadConfig loads the config at cfgPath. Falls back to config.Default() when
// the file is absent (the repo may not have an .archfit.yaml yet).
func loadConfig(ctx context.Context, cfgPath string) (config.Config, error) {
	cfg, err := config.Load(ctx, cfgPath)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return config.Default(), nil
	}
	return config.Config{}, err
}
