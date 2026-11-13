package main

// CheckCmd is the status of the change against the version 3 policy.
type CheckCmd struct {
	Base    string   `help:"The target branch, such as origin/main. Default: the remote's default branch (origin/HEAD)." default:"auto" placeholder:"REF"`
	Policy  string   `help:"Preview the change with this policy file, without comparing it with the target branch's policy." placeholder:"FILE"`
	Format  []string `name:"format" help:"Output: text (default), json or sarif." enum:"text,json,sarif"`
	Verbose bool     `help:"Add the policy digest and the engine version."`
	Dir     string   `short:"C" name:"directory" help:"Run in DIR instead of the current directory." placeholder:"DIR"`
}

func (*CheckCmd) Help() string {
	return `The status of the change in the working tree: what it adds that a rule of
.archfit.yaml forbids. The change is compared with its merge base with the
target branch, and the policy with the policy of the target branch.

Output: one record per error on stdout, then the status line. Progress and
tool errors go to stderr.

Exit codes:
  0  pass
  1  fail
  2  unknown (no verdict), wrong usage, or a tool error

Common runs:
  archfit check
  archfit check --base origin/main
  archfit check --policy candidate.yaml     # preview a policy change
  archfit check --format json
  archfit check --format sarif > archfit.sarif

`
}

func (c *CheckCmd) Run(deps *appDeps) error {
	return c.runChange(deps)
}
