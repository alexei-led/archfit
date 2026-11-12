package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/alexei-led/archfit/v3/internal/application/changecheck"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/output/record"
)

// Exit codes of the version 3 commands (check, can, rules): 0 pass, yes or
// done; 1 fail, waiting or no; 2 no verdict: unknown, wrong usage or a tool
// error. Task 31 applies them to the whole binary.
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

// version2 reports whether the check runs the Archfit 3 path: the policy file
// is absent or is version 2 (D1). With -c the file is the one it names; else
// .archfit.yaml at the root of the repository of -C, or of the current
// directory. Outside a git repository the file is .archfit.yaml there.
func (c *CheckCmd) version2(deps *appDeps) (bool, error) {
	file := c.Config
	if file == "" {
		co, err := openCheckout(deps.runContext(), deps, dirOr(c.Dir))
		switch {
		case err == nil:
			file = co.policyPath("")
		case c.Dir != "":
			return false, noVerdict(err)
		default:
			file = defaultConfigPath
		}
	}
	data, err := os.ReadFile(file) //nolint:gosec // the policy file the user names
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case err != nil:
		return false, noVerdict(err)
	}
	return isVersion2(data), nil
}

// version2Flags are the flags of the Archfit 3 path that a version 3 check
// refuses, by name.
func (c *CheckCmd) version2Flags() []string {
	var set []string
	for _, f := range []struct {
		name string
		on   bool
	}{
		{"--root", c.Root != ""}, {"--target", c.Target != ""},
		{"--no-advisories", c.NoAdvisories}, {"--min-severity", c.MinSeverity != ""},
		{"--refresh", c.Refresh}, {"--require-tools", c.RequireTools}, {"--lang", len(c.Lang) > 0},
		{"--json", c.JSON}, {"--markdown", c.Markdown}, {"--sarif", c.Sarif},
		{"--progress", c.Progress != ""}, {"-q", c.Quiet},
	} {
		if f.on {
			set = append(set, f.name)
		}
	}
	return set
}

// changeFormat is the one output format of a version 3 check.
func (c *CheckCmd) changeFormat() (string, error) {
	switch {
	case len(c.Format) == 0:
		return formatText, nil
	case len(c.Format) > 1:
		return "", errors.New("--format takes one value: text, json or sarif")
	case !slices.Contains([]string{formatText, formatJSON, formatSarif}, c.Format[0]):
		return "", fmt.Errorf("--format %s needs a version 2 policy: use text, json or sarif", c.Format[0])
	}
	return c.Format[0], nil
}

// runChange is `archfit check` on a version 3 policy: the status of the
// change (internal/application/changecheck), as a record, JSON or SARIF on
// stdout. The notes of the run go to stderr.
func (c *CheckCmd) runChange(deps *appDeps) error {
	if c.Config != "" {
		return noVerdict(fmt.Errorf("-c %s: a version 3 check reads %s at the repository root; --policy previews another file",
			c.Config, defaultConfigPath))
	}
	if set := c.version2Flags(); len(set) > 0 {
		return noVerdict(fmt.Errorf("%v: these flags need a version 2 policy", set))
	}
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
	out, err := co.changeCheck().Check(ctx, changecheck.Options{Base: base, Policy: c.Policy})
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
