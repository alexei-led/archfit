// Package gitnexus provides an optional dependant-count provider backed by the
// gitnexus CLI's knowledge graph. It is always an enrichment, never required for
// correctness.
//
// Tool selection is three-state on tools.gitnexus.enabled:
//
//   - on            → always attempt to query the index.
//   - off (explicit)→ never query; if a .gitnexus/.codegraph index is present,
//     report it as present-but-disabled (the actionable case — flip the flag)
//     rather than ignoring it silently.
//   - auto / unset  → auto-detect: query iff a .gitnexus/.codegraph index is
//     present on disk. This fixes the prior blind spot where an opt-in default
//     silently ignored an index that was sitting right there.
//
// Refresh archfit's own index with `node .gitnexus/run.cjs analyze --index-only`
// (the --index-only flag keeps gitnexus from rewriting CLAUDE.md / installing
// skills); archfit only reads the index, it never regenerates it.
//
// Real CLI contract (verified 2026-06-11 against gitnexus's Cypher interface):
//
//	gitnexus cypher -r <root> "<query>"
//
// stdout is a JSON envelope {"markdown": "<table>", "row_count": N}; logs go
// to stderr. The query aggregates, per source file, the count of DISTINCT
// other files whose defined symbols reference this file's defined symbols
// (CALLS/ACCESSES/IMPORTS/EXTENDS) — a repo-wide dependant-count map in one
// deterministic query.
//
// When used and the CLI + index are present, Run returns
// map[string]int (repo-relative file path → distinct dependant-file count).
// When disabled, absent, or the repo is not indexed, it returns an empty map
// with absent/partial coverage — never an error.
package gitnexus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolName        = "gitnexus"
	statusOK        = "ok"
	statusPartial   = "partial"
	statusAbsent    = "absent"
	gitnexusTimeout = 5 * time.Minute

	// Absent/partial-coverage reasons: why historical-impact enrichment is n/a
	// and the enable step. Static strings so a double-run stays byte-stable.
	reasonDisabledHasIndex = "a gitnexus index is present but disabled — set `tools.gitnexus.enabled: on` (or remove the `off` override) to use it"
	reasonDisabledNoIndex  = "gitnexus is disabled (`tools.gitnexus.enabled: off`)"
	reasonOptInNoIndex     = "gitnexus is auto-detected — run `gitnexus analyze` (or `node .gitnexus/run.cjs analyze --index-only`) to build an index; it is used automatically once present"
	reasonNotInstalled     = "gitnexus CLI not found — install it and run `gitnexus analyze`"
	reasonNotIndexed       = "gitnexus index missing, stale, or unreadable — run `gitnexus analyze` in the repo"
	reasonHasIndexNoCLI    = "a gitnexus index is present but the gitnexus CLI is not installed — install it to use the index"
	reasonAutoDetected     = "gitnexus index auto-detected (.gitnexus/.codegraph present); refresh with `node .gitnexus/run.cjs analyze --index-only`"
)

// indexDirs are the on-disk index directories archfit recognises as "gitnexus
// present" when the tool is opt-in disabled.
var indexDirs = []string{".gitnexus", ".codegraph"}

// dependantsQuery counts, per file, the distinct other files that reference
// its defined symbols. Ordered for deterministic output.
const dependantsQuery = `MATCH (fa:File)-[d1:CodeRelation]->(sa)-[r:CodeRelation]->(sb)<-[d2:CodeRelation]-(fb:File) ` +
	`WHERE d1.type='DEFINES' AND d2.type='DEFINES' ` +
	`AND r.type IN ['CALLS','ACCESSES','IMPORTS','EXTENDS'] ` +
	`AND fa.filePath <> fb.filePath ` +
	`RETURN fb.filePath AS file, COUNT(DISTINCT fa.filePath) AS dependants ` +
	`ORDER BY file`

// cypherEnvelope mirrors the gitnexus cypher JSON output.
type cypherEnvelope struct {
	Markdown string `json:"markdown"`
	RowCount int    `json:"row_count"`
	Error    string `json:"error"`
}

// Run queries the gitnexus knowledge graph for root and returns a repo-relative
// file path → distinct dependant-file count map.
//
// Selection is three-state (see the package doc): forceOn always attempts;
// explicitlyDisabled never does (but reports a present index); otherwise the
// provider auto-detects — it attempts iff a .gitnexus/.codegraph index is on
// disk. Returns empty + absent when not used or the CLI is missing, empty +
// partial on any execution or parse failure (including an unindexed repo).
// Never returns a non-nil error — all failures degrade gracefully.
func Run(ctx context.Context, runner toolrun.Runner, root string, forceOn, explicitlyDisabled bool) (map[string]int, diagnostic.Coverage, error) {
	indexPresent := hasIndex(root)

	switch {
	case explicitlyDisabled:
		// Respect an explicit opt-out, but never silently: a present index is the
		// actionable case (flip the flag) so name it.
		return nil, diagnostic.Coverage{Tool: toolName, Status: statusAbsent, Reason: disabledReason(indexPresent)}, nil
	case !forceOn && !indexPresent:
		// Auto-detect mode with nothing on disk to detect.
		return nil, diagnostic.Coverage{Tool: toolName, Status: statusAbsent, Reason: reasonOptInNoIndex}, nil
	}
	// forceOn, or auto-detect with a present index → attempt to use it.

	if _, found := runner.Detect(ctx, toolName); !found {
		// A present index we cannot read (CLI missing) is more actionable as a
		// "install the CLI" message than a generic "not installed".
		if indexPresent {
			return nil, diagnostic.Coverage{Tool: toolName, Status: statusAbsent, Reason: reasonHasIndexNoCLI}, nil
		}
		return nil, diagnostic.Coverage{Tool: toolName, Status: statusAbsent, Reason: reasonNotInstalled}, nil
	}

	partial := diagnostic.Coverage{Tool: toolName, Status: statusPartial, Reason: reasonNotIndexed}

	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolName,
		Args:    []string{"cypher", "-r", root, dependantsQuery},
		WorkDir: root,
		Timeout: gitnexusTimeout,
	})
	if err != nil || out.ExitCode != 0 {
		return nil, partial, nil
	}

	impact, perr := parseDependants(out.Stdout)
	if perr != nil {
		return nil, partial, nil
	}

	cov := diagnostic.Coverage{
		Tool:            toolName,
		FilesSeen:       len(impact),
		FilesApplicable: len(impact),
		Status:          statusOK,
	}
	if !forceOn {
		// Used via auto-detection (no explicit opt-in): make the run
		// self-documenting and point at the refresh command.
		cov.Reason = reasonAutoDetected
	}
	return impact, cov, nil
}

// parseDependants parses the cypher JSON envelope and its two-column markdown
// table (| file | dependants |) into a file → count map.
func parseDependants(data []byte) (map[string]int, error) {
	var env cypherEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Error != "" {
		return nil, errCypher(env.Error)
	}

	m := make(map[string]int, env.RowCount)
	for line := range strings.SplitSeq(env.Markdown, "\n") {
		cells := strings.Split(line, "|")
		// A data row is "| <file> | <count> |" → 4 cells after split.
		if len(cells) != 4 {
			continue
		}
		file := strings.TrimSpace(cells[1])
		count, err := strconv.Atoi(strings.TrimSpace(cells[2]))
		if err != nil || file == "" || file == "file" {
			continue // header, separator, or non-numeric row
		}
		m[file] = count
	}
	return m, nil
}

// disabledReason distinguishes "index present but explicitly off" (the actionable
// case — flip the flag) from "off and no index", so a present-but-disabled index
// is never reported as silently absent.
func disabledReason(indexPresent bool) string {
	if indexPresent {
		return reasonDisabledHasIndex
	}
	return reasonDisabledNoIndex
}

// hasIndex reports whether a gitnexus/codegraph index directory exists at root.
func hasIndex(root string) bool {
	for _, dir := range indexDirs {
		if info, err := os.Stat(filepath.Join(root, dir)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// errCypher carries the gitnexus-reported query error.
type errCypher string

func (e errCypher) Error() string { return "gitnexus cypher: " + string(e) }
