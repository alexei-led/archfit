package main

import (
	"github.com/alexei-led/archfit/v3/internal/application/canquery"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/output/record"
)

// CanCmd is `archfit can <from> <to>` (internal/application/canquery): may
// this dependency or library be used. It reads a version 3 policy.
type CanCmd struct {
	From    string `arg:"" optional:"" help:"The code that would depend: a file, a directory or a module."`
	To      string `arg:"" optional:"" help:"What it would use: a file, a directory, a module, a Go package or a library."`
	Format  string `help:"Output: text or json." enum:"text,json" default:"text"`
	Verbose bool   `help:"Add the policy digest and the engine version."`
	Dir     string `short:"C" name:"directory" help:"Run in DIR instead of the current directory." placeholder:"DIR"`
}

func (*CanCmd) Help() string {
	return `Ask before writing code: may <from> depend on <to>? The answer applies the
rules archfit check applies to one new dependency.

Each side is a module name, a file or a directory (relative to the repository
root), or a Go import path; <to> can also be a library.

Exit codes:
  0  yes
  1  no: the answer names the rule and the next step
  2  unknown (a path in no module), wrong usage, or a tool error

The answer does not apply exceptions and does not see a cycle the dependency
would close; archfit check does both.

Examples:
  archfit can pkg/tc pkg/util
  archfit can internal/app/run.go github.com/spf13/cobra`
}

func (c *CanCmd) Run(deps *appDeps) error {
	ctx := deps.runContext()
	co, err := openCheckout(ctx, deps, dirOr(c.Dir))
	if err != nil {
		return noVerdict(err)
	}
	// The tree is read before the answer: an edit during the run makes the
	// stored answer stale, never a newer tree's.
	tree, err := co.workingTree(ctx)
	if err != nil {
		return noVerdict(err)
	}
	stamp, err := co.stamp(ctx)
	if err != nil {
		return noVerdict(err)
	}
	// A missing argument is canquery.ErrUsage; it exits 2 as every error does.
	a, err := co.canQuery().Can(ctx, canquery.Question{From: c.From, To: c.To, Stamp: stamp})
	if err != nil {
		return noVerdict(err)
	}
	if err := co.saveAnswer(tree, a); err != nil {
		return noVerdict(err)
	}
	if c.Format == formatJSON {
		err = record.AnswerJSON(deps.Stdout, a)
	} else {
		err = record.Answer(deps.Stdout, a, record.Options{Verbose: c.Verbose})
	}
	if err != nil {
		return noVerdict(err)
	}
	return exitWith(replyExitCode(a.Answer))
}

// replyExitCode maps a `can` answer to the exit code.
func replyExitCode(r result.Reply) int {
	switch r {
	case result.ReplyYes:
		return 0
	case result.ReplyNo:
		return exitNo
	default:
		return exitUnknown
	}
}
