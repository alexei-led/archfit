package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition/checkfacts"
	"github.com/alexei-led/archfit/v3/internal/initcfg/policy3"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// agentLines are the five lines for agents that init writes into AGENTS.md.
// The session-start hook prints them too.
const agentLines = policy3.Lines

// RepoInitCmd is `archfit init` (internal/initcfg/policy3): set up a
// repository. It writes what is missing and never changes a version 3
// policy: a fresh repository gets a version 3 policy and the instruction
// block for agents; an Archfit 3 repository is converted, which replaces its
// policy and removes the other Archfit 3 files; a repository with a version 3
// policy gets only a missing block. It writes no workflow file
// (D11: the workflow waits for Action 4).
type RepoInitCmd struct {
	Dir string `short:"C" name:"directory" help:"Run in DIR instead of the current directory." placeholder:"DIR"`
}

func (*RepoInitCmd) Help() string {
	return `Set up a repository for archfit. It writes what is missing and never
changes a version 3 policy.

  No .archfit.yaml      writes a version 3 policy from the code: modules,
                        public, depends_on as the dependencies that exist,
                        tests, generated, owners from the origin remote,
                        and an exception for each error that exists today,
                        so the first check passes
  Archfit 3 policy      converts .archfit.yaml version 2 and its files
                        (.archfit-baseline.json, .archfit-labels.yaml,
                        .archfit-app.yaml) into one version 3 policy,
                        removes the old files, and says what it could not
                        convert
  Version 3 policy      leaves it as it is

Each case writes the five lines for agents between the archfit markers in
AGENTS.md; text outside the markers stays as it is.

Then read .archfit.yaml and open a pull request with it.

Exit codes:
  0  done, also when nothing was missing
  2  wrong usage or a tool error`
}

// initNext is the last line of init: the owner's step.
const initNext = "next: read " + policy3.PolicyFile + ", then open a pull request with it"

func (c *RepoInitCmd) Run(deps *appDeps) error {
	ctx := deps.runContext()
	co, err := openCheckout(ctx, deps, dirOr(c.Dir))
	if err != nil {
		return noVerdict(err)
	}
	data, found, err := co.policyAt("")
	if err != nil {
		return noVerdict(err)
	}
	var b strings.Builder
	switch {
	case !found:
		err = co.initFresh(ctx, &b)
	case isVersion3(data):
		err = co.initBlock(&b)
	default:
		err = co.initConvert(ctx, &b)
	}
	if err != nil {
		return noVerdict(err)
	}
	if _, err := io.WriteString(deps.Stdout, b.String()); err != nil {
		return noVerdict(err)
	}
	return nil
}

// initFresh writes the first policy of a repository.
func (co checkout) initFresh(ctx context.Context, b *strings.Builder) error {
	res, err := policy3.Init(ctx, policy3.Deps{Runner: co.runner, Facts: co.facts(policy3.FactsConfig())}, co.root)
	if err != nil {
		return err
	}
	writePolicySummary(b, "wrote", res.File)
	b.WriteString("wrote " + policy3.AgentsFile + ": the instruction block for agents\n")
	writeUnread(b, res.Unknown)
	b.WriteString(initNext + "\n")
	return nil
}

// initConvert converts an Archfit 3 repository.
func (co checkout) initConvert(ctx context.Context, b *strings.Builder) error {
	conv, err := policy3.Convert(ctx, policy3.ConvertDeps{
		Runner: co.runner,
		Facts:  func(cfg checkfacts.Config) policy3.HeadFacts { return co.facts(cfg) },
	}, co.root)
	if err != nil {
		return err
	}
	writePolicySummary(b, "converted", conv.File)
	b.WriteString(conv.Text())
	if len(conv.Removed) > 0 {
		b.WriteString("removed: " + strings.Join(conv.Removed, ", ") + "\n")
	}
	b.WriteString("wrote " + policy3.AgentsFile + ": the instruction block for agents\n")
	writeUnread(b, conv.Unknown)
	b.WriteString(initNext + "\n")
	return nil
}

// initBlock is init in a repository with a version 3 policy: only the
// instruction block can be missing.
func (co checkout) initBlock(b *strings.Builder) error {
	wrote, err := policy3.WriteBlock(co.root)
	if err != nil {
		return err
	}
	b.WriteString("left " + policy3.PolicyFile + " as it is: archfit init never changes a version 3 policy\n")
	if wrote {
		b.WriteString("wrote " + policy3.AgentsFile + ": the instruction block for agents\n")
	} else {
		b.WriteString("left " + policy3.AgentsFile + " as it is: it has the instruction block\n")
	}
	return nil
}

// writePolicySummary writes what the policy holds, then its exceptions: the
// errors that exist today, which an owner reads before the pull request.
func writePolicySummary(b *strings.Builder, verb string, f policyfile.File) {
	deps := 0
	for _, m := range f.Modules {
		deps += len(m.DependsOn)
	}
	fmt.Fprintf(b, "%s %s: %s, %s, %s, %s\n", verb, policy3.PolicyFile,
		count(len(f.Modules), "module"), count(deps, "allowed dependency"),
		count(len(f.Rules), "rule"), count(len(f.Exceptions), "exception"))
	for _, x := range f.Exceptions {
		place := x.From
		if x.To != "" {
			place += " -> " + x.To
		}
		b.WriteString("  exception: " + x.Rule + " " + place)
		if x.Expires != "" {
			b.WriteString(" until " + x.Expires)
		}
		b.WriteString("\n")
	}
}

// writeUnread lists the places of the code archfit could not read. The
// policy is written without them.
func writeUnread(b *strings.Builder, items []evidence.UnknownItem) {
	for _, u := range items {
		b.WriteString("not read: ")
		if u.File != "" {
			b.WriteString(location(u.File, u.Line) + ": ")
		}
		b.WriteString(u.Reason + "\n")
		if u.Fix != "" {
			b.WriteString("  fix: " + u.Fix + "\n")
		}
	}
}

// count is "n noun" with the noun in the plural for any n but 1.
func count(n int, noun string) string {
	if n != 1 {
		if strings.HasSuffix(noun, "y") {
			noun = strings.TrimSuffix(noun, "y") + "ies"
		} else {
			noun += "s"
		}
	}
	return strconv.Itoa(n) + " " + noun
}
