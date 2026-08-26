package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alexei-led/archfit/internal/config"
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
	// An unversioned file is not "already current" — it does not load under any
	// schema. Saying nothing needs migrating would report success on the one
	// command advertised as the escape from an unloadable config.
	if result.Status == initcfg.MigrationUnversioned {
		return &exitError{code: 3, msg: fmt.Sprintf(
			"error: %s declares no root `version:` key, so there is no schema to migrate from — "+
				"add `version: %d` at column zero and re-run",
			c.Config, result.ToVersion)}
	}
	if !result.Changed() {
		_, _ = fmt.Fprintf(deps.Stdout, "%s is already schema v%d — nothing to migrate\n", c.Config, result.ToVersion)
		return nil
	}
	// The migration is a line transform over YAML it does not parse, so
	// "the edit produced a loadable v2 config" has to be an enforced
	// post-condition, not an assumption. safeWriteConfig validates through
	// config.Load + ValidateRules, backs the original up, and renames
	// atomically — the same protocol every other config writer uses.
	if err := safeWriteConfig(context.Background(), deps, c.Config, result.Output, src); err != nil {
		return &exitError{code: 3, msg: migrationWriteError(c.Config, err)}
	}
	writeMigrationPreview(deps, c.Config, result)
	_, _ = fmt.Fprintf(deps.Stdout, "\napplied to %s\n", c.Config)
	return nil
}

// migrationWriteError renders a rejected migration write.
//
// The post-condition rejects the result when the line transform could not reach
// every retired key — a flow mapping (`gate: {min_band: mixed}`) keeps them on
// one line, which the per-line regex cannot split. config's own error ends in
// MigrationHint, which names THIS command, so surfacing it verbatim tells the
// user to re-run the thing that just failed. Replace it with the manual remedy.
func migrationWriteError(path string, err error) string {
	msg := fmt.Sprintf("error: migrating %s: %v", path, err)
	if !strings.Contains(msg, config.MigrationHint) {
		return msg
	}
	return strings.ReplaceAll(msg, config.MigrationHint,
		"the migration is a line transform and cannot rewrite these keys where they are written "+
			"(most often a flow mapping such as `gate: {min_band: …}`) — rewrite that mapping in block "+
			"style, one key per line, and re-run. "+path+" was not modified")
}

// migrationReport is the JSON envelope. It names the file so a report captured
// from a sweep can be traced back to the config it describes.
type migrationReport struct {
	initcfg.MigrationResult
	Config string `json:"config"`
}

func writeMigrationPreview(deps *appDeps, path string, r initcfg.MigrationResult) {
	if r.Status == initcfg.MigrationUnversioned {
		_, _ = fmt.Fprintf(deps.Stdout,
			"%s declares no root `version:` key — nothing to migrate from. "+
				"Add `version: %d` at column zero and re-run.\n", path, r.ToVersion)
		return
	}
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
