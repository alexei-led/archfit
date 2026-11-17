package main

import (
	"context"
	"fmt"

	"github.com/alexei-led/archfit/v4/internal/application"
	"github.com/alexei-led/archfit/v4/internal/application/repostate"
	"github.com/alexei-led/archfit/v4/internal/config"
	"github.com/alexei-led/archfit/v4/internal/evidence/acquisition"
	"github.com/alexei-led/archfit/v4/internal/evidence/acquisition/checkfacts"
	"github.com/alexei-led/archfit/v4/internal/model/report"
	"github.com/alexei-led/archfit/v4/internal/output/jsonout"
	"github.com/alexei-led/archfit/v4/internal/output/reporttext"
	"github.com/alexei-led/archfit/v4/internal/policy/resolved"
	reportports "github.com/alexei-led/archfit/v4/internal/report/ports"
)

// ReportCmd is `archfit report`: the repository-wide picture for owners. It
// runs the measurements of the report pipeline (D2: today's analyze pipeline,
// with no verdict) and the state of the repository under a version 3 policy
// (internal/application/repostate). It never runs on the check path.
type ReportCmd struct {
	Format string `help:"Output: text or json." enum:"text,json" default:"text"`
	Dir    string `short:"C" name:"directory" help:"Run in DIR instead of the current directory." placeholder:"DIR"`
}

func (*ReportCmd) Help() string {
	return `The repository-wide picture for owners: the state of the repository and
the measurements of the code.

The state judges the whole tree with the rules and exceptions of
.archfit.yaml (version 3) on today's date:

  clean    no error outside the exceptions
  drift    an error exists that no exception covers
  expired  an exception passed its date and the code still breaks the rule

Beside the state: the places the check could not read, the exceptions close
to their date, the rules nothing checks, and the balance of each allowed
dependency. A version 2 policy is rejected: run archfit init to convert it.

The report decides nothing: it exits 0 whatever it shows.

Exit codes:
  0  done
  2  wrong usage or a tool error`
}

func (c *ReportCmd) Run(deps *appDeps) error {
	ctx := deps.runContext()
	co, err := openCheckout(ctx, deps, dirOr(c.Dir))
	if err != nil {
		return noVerdict(err)
	}
	data, found, err := co.policyAt("")
	switch {
	case err != nil:
		return noVerdict(err)
	case !found:
		return noVerdict(errNoPolicy)
	}

	in, err := config.NewReportInput(data)
	if err != nil {
		return noVerdict(err)
	}
	return c.runLoaded(ctx, deps, co, in)
}

// runLoaded continues with the original consumed policy, even if disk changes.
func (c *ReportCmd) runLoaded(ctx context.Context, deps *appDeps, co checkout, in config.ReportInput) error {
	snapshots := acquisition.ReportSnapshotSource{Root: co.root, PolicyPath: co.policyPath(""), Exclude: in.Config.RunOptions().Exclusions, Inputs: reportSnapshotInputs(in), Runner: deps.Runner}
	request := reportRequest(ctx, co, deps, in, snapshots)
	state, err := co.repositoryState(ctx, in.Policy)
	if err != nil {
		return noVerdict(err)
	}
	doc, err := co.measure(ctx, deps, in, request)
	if err != nil {
		return noVerdict(err)
	}
	doc.State.Repository = &state
	var r reportports.Renderer = reporttext.New()
	if c.Format == formatJSON {
		r = jsonout.New()
	}
	if err := r.Render(doc, deps.Stdout); err != nil {
		return noVerdict(err)
	}
	return nil
}

// repositoryState judges the working tree with the local version 3 policy.
func (co checkout) repositoryState(ctx context.Context, p resolved.Policy) (report.RepositoryState, error) {
	src := reportPolicySource{p}
	svc := repostate.Service{Policies: src, Tree: repository{co}, Facts: co.facts(checkfacts.ConfigFrom(p))}
	state, err := svc.State(ctx, co.root, co.now())
	if err != nil {
		return report.RepositoryState{}, err
	}
	protected, err := protection{co}.Protected(ctx)
	if err != nil {
		return report.RepositoryState{}, err
	}
	return state.Report(defaultConfigPath, protected), nil
}

// measure runs the report pipeline on the whole checkout, without stored references.
func (co checkout) measure(ctx context.Context, deps *appDeps, in config.ReportInput, request application.Request) (report.Document, error) {
	deps.progress = func(string) {}
	file := co.policyPath("")
	stages := newAnalysisStages(file, co.root, in.Config, deps)
	stages.Evidence = reportEvidenceService(file, co.root, in, deps)
	resp, err := application.Service{Stages: stages}.Execute(ctx, request)
	if err != nil {
		return report.Document{}, fmt.Errorf("the measurements: %w", err)
	}
	return resp.Document, nil
}
