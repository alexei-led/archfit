// Package main — `archfit config lint`.
//
// Lint reports configuration defects that loading accepts but that silently
// weaken the policy: rule selectors that match nothing, guard rules, module
// values classification cannot read, public entries outside their module or
// naming no source, and ownership ties. It reads the source inventory, runs no
// analyzer, and writes nothing. The judgment is the application use case; this
// file owns flags, the two output shapes, and the exit code.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/alexei-led/archfit/internal/application"
)

// configLintSchemaVersion versions the `config lint --json` document.
const configLintSchemaVersion = "archfit.config-lint.v1"

// LintCmd reports configuration defects judged against the source tree.
type LintCmd struct {
	Config string `short:"c" help:"Config file path." default:".archfit.yaml"`
	Root   string `short:"r" help:"Repository root to lint against (default: the analysis root of --config, as check resolves it)." type:"path"`
	JSON   bool   `name:"json" help:"Emit the diagnostics as a JSON document (archfit.config-lint.v1)."`
}

func (*LintCmd) Help() string {
	return `Lint the config against the source tree, before a gate reads it.

Exit codes:
  0  no error diagnostics (warnings and info may be printed)
  1  at least one error diagnostic
  3  the config cannot be read, parsed, or validated

Errors: dead_selector, unknown_volatility, unknown_subdomain, undeclared_layer,
public_outside_module, public_matches_nothing, ambiguous_ownership.
Warnings: guard_matches_source, and dead_selector on a gate: off rule.
Info: guard_rule.

A dead selector is decided by the same predicate check uses: a gated rule with
one is listed in decision.unevaluated_required_rules (gate: fail) or as a config
warning (gate: warn). Mark an intentional match-nothing rule with guard: true.

Common runs:
  archfit config lint
  archfit config lint --json -c .archfit.yaml`
}

func (c *LintCmd) Run(deps *appDeps) error {
	ctx := context.Background()
	cfg, err := loadAnalysisConfig(ctx, c.Config)
	if err != nil {
		return configLoadError(err)
	}
	diagnostics, err := newConfigLintService(c.Config, cfg, deps).Execute(ctx,
		application.ConfigLintRequest{Policy: cfg.PolicySnapshot(), Root: c.Root})
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	doc := configLintDoc{SchemaVersion: configLintSchemaVersion, Diagnostics: make([]configLintDiagnostic, 0, len(diagnostics))}
	failed := false
	for _, d := range diagnostics {
		doc.Diagnostics = append(doc.Diagnostics, configLintDiagnostic{Code: d.Code, Severity: d.Severity, Path: d.Path, Message: d.Message})
		failed = failed || d.IsError()
	}
	if c.JSON {
		err = writeConfigLintJSON(deps.Stdout, doc)
	} else {
		err = writeConfigLintText(deps.Stdout, doc)
	}
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: writing lint output: %v", err)}
	}
	if failed {
		return &exitError{code: 1}
	}
	return nil
}

// configLintDoc is the `--json` contract (archfit.config-lint.v1). The wire
// shape is owned here, in the composition root, like config compare's.
type configLintDoc struct {
	SchemaVersion string                 `json:"schema_version"`
	Diagnostics   []configLintDiagnostic `json:"diagnostics"`
}

// configLintDiagnostic is one defect: a stable code, its severity, the config
// path it points at, and a one-line message.
type configLintDiagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

func writeConfigLintJSON(w io.Writer, doc configLintDoc) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// writeConfigLintText prints one line per diagnostic: severity, code, config
// path, message. A clean config prints nothing.
func writeConfigLintText(w io.Writer, doc configLintDoc) error {
	for _, d := range doc.Diagnostics {
		if _, err := fmt.Fprintf(w, "%s %s %s: %s\n", d.Severity, d.Code, d.Path, d.Message); err != nil {
			return err
		}
	}
	return nil
}
