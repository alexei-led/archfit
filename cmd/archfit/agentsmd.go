// Package main — `archfit agents-md`.
//
// agents-md renders one managed block of agent instructions from the config:
// the module table, the rules that block as sentences, and the agent loop. It
// writes the block between two markers in AGENTS.md (or another file) and
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

	"github.com/alexei-led/archfit/internal/policy"
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
	return `Render the archfit block of agent instructions from the config: the
module table, the rules that block as sentences, and the agent loop (ask
policy can-import, run check --format agent, never edit policy to pass).

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
	cfg, err := loadAnalysisConfig(context.Background(), c.Config)
	if err != nil {
		return configLoadError(err)
	}
	block := renderAgentsBlock(cfg.PolicySnapshot(), configArg(c.Config, c.File))
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

// renderAgentsBlock renders the managed block, markers included, from the
// policy. config names the config, relative to the instruction file; the
// commands carry -c unless it is the default .archfit.yaml.
func renderAgentsBlock(p policy.PolicySnapshot, config string) string {
	flag := ""
	if config != defaultConfigPath {
		flag = " -c " + config
	}
	var b strings.Builder
	b.WriteString(agentsMDStart + "\n")
	fmt.Fprintf(&b, "<!-- Generated by `archfit agents-md` from %s. Edit the config, not this block. -->\n\n", config)
	b.WriteString("## Architecture guardrails (archfit)\n\n")
	fmt.Fprintf(&b, "- Before you add an import across modules, ask: `archfit policy can-import%s <file> <target>`.\n", flag)
	b.WriteString("  A `denied` answer names the rule and the repair. `unconstrained` is not permission.\n")
	fmt.Fprintf(&b, "- Before you finish, run `archfit check%s --format agent` and follow `next_action`.\n", flag)
	b.WriteString("  `none` is a correct finish, even with exit code 2.\n")
	fmt.Fprintf(&b, "- Never edit %s, the baseline, waivers, or labels to make a check pass.\n", code(config))
	b.WriteString("  Ask the architecture owner instead.\n")
	writeModuleTable(&b, p.Topology)
	writeBlockingRules(&b, p)
	b.WriteString(agentsMDEnd + "\n")
	return b.String()
}

func writeModuleTable(b *strings.Builder, t policy.TopologyView) {
	if len(t.Modules) == 0 {
		return
	}
	b.WriteString("\n### Modules\n\n")
	b.WriteString("| Module | Paths | Layer | Owner | Public | Depends on | Visible to |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	names := make([]string, 0, len(t.Modules))
	for name := range t.Modules {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		def := t.Modules[name]
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			code(name), codeList(def.Paths), cell(def.Layer), cell(def.Owner), codeList(def.Public),
			allowlist(def.DependsOn), allowlist(def.VisibleTo))
	}
	if len(t.Layers) > 0 {
		layers := make([]string, 0, len(t.Layers))
		for _, layer := range t.Layers {
			layers = append(layers, code(layer))
		}
		fmt.Fprintf(b, "\nLayers, inner to outer: %s. A layer may import only itself and layers inner to it.\n", strings.Join(layers, ", "))
	}
}

// writeBlockingRules lists the rules that block as sentences, grouped by the
// code they constrain.
func writeBlockingRules(b *strings.Builder, p policy.PolicySnapshot) {
	groups := map[string][]string{}
	for _, def := range p.Gates.Rules.Rules {
		if !def.Blocks() {
			continue
		}
		sentence := "`" + def.ID + "`: " + ruleSentence(def)
		if def.Rationale != "" {
			sentence += " Why: " + def.Rationale
		}
		if def.Docs != "" {
			sentence += " See " + def.Docs + "."
		}
		groups[ruleSource(def)] = append(groups[ruleSource(def)], sentence)
	}
	if len(groups) == 0 {
		return
	}
	b.WriteString("\n### Rules that block\n")
	sources := make([]string, 0, len(groups))
	for source := range groups {
		sources = append(sources, source)
	}
	slices.Sort(sources)
	for _, source := range sources {
		fmt.Fprintf(b, "\n%s:\n\n", source)
		sentences := groups[source]
		slices.Sort(sentences)
		for _, s := range sentences {
			fmt.Fprintf(b, "- %s\n", s)
		}
	}
}

// ruleSource names the code a rule constrains: its source selector, or all
// code for a rule without one.
func ruleSource(def policy.RuleDef) string {
	switch {
	case def.From != "":
		return "Code in " + code(def.From)
	case def.FromModule != "":
		return "Modules " + code(def.FromModule)
	default:
		return "All code"
	}
}

// ruleSentence states what a rule forbids, in one sentence per type.
func ruleSentence(def policy.RuleDef) string {
	target := def.To
	if target == "" {
		target = def.ToModule
	}
	switch def.Type {
	case "forbidden_dependency":
		if target == "" {
			return "must not import the target the rule names."
		}
		return "must not import " + code(target) + "."
	case "public_api_only":
		return "may import another module only through that module's `public` paths."
	case "internal_api_access":
		if target == "" {
			return "must not import another module's `internal` paths."
		}
		return "must not import the internal paths of " + code(target) + "."
	case "forbidden_layer_direction":
		return "must not import a layer outer to its own."
	case "module_dependencies":
		return "must keep to the `depends_on` and `visible_to` lists in the module table."
	case "module_cycle":
		return "must not close a dependency cycle between declared modules."
	case "cycle":
		return "must not close an import cycle."
	case "new_cross_module_dependency":
		return "must not add a new dependency between two declared modules without an architecture-owner decision."
	case "public_api_max":
		if def.Max != nil {
			return fmt.Sprintf("must not export more than %d declarations per module.", *def.Max)
		}
		return "must keep each module's exported declarations under the rule's limit."
	case "public_api_change":
		return "must not change a module's public API without review."
	case "public_api_type_leak":
		return "must not expose an external type in a module's public API."
	case "forbidden_pattern":
		return fmt.Sprintf("must not contain code that matches the rule's %d ast-grep pattern(s).", len(def.Patterns))
	default:
		return "must satisfy rule type " + code(def.Type) + "."
	}
}

// code and cell keep config text inside its table cell or list item: a YAML
// block scalar's line breaks would end the row, and a bare | would split it.
func code(s string) string { return "`" + tableText(s) + "`" }

func cell(s string) string {
	if s == "" {
		return "—"
	}
	return tableText(s)
}

func tableText(s string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(s), " "), "|", "\\|")
}

func codeList(items []string) string {
	if len(items) == 0 {
		return "—"
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, code(item))
	}
	return strings.Join(out, ", ")
}

// allowlist renders an allowlist: absent is unconstrained, empty allows none.
func allowlist(items []string) string {
	switch {
	case items == nil:
		return "any"
	case len(items) == 0:
		return "none"
	default:
		return codeList(items)
	}
}
