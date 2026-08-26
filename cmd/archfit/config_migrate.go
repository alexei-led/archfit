package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alexei-led/archfit/internal/initcfg"
)

// migrationConflicts are the flags a schema migration cannot combine with. They
// all mean "analyse the tree and propose structural edits", which is the
// opposite of a mechanical one-file schema rewrite.
const (
	flagAIClassifyName = "--ai-classify"
	flagRefreshName    = "--refresh"
)

// runMigration previews or applies the config schema migration.
//
// The refusals happen before any read of the tree: an invalid flag combination
// is a usage error, and reporting it after a discovery pass would charge the
// user for work whose result is discarded.
func (c *UpdateCmd) runMigration(deps *appDeps) error {
	var conflicts []string
	if c.AIClassify {
		conflicts = append(conflicts, flagAIClassifyName)
	}
	if c.Refresh {
		conflicts = append(conflicts, flagRefreshName)
	}
	if len(conflicts) > 0 {
		return &exitError{code: 3, msg: "error: --migration-only cannot be combined with " + strings.Join(conflicts, ", ")}
	}
	if c.JSON && c.Apply {
		return &exitError{code: 3, msg: "error: --migration-only --json previews and --migration-only --apply writes; use one"}
	}

	src, err := os.ReadFile(c.Config) // #nosec G304 — path is the caller-supplied config file
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: reading %s: %v", c.Config, err)}
	}
	result := initcfg.MigrateToV2(src)

	if c.JSON {
		enc := json.NewEncoder(deps.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(migrationReport{MigrationResult: result, Config: c.Config}); err != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("error: encoding migration report: %v", err)}
		}
		return nil
	}
	if !c.Apply {
		writeMigrationPreview(deps, c.Config, result)
		return nil
	}
	if !result.Changed() {
		_, _ = fmt.Fprintf(deps.Stdout, "%s is already schema v%d — nothing to migrate\n", c.Config, result.ToVersion)
		return nil
	}
	if err := os.WriteFile(c.Config, result.Output, 0o600); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: writing %s: %v", c.Config, err)}
	}
	writeMigrationPreview(deps, c.Config, result)
	_, _ = fmt.Fprintf(deps.Stdout, "\napplied to %s\n", c.Config)
	return nil
}

// migrationReport is the JSON envelope. It names the file so a report captured
// from a sweep can be traced back to the config it describes.
type migrationReport struct {
	initcfg.MigrationResult
	Config string `json:"config"`
}

func writeMigrationPreview(deps *appDeps, path string, r initcfg.MigrationResult) {
	if !r.Changed() {
		_, _ = fmt.Fprintf(deps.Stdout, "%s is already schema v%d — nothing to migrate\n", path, r.ToVersion)
		return
	}
	_, _ = fmt.Fprintf(deps.Stdout, "%s: schema v%d → v%d\n\n", path, r.FromVersion, r.ToVersion)
	for _, ch := range r.Changes {
		_, _ = fmt.Fprintf(deps.Stdout, "  %-8s %s  (%s)\n", ch.Action, ch.Key, ch.Detail)
	}
	if len(r.PolicyChanges) > 0 {
		_, _ = fmt.Fprintln(deps.Stdout, "\nPOLICY CHANGE")
		for _, p := range r.PolicyChanges {
			_, _ = fmt.Fprintf(deps.Stdout, "  · %s\n", p)
		}
	}
}
