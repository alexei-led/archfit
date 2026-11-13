package main

import (
	"context"
	"fmt"

	"github.com/alexei-led/archfit/v3/internal/application"
	"github.com/alexei-led/archfit/v3/internal/application/repostate"
	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition/checkfacts"
	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/output/jsonout"
	"github.com/alexei-led/archfit/v3/internal/output/reporttext"
	reportports "github.com/alexei-led/archfit/v3/internal/report/ports"
)

// ReportCmd is `archfit report`: the repository-wide picture for owners. It
// runs the measurements of the report pipeline (D2: today's analyze pipeline,
// with no verdict) and, on a version 3 policy, the state of the repository
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
dependency. A version 2 policy gives the measurements only: run archfit init.

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

	var cfg config.Config
	var repo *report.RepositoryState
	if isVersion3(data) {
		// The state comes first: a policy that does not fit the tree stops
		// the report before the slow measurements.
		state, err := co.repositoryState(ctx)
		if err != nil {
			return noVerdict(err)
		}
		repo = &state
		if cfg, err = config.ReportConfig(data); err != nil {
			return noVerdict(err)
		}
	} else if cfg, err = loadAnalysisConfig(ctx, co.policyPath("")); err != nil {
		return noVerdict(err)
	}

	doc, err := co.measure(ctx, deps, cfg)
	if err != nil {
		return noVerdict(err)
	}
	doc.State.Repository = repo
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
func (co checkout) repositoryState(ctx context.Context) (report.RepositoryState, error) {
	src := localPolicy{co}
	p, err := src.Policy(ctx)
	if err != nil {
		return report.RepositoryState{}, err
	}
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
func (co checkout) measure(ctx context.Context, deps *appDeps, cfg config.Config) (report.Document, error) {
	deps.progress = func(string) {}
	file := co.policyPath("")
	resp, err := application.Service{Stages: newAnalysisStages(file, co.root, cfg, deps)}.Execute(ctx, application.Request{
		ConfigSource: file, BundleDir: co.root, Formats: []string{formatJSON},
		Identity: identityInputs(ctx, deps.Runner, file, co.root, co.root, ""),
	})
	if err != nil {
		return report.Document{}, fmt.Errorf("the measurements: %w", err)
	}
	return resp.Document, nil
}
