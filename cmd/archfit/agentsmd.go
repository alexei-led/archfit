// Package main — `archfit agents-md`.
//
// agents-md renders one managed block of agent instructions: the worker loop,
// at most agentsMDMaxLines lines. It writes the block between two markers in
// AGENTS.md (or another file) and
// leaves every byte outside the markers as it was. The output is sorted and
// has no timestamp, so a second write changes nothing and --check can detect
// drift in CI.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Markers of the managed block.
const (
	agentsMDStart = "<!-- archfit:start -->"
	agentsMDEnd   = "<!-- archfit:end -->"
)

// AgentsMDCmd renders the managed agent-instruction block.
type AgentsMDCmd struct {
	Config string `short:"c" help:"Path to config file." default:".archfit.yaml"`
	File   string `help:"Instruction file that holds the block." default:"AGENTS.md" type:"path"`
	Write  bool   `help:"Write the block into --file: replace the text between the markers, or append the block when the file has none."`
	Check  bool   `help:"Exit 1 when the block in --file differs from the rendered one (CI drift check)."`
}

func (*AgentsMDCmd) Help() string {
	return `Render the archfit block of agent instructions: the worker loop in at
most 15 lines (ask policy can-import, run check --format agent, look up module
facts with policy where, run doctor, never edit policy to pass, escalate on
ask_owner or unknown).

The block sits between <!-- archfit:start --> and <!-- archfit:end -->. Text
outside the markers stays byte-identical. The output is sorted and has no
timestamp, so a second --write changes nothing.

Without --write or --check it prints the block.

Exit codes:
  0  printed, written, or current
  1  --check: the block in --file is missing or out of date
  3  usage, config, or file error

Examples:
  archfit agents-md
  archfit agents-md --write
  archfit agents-md --write --file CLAUDE.md
  archfit agents-md --check`
}

func (c *AgentsMDCmd) Run(deps *appDeps) error {
	if c.Write && c.Check {
		return &exitError{code: 3, msg: "error: --write and --check are mutually exclusive"}
	}
	// The config is loaded to validate it: a block must not point an agent at
	// a config that does not load.
	if _, err := loadAnalysisConfig(context.Background(), c.Config); err != nil {
		return configLoadError(err)
	}
	block := renderAgentsBlock(configArg(c.Config, c.File))
	if !c.Write && !c.Check {
		_, err := fmt.Fprint(deps.Stdout, block)
		return err
	}
	current, err := os.ReadFile(c.File)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return &exitError{code: 3, msg: fmt.Sprintf("error: read %s: %v", c.File, err)}
	}
	updated, err := spliceAgentsBlock(current, block)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %s: %v", c.File, err)}
	}
	if c.Check {
		if !bytes.Equal(current, updated) {
			return &exitError{code: 1, msg: fmt.Sprintf("%s: the archfit block is missing or out of date; run: archfit agents-md --write --file %s", c.File, c.File)}
		}
		return nil
	}
	if bytes.Equal(current, updated) {
		return nil
	}
	if err := os.WriteFile(c.File, updated, 0o644); err != nil { //nolint:gosec // an instruction file is world-readable by design
		return &exitError{code: 3, msg: fmt.Sprintf("error: write %s: %v", c.File, err)}
	}
	_, err = fmt.Fprintf(deps.Stdout, "wrote the archfit block to %s\n", c.File)
	return err
}

// spliceAgentsBlock replaces the text between the markers, markers included,
// with block, or appends block when the file has none. Every byte outside the
// markers stays as it was.
func spliceAgentsBlock(current []byte, block string) ([]byte, error) {
	start := bytes.Index(current, []byte(agentsMDStart))
	end := bytes.Index(current, []byte(agentsMDEnd))
	switch {
	case start < 0 && end < 0:
		// Append only: every existing byte stays, a blank line separates.
		out := slices.Clone(current)
		switch {
		case len(out) == 0:
		case bytes.HasSuffix(out, []byte("\n")):
			out = append(out, '\n')
		default:
			out = append(out, "\n\n"...)
		}
		return append(out, block...), nil
	case start < 0 || end < start:
		return nil, errors.New("the archfit markers are unbalanced")
	}
	end += len(agentsMDEnd)
	if end < len(current) && current[end] == '\n' {
		end++
	}
	out := append(slices.Clone(current[:start]), block...)
	return append(out, current[end:]...), nil
}

// configArg spells the config the way a command run from the instruction
// file's directory names it: relative to that directory, slash-separated.
func configArg(configPath, file string) string {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		return filepath.ToSlash(configPath)
	}
	if rel, err := filepath.Rel(filepath.Dir(file), abs); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

// agentsMDMaxLines is the size the block must stay under, markers included:
// it is loaded into every agent session, and the module and rule facts are one
// `archfit policy where` away.
const agentsMDMaxLines = 15

// renderAgentsBlock renders the managed block, markers included. config names
// the config, relative to the instruction file; the commands carry -c unless it
// is the default .archfit.yaml. The block holds the worker loop only: the
// module table and the rule list are in `archfit policy where`.
func renderAgentsBlock(config string) string {
	flag := ""
	if config != defaultConfigPath {
		flag = " -c " + config
	}
	var b strings.Builder
	b.WriteString(agentsMDStart + "\n")
	fmt.Fprintf(&b, "<!-- Generated by `archfit agents-md` from %s. Edit the config, not this block. -->\n", config)
	b.WriteString("## Architecture guardrails (archfit)\n\n")
	fmt.Fprintf(&b, "- Before a cross-module import: `archfit policy can-import%s <file> <target>`. `denied` names the rule and what to do instead; `unconstrained` is not permission.\n", flag)
	fmt.Fprintf(&b, "- Before you finish: `archfit check%s --format agent`. Follow `next_action`; `change.status: pass` is a correct finish, even with exit code 2.\n", flag)
	fmt.Fprintf(&b, "- Module owner, layer, allowlists and rules for a path: `archfit policy where%s <path>`.\n", flag)
	b.WriteString("- At session start, or when unsure the hooks run: `archfit doctor`.\n")
	fmt.Fprintf(&b, "- Never edit %s, `.archfit-baseline.json` or `.archfit-labels.yaml`, and never add a waiver, to make a check pass.\n", code(config))
	b.WriteString("- `ask_owner` or `change.status: unknown`: stop, report the rule and what you need, and ask the architecture owner.\n")
	b.WriteString(agentsMDEnd + "\n")
	return b.String()
}

// code renders s as inline code, safe inside a Markdown table cell.
func code(s string) string { return "`" + strings.ReplaceAll(s, "|", "\\|") + "`" }
