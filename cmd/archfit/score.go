package main

// ScoreCmd emits the architect-style seven-dimension banded scorecard. It is a
// convenience wrapper over check: always advisory (so coupling_balance sees the
// Balanced-Coupling edges), always report-only (a scorecard informs, it does not
// gate), and renders the scorecard format. With --base it runs a delta so the
// change_locality dimension is measured; otherwise it scans the whole repo.
type ScoreCmd struct {
	Config string `short:"c" help:"Config file." default:".archfit.yaml"`
	Base   string `help:"Git ref to compare against, enabling the change-locality dimension (e.g. main, HEAD~1)."`
	Full   bool   `help:"Scan all files, not just files changed since --base (implied when --base is absent)."`
}

func (c *ScoreCmd) Run(deps *appDeps) error {
	check := CheckCmd{
		Config:   c.Config,
		Base:     c.Base,
		Full:     c.Full || c.Base == "",
		Advisory: true,
		Report:   true,
		Format:   []string{"scorecard"},
	}
	return check.Run(deps)
}
