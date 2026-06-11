// Package gitnexus provides an optional, opt-in dependant-count provider
// backed by the gitnexus CLI's knowledge graph. It is gated by
// tools.gitnexus.enabled (never auto) because gitnexus requires its own index
// (`gitnexus analyze`) and is always an enrichment, never required for
// correctness.
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
// When enabled and the CLI + index are present, Run returns
// map[string]int (repo-relative file path → distinct dependant-file count).
// When disabled, absent, or the repo is not indexed, it returns an empty map
// with absent/partial coverage — never an error.
package gitnexus

import (
	"context"
	"encoding/json"
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
)

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
// Returns empty + absent when enabled is false or the tool is not in PATH.
// Returns empty + partial on any execution or parse failure (including a repo
// that has no gitnexus index yet — run `gitnexus analyze` to create one).
// Never returns a non-nil error — all failures degrade gracefully.
func Run(ctx context.Context, runner toolrun.Runner, root string, enabled bool) (map[string]int, diagnostic.Coverage, error) {
	absent := diagnostic.Coverage{Tool: toolName, Status: statusAbsent}
	if !enabled {
		return nil, absent, nil
	}

	if _, found := runner.Detect(ctx, toolName); !found {
		return nil, absent, nil
	}

	partial := diagnostic.Coverage{Tool: toolName, Status: statusPartial}

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

	return impact, diagnostic.Coverage{
		Tool:            toolName,
		FilesSeen:       len(impact),
		FilesApplicable: len(impact),
		Status:          statusOK,
	}, nil
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

// errCypher carries the gitnexus-reported query error.
type errCypher string

func (e errCypher) Error() string { return "gitnexus cypher: " + string(e) }
