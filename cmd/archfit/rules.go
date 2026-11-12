package main

import (
	"github.com/alexei-led/archfit/v3/internal/application/rulesquery"
)

// RulesCmd is `archfit rules` (internal/application/rulesquery): the rules
// that apply, in plain words. It reads a version 3 policy.
type RulesCmd struct {
	Args  []string `arg:"" optional:"" name:"path-or-rule" help:"Paths (relative to the repository root), or one rule id."`
	Graph bool     `help:"The modules by layer, with their allowed dependencies."`
	Dir   string   `short:"C" name:"directory" help:"Run in DIR instead of the current directory." placeholder:"DIR"`
}

func (*RulesCmd) Help() string {
	return `The rules that apply, in plain words.

  archfit rules                  the rules for the files the change touches
  archfit rules pkg/tc pkg/util  each path's module: public part, volatility,
                                 layer, allowed dependencies, rules, exceptions
  archfit rules graph            one rule in full, with its exceptions
  archfit rules --graph          the modules by layer and their allowed
                                 dependencies

Notes of the policy are shown as "not checked".

Exit codes:
  0  done
  2  wrong usage or a tool error`
}

func (c *RulesCmd) Run(deps *appDeps) error {
	ctx := deps.runContext()
	co, err := openCheckout(ctx, deps, dirOr(c.Dir))
	if err != nil {
		return noVerdict(err)
	}
	// Mixed forms are rulesquery.ErrUsage; it exits 2 as every error does.
	listing, err := co.rulesQuery().Rules(ctx, rulesquery.Request{Args: c.Args, Graph: c.Graph, Date: co.now()})
	if err != nil {
		return noVerdict(err)
	}
	if err := listing.Write(deps.Stdout); err != nil {
		return noVerdict(err)
	}
	return nil
}
