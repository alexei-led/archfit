package main

import (
	"errors"
	"fmt"
	"slices"

	"github.com/alexei-led/archfit/v3/internal/application/changecheck"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/output/record"
)

// Exit codes of the CLI: 0 pass, yes or
// done; 1 fail, waiting or no; 2 no verdict: unknown, wrong usage or a tool
// error.
const (
	exitNo      = 1
	exitUnknown = 2
)

// noVerdict is an error of a version 3 command: wrong usage or a tool error.
// It exits 2 with the message on stderr.
func noVerdict(err error) error {
	return &exitError{code: exitUnknown, msg: "archfit: " + err.Error()}
}

// statusExitCode maps the status of a change to the exit code.
func statusExitCode(s result.Status) int {
	switch s {
	case result.StatusPass:
		return 0
	case result.StatusFail, result.StatusWaiting:
		return exitNo
	default:
		return exitUnknown
	}
}

// exitWith returns the error that ends a command with code; nil for 0.
func exitWith(code int) error {
	if code == 0 {
		return nil
	}
	return &exitError{code: code}
}

// changeFormat is the one output format of a version 3 check.
func (c *CheckCmd) changeFormat() (string, error) {
	switch {
	case len(c.Format) == 0:
		return formatText, nil
	case len(c.Format) > 1:
		return "", errors.New("--format takes one value: text, json or sarif")
	case !slices.Contains([]string{formatText, formatJSON, formatSarif}, c.Format[0]):
		return "", fmt.Errorf("unknown format %s: use text, json or sarif", c.Format[0])
	}
	return c.Format[0], nil
}

// runChange is `archfit check` on a version 3 policy: the status of the
// change (internal/application/changecheck), as a record, JSON or SARIF on
// stdout. The notes of the run go to stderr.
func (c *CheckCmd) runChange(deps *appDeps) error {
	format, err := c.changeFormat()
	if err != nil {
		return noVerdict(err)
	}
	base := c.Base
	if base == "auto" {
		base = ""
	}
	ctx := deps.runContext()
	co, err := openCheckout(ctx, deps, dirOr(c.Dir))
	if err != nil {
		return noVerdict(err)
	}
	policy := c.Policy
	if policy != "" {
		policy = co.policyArg(policy)
	}
	out, err := co.changeCheck().Check(ctx, changecheck.Options{Base: base, Policy: policy})
	if err != nil {
		return noVerdict(err)
	}
	for _, note := range out.Notes {
		_, _ = fmt.Fprintln(deps.stderr(), "archfit: "+note)
	}
	switch format {
	case formatJSON:
		err = record.ResultJSON(deps.Stdout, out.Result)
	case formatSarif:
		err = record.ResultSARIF(deps.Stdout, out.Result)
	default:
		err = record.Result(deps.Stdout, out.Result, record.Options{Verbose: c.Verbose})
	}
	if err != nil {
		return noVerdict(err)
	}
	return exitWith(statusExitCode(out.Result.Status))
}

// dirOr returns dir, or the current directory when it is empty.
func dirOr(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}
