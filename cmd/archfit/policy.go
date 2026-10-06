// Package main — `archfit policy where` and `archfit policy can-import`.
//
// The pre-edit queries read the config, the waivers, and the baseline. They run
// no extractor and never open the fact cache. can-import judges the queried
// import with the application use case, which runs the relationship analysis
// and rule pass check runs; this file owns flags, the wire shape, and the exit
// code.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/policy"
)

// policyAnswerSchemaVersion versions the `archfit policy` answer document.
const policyAnswerSchemaVersion = "archfit.policy-answer.v1"

// Answers of `policy can-import`, as the evaluator names them.
const (
	answerDenied     = "denied"
	answerNotDecided = "not_decided"
	answerAllowed    = "allowed"
)

// PolicyCmd groups the pre-edit queries.
type PolicyCmd struct {
	Where     WhereCmd     `cmd:"" help:"Which module owns a path: its layer, role, owner, public surface, allowlists, scope, and the rules that select it."`
	CanImport CanImportCmd `cmd:"" name:"can-import" help:"Whether a file may import a target, judged by the rule pass check runs. Exits 1 when a target is denied."`
}

// policyFlags are the flags both queries share.
type policyFlags struct {
	Config string `short:"c" help:"Path to config file." default:".archfit.yaml"`
	Root   string `help:"Repository root the paths are relative to (default: directory of --config)." type:"path"`
}

// WhereCmd answers which module owns each path.
type WhereCmd struct {
	policyFlags
	Paths []string `arg:"" name:"path" help:"Repo-relative file or directory paths."`
}

// CanImportCmd answers whether one file may import each target.
type CanImportCmd struct {
	policyFlags
	From    string   `arg:"" name:"from" help:"The importing source file, repo-relative."`
	Targets []string `arg:"" name:"target" help:"The imported package dir (Go), file (TypeScript), or dotted module or file (Python)."`
}

func (*WhereCmd) Help() string {
	return `Answer which module owns each path, before an edit.

It reads the config only: no analyzer runs and the fact cache is not read.
"excluded" is true when check never reads the path (an exclude: glob or a
language switched off). "rules" lists every rule whose selector names the path
or its module; the list is for reading, not a verdict.

Exit codes:
  0  answered
  3  usage or config error

Example:
  archfit policy where internal/assessment/rules/rules.go`
}

func (*CanImportCmd) Help() string {
	return `Answer whether a file may import each target, before an edit.

The import goes through the relationship analysis and rule pass check runs,
under the same baseline and waivers, so a denied import is the gate finding
check would report, with the same finding ID. No analyzer runs and the fact
cache is not read.

Answers:
  denied         a fail-gated rule fires on the import (the finding ID and
                 the repair text are in "denials")
  not_decided    no rule denies it, but a fail-gated rule that needs the whole
                 graph (a cycle rule, the seam gate in mode: fail) can still
                 block it, or the language needs its analyzer (Rust)
  allowed        an allowlist entry or the layer order permits it
  unconstrained  no rule decides it; this is not permission

Exit codes:
  0  no target denied, every target decided
  1  a target is denied
  2  no target denied, a target is not decided
  3  usage or config error

Examples:
  archfit policy can-import internal/relationship/scoring/scorer_book.go internal/toolrun
  archfit policy can-import web/src/app.ts web/src/db/client.ts
  archfit policy can-import src/myapp/handlers.py myapp.domain`
}

// policyQuerier is what the queries read beyond the config: the analysis
// root's path spelling and the declared scope.
type policyQuerier interface {
	Rel(path string) string
	OutOfScope(file string) bool
}

func (c *WhereCmd) Run(deps *appDeps) error {
	cfg, err := loadAnalysisConfig(context.Background(), c.Config)
	if err != nil {
		return configLoadError(err)
	}
	q := newPolicyQuery(c.Config, c.Root, cfg)
	doc := whereDoc{SchemaVersion: policyAnswerSchemaVersion, Command: "where", Paths: make([]whereAnswer, 0, len(c.Paths))}
	for _, path := range c.Paths {
		doc.Paths = append(doc.Paths, where(cfg.PolicySnapshot(), q, path))
	}
	return writePolicyJSON(deps.Stdout, doc)
}

func (c *CanImportCmd) Run(deps *appDeps) error {
	ctx := context.Background()
	cfg, err := loadAnalysisConfig(ctx, c.Config)
	if err != nil {
		return configLoadError(err)
	}
	answers, err := newPolicyQueryService(c.Config, c.Root, cfg).CanImport(ctx, application.CanImportRequest{
		Policy: cfg.PolicySnapshot(), BundleDir: bundleDirOf(c.Config), From: c.From, Targets: c.Targets, Now: time.Now(),
	})
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	doc := canImportDoc{SchemaVersion: policyAnswerSchemaVersion, Command: "can-import", Answers: make([]canImportAnswer, 0, len(answers))}
	denied, undecided := false, false
	for _, a := range answers {
		doc.Answers = append(doc.Answers, canImportAnswerOf(a))
		denied = denied || a.Answer == answerDenied
		undecided = undecided || a.Answer == answerNotDecided
	}
	if err := writePolicyJSON(deps.Stdout, doc); err != nil {
		return err
	}
	switch {
	case denied:
		return &exitError{code: 1}
	case undecided:
		return &exitError{code: 2}
	default:
		return nil
	}
}

// whereDoc is the `policy where` answer (archfit.policy-answer.v1).
type whereDoc struct {
	SchemaVersion string        `json:"schema_version"`
	Command       string        `json:"command"`
	Paths         []whereAnswer `json:"paths"`
}

// whereAnswer is what the config says about one path.
type whereAnswer struct {
	Path      string      `json:"path"`
	Language  string      `json:"language,omitempty"`
	Selector  string      `json:"selector,omitempty"`
	Excluded  bool        `json:"excluded"`
	Module    string      `json:"module,omitempty"`
	Layer     string      `json:"layer,omitempty"`
	Role      string      `json:"role,omitempty"`
	Owner     string      `json:"owner,omitempty"`
	Public    []string    `json:"public,omitempty"`
	DependsOn []string    `json:"depends_on,omitempty"`
	VisibleTo []string    `json:"visible_to,omitempty"`
	Rules     []whereRule `json:"rules"`
}

// whereRule is one rule whose selector names the path or its module.
type whereRule struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Gate  string `json:"gate,omitempty"`
	Match string `json:"match"`
}

// where reads one path against the declared policy. Ownership is the
// most-specific declared module, as check resolves it for a source file.
func where(p policy.PolicySnapshot, q policyQuerier, raw string) whereAnswer {
	path := q.Rel(raw)
	mm := p.Topology.ModuleMap
	out := whereAnswer{Path: path, Excluded: q.OutOfScope(path), Rules: []whereRule{}}
	if language, selector, ok := mm.RuleSelectorForFile(path); ok {
		out.Language, out.Selector = language, selector
	}
	module, owned := mm.ModuleForFile(path)
	if owned {
		def := p.Topology.Modules[module]
		out.Module, out.Layer, out.Role, out.Owner = module, def.Layer, string(def.Role), def.Owner
		out.Public, out.DependsOn, out.VisibleTo = def.Public, def.DependsOn, def.VisibleTo
	}
	for _, def := range p.Gates.Rules.Rules {
		if match := ruleMatch(mm, p.Topology.Layers, def, path, out.Selector, module, owned, out.Layer); match != "" {
			out.Rules = append(out.Rules, whereRule{ID: def.ID, Type: def.Type, Gate: def.Gate, Match: match})
		}
	}
	return out
}

// ruleMatch names how a rule selects a path: through its from or to glob (on
// the path or its node selector), its from_module or to_module, or — for the
// module-wide rule types — the owning module. "" when it does not.
func ruleMatch(mm policy.ModuleMap, layers []string, def policy.RuleDef, path, selector, module string, owned bool, layer string) string {
	globMatches := func(glob string) bool {
		if glob == "" {
			return false
		}
		for _, candidate := range []string{path, selector} {
			if matched, _ := doublestar.Match(glob, candidate); candidate != "" && matched {
				return true
			}
		}
		return false
	}
	switch {
	case globMatches(def.From):
		return "from"
	case globMatches(def.To):
		return "to"
	case owned && def.FromModule != "" && mm.SelectsModule(def.FromModule, module):
		return "from_module"
	case owned && def.ToModule != "" && mm.SelectsModule(def.ToModule, module):
		return "to_module"
	}
	if !owned {
		return ""
	}
	switch def.Type {
	case "module_cycle", "module_dependencies", "new_cross_module_dependency":
		return "module"
	case "forbidden_layer_direction":
		if slices.Contains(layers, layer) {
			return "layer"
		}
	}
	return ""
}

// canImportDoc is the `policy can-import` answer (archfit.policy-answer.v1).
type canImportDoc struct {
	SchemaVersion string            `json:"schema_version"`
	Command       string            `json:"command"`
	Answers       []canImportAnswer `json:"answers"`
}

// canImportAnswer is the judgment for one target, with the next step.
type canImportAnswer struct {
	From       string          `json:"from"`
	Target     string          `json:"target"`
	Language   string          `json:"language,omitempty"`
	Answer     string          `json:"answer"`
	Next       string          `json:"next"`
	Reasons    []string        `json:"reasons"`
	Denials    []policyFinding `json:"denials"`
	Accepted   []policyFinding `json:"accepted"`
	Advisories []policyFinding `json:"advisories"`
}

// policyFinding is one finding on the queried import, with its repair text.
type policyFinding struct {
	FindingID   string   `json:"finding_id"`
	RuleID      string   `json:"rule_id"`
	Status      string   `json:"status"`
	Why         string   `json:"why"`
	RepairKind  string   `json:"repair_kind,omitempty"`
	Goal        string   `json:"goal,omitempty"`
	Constraints []string `json:"constraints"`
}

// Next steps per answer.
const (
	nextDenied     = "Do not add this import: follow the goal and constraints of each denial. If the import is intended, ask the architecture owner; do not edit the policy, baseline, or waivers yourself."
	nextNotDecided = "The rule pass does not deny the import, but it can still fail a rule that needs the whole graph: add it, then run archfit check --format agent."
	nextAllowed    = "The policy permits the import: add it, then run archfit check --format agent."
	nextFree       = "No rule decides this import, which is not permission: add it only if the design calls for it, then run archfit check --format agent."
)

func canImportAnswerOf(a application.CanImportAnswer) canImportAnswer {
	out := canImportAnswer{
		From: a.From, Target: a.Target, Language: a.Language, Answer: a.Answer,
		Reasons: nonNil(a.Reasons), Denials: policyFindings(a.Denials),
		Accepted: policyFindings(a.Accepted), Advisories: policyFindings(a.Advisories),
	}
	switch a.Answer {
	case answerDenied:
		out.Next = nextDenied
	case answerNotDecided:
		out.Next = nextNotDecided
	case answerAllowed:
		out.Next = nextAllowed
	default:
		out.Next = nextFree
	}
	return out
}

func policyFindings[T interface {
	~struct {
		FindingID   string
		RuleID      string
		Status      string
		Why         string
		RepairKind  string
		Goal        string
		Constraints []string
	}
}](in []T) []policyFinding {
	out := make([]policyFinding, 0, len(in))
	for _, f := range in {
		v := (struct {
			FindingID   string
			RuleID      string
			Status      string
			Why         string
			RepairKind  string
			Goal        string
			Constraints []string
		})(f)
		out = append(out, policyFinding{
			FindingID: v.FindingID, RuleID: v.RuleID, Status: v.Status, Why: v.Why,
			RepairKind: v.RepairKind, Goal: v.Goal, Constraints: nonNil(v.Constraints),
		})
	}
	return out
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// writePolicyJSON writes one compact JSON document. HTML escaping is off: an
// agent reads "a -> b", not a browser.
func writePolicyJSON(w io.Writer, doc any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: writing policy answer: %v", err)}
	}
	return nil
}
