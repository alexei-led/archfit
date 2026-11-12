package main

// CheckCmd is `archfit check`. During milestone 1 it reads the policy version
// first (D1): a version 3 policy runs the check of the change (change_check.go);
// no policy file, or a version 2 one, runs the Archfit 3 path (runScan) with its
// flags and exit codes unchanged. The flags of the Archfit 3 path are hidden
// from the help.
type CheckCmd struct {
	Base    string   `help:"The target branch, such as origin/main. The change is the working tree against its merge base with that branch. Default: the remote's default branch (origin/HEAD)." default:"auto" placeholder:"REF"`
	Policy  string   `help:"Judge the change with this policy file instead of .archfit.yaml, without comparing it with the policy of the target branch. A preview: archfit request cannot use its result." placeholder:"FILE"`
	Format  []string `name:"format" help:"Output: text (default), json or sarif." enum:"json,text,markdown,md,sarif,scorecard,agent"`
	Verbose bool     `help:"Add the policy digest and the engine version."`
	Dir     string   `short:"C" name:"directory" help:"Run in DIR instead of the current directory." placeholder:"DIR"`

	// The flags of the Archfit 3 path, for a version 2 policy.
	Config string `short:"c" hidden:"" help:"Path to the version 2 policy file (default .archfit.yaml)."`
	Root   string `hidden:"" help:"Repository root to read (default: directory of --config)." type:"path"`
	Target string `hidden:"" help:"Branch the change lands on (version 2 policy)."`

	NoAdvisories bool     `name:"no-advisories" hidden:"" help:"Drop coupling advisories (version 2 policy)."`
	MinSeverity  string   `name:"min-severity" hidden:"" help:"Minimum advisory severity: low, medium, high, critical (version 2 policy)." enum:"low,medium,high,critical," default:""`
	Refresh      bool     `name:"refresh" hidden:"" help:"Re-run all extractors and refresh the cache (version 2 policy)."`
	RequireTools bool     `name:"require-tools" hidden:"" help:"Exit non-zero when any required tool is missing (version 2 policy)."`
	Lang         []string `name:"lang" hidden:"" help:"Language to force on: go, typescript (ts), python (py), rust (rs), dotnet (csharp). Repeatable (version 2 policy)."`

	JSON     bool   `name:"json" hidden:"" help:"Shorthand for --format json (version 2 policy)."`
	Markdown bool   `name:"markdown" hidden:"" help:"Shorthand for --format markdown (version 2 policy)."`
	Sarif    bool   `name:"sarif" hidden:"" help:"Shorthand for --format sarif (version 2 policy)."`
	Progress string `name:"progress" hidden:"" help:"Progress on stderr: auto, plain, none (version 2 policy)." enum:"auto,plain,none," default:""`
	Quiet    bool   `short:"q" hidden:"" help:"No progress output (version 2 policy)."`
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

A version 2 policy (Archfit 3) runs the Archfit 3 check, with its own flags
and exit codes, until the repository is converted with archfit init.`
}

func (c *CheckCmd) Run(deps *appDeps) error {
	if c.Policy == "" {
		v2, err := c.version2(deps)
		if err != nil {
			return err
		}
		if v2 {
			return c.runVersion2(deps)
		}
	}
	return c.runChange(deps)
}

// runVersion2 is the Archfit 3 check.
func (c *CheckCmd) runVersion2(deps *appDeps) error {
	if c.Dir != "" || c.Verbose {
		return &exitError{code: 3, msg: "archfit: -C and --verbose need a version 3 policy"}
	}
	// A linter prints its output and nothing else: progress only on request.
	progress := c.Progress
	if progress == "" {
		progress = progressNone
	}
	configPath := c.Config
	if configPath == "" {
		configPath = defaultConfigPath
	}
	return runScan(deps.runContext(), deps, scanRequest{
		configPath:   configPath,
		root:         c.Root,
		baseRef:      c.Base,
		target:       c.Target,
		json:         c.JSON,
		markdown:     c.Markdown,
		sarif:        c.Sarif,
		formats:      c.Format,
		noAdvisories: c.NoAdvisories,
		minSeverity:  c.MinSeverity,
		lang:         c.Lang,
		requireTools: c.RequireTools,
		progress:     progress,
		lint:         true,
		quiet:        c.Quiet,
		refresh:      c.Refresh,
		reportOnly:   false,
	})
}
