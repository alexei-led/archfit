package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/alexei-led/archfit/v3/internal/application"
	"github.com/alexei-led/archfit/v3/internal/baseline"
)

// BaselineCmd runs the engine and saves findings as the new baseline.
type BaselineCmd struct {
	Config       string `short:"c" help:"Config file." default:".archfit.yaml"`
	Root         string `short:"r" help:"Repository root to analyze (default: directory of --config)." type:"path"`
	NoAdvisories bool   `name:"no-advisories" help:"Exclude advisory findings from the baseline: Balanced-Coupling advisories and violations of gate: warn rules."`
	Refresh      bool   `name:"refresh" help:"Re-run all extractors and refresh the cache. Use after installing or updating analyzer tools."`
	Reanchor     bool   `name:"reanchor" help:"Carry the stored baseline's accepted debt into this engine's epoch and accept nothing new."`
	From         string `name:"from" help:"Stored baseline to re-anchor from (default: the baseline beside --config). Requires --reanchor." type:"path"`
}

func (*BaselineCmd) Help() string {
	return `Use baseline after reviewing current findings so CI can block only new architecture drift.

It records one full baseline of the tree as checked out. There is no git-base mode; compare against a ref with ` + "`archfit check --base`" + ` instead.

Typical calibration:
  archfit check --config .archfit.yaml
  archfit baseline --config .archfit.yaml
  archfit check --config .archfit.yaml --base origin/main`
}

func (c *BaselineCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := loadConfig(ctx, c.Config)
	if err != nil {
		return configLoadError(err)
	}
	deps.refresh = c.Refresh
	bPath := filepath.Join(filepath.Dir(c.Config), defaultBaselinePath)
	req := application.BaselineRequest{ConfigPath: c.Config, Root: c.Root, Path: bPath, NoAdvisories: c.NoAdvisories}
	if c.From != "" && !c.Reanchor {
		return &exitError{code: 3, msg: "error: --from requires --reanchor"}
	}
	if c.Reanchor {
		from := c.From
		if from == "" {
			from = bPath
		}
		stored, err := loadStoredBaseline(ctx, from)
		if err != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("error: %v (a first capture runs without --reanchor)", err)}
		}
		req.Reanchor = &stored
	}
	service := application.BaselineService{Stages: newAnalysisStages(c.Config, c.Root, cfg, deps), Writer: baselineWriterAdapter{}}
	resp, err := service.Execute(ctx, req)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	_, _ = fmt.Fprintf(deps.Stdout, "baseline saved: %s\n", bPath)
	if resp.SkippedWaived > 0 {
		_, _ = fmt.Fprintf(deps.Stdout, "skipped %d finding(s) covered by temporary waivers, including expired waivers; not accepted as permanent debt\n", resp.SkippedWaived)
	}
	if resp.Reanchor != nil {
		writeReanchorReport(deps.Stdout, *resp.Reanchor)
	}
	return nil
}

// loadStoredBaseline reads the file a re-anchor carries forward.
func loadStoredBaseline(ctx context.Context, path string) (application.StoredBaseline, error) {
	b, err := baseline.LoadForReanchor(ctx, path)
	if err != nil {
		return application.StoredBaseline{}, err
	}
	out := application.StoredBaseline{Metrics: b.Metrics, State: applicationBaseline(b).State}
	for _, a := range b.Accepted {
		out.Accepted = append(out.Accepted, application.BaselineFinding{Fingerprint: a.Fingerprint, RuleID: a.RuleID, Kind: a.Kind, Severity: a.Severity})
	}
	if b.State != nil {
		out.QualifyingSeamIDs = b.State.QualifyingSeamIDs
	}
	return out, nil
}

// writeReanchorReport prints what a re-anchor kept and what it left for review.
// Every list is complete: the reviewer of the baseline change must see each
// dropped and each unaccepted finding.
func writeReanchorReport(w io.Writer, r application.ReanchorReport) {
	_, _ = fmt.Fprintf(w, "re-anchor: kept %d accepted entries, dropped %d, left %d current finding(s) unaccepted\n",
		r.Kept, len(r.Dropped), len(r.NotAccepted))
	for _, reason := range r.DriftReasons {
		_, _ = fmt.Fprintf(w, "  drift: %s\n", reason)
	}
	for _, d := range r.Dropped {
		_, _ = fmt.Fprintf(w, "  dropped: %s %s\n", d.RuleID, d.Fingerprint)
	}
	for _, n := range r.NotAccepted {
		_, _ = fmt.Fprintf(w, "  not accepted: %s %s %s -> %s %s\n", n.RuleID, n.ID, orDash(n.FromModule), orDash(n.ToModule), n.Path)
	}
	for _, id := range r.DroppedSeams {
		_, _ = fmt.Fprintf(w, "  seam no longer qualifies: %s\n", id)
	}
	for _, id := range r.NewSeams {
		_, _ = fmt.Fprintf(w, "  seam qualifies only now (stays new): %s\n", id)
	}
	for _, m := range r.WorsenedMetrics {
		_, _ = fmt.Fprintf(w, "  metric worsened: %s %g -> %g (the new baseline records %g)\n", m.Name, m.Before, m.After, m.After)
	}
}

// orDash names an endpoint no declared module owns.
func orDash(module string) string {
	if module == "" {
		return "-"
	}
	return module
}
