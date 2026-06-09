// Package gitnexus provides an optional, opt-in historical symbol-impact
// provider backed by the gitnexus CLI. It is gated by tools.gitnexus.enabled
// (never auto) because gitnexus may require network access and is always an
// enrichment, never required for correctness.
//
// When enabled and the gitnexus CLI is found, Run queries per-module historical
// change impact and returns a map[string]int (module path → historical impact
// count). When disabled, absent, or any non-fatal failure occurs, it returns an
// empty map with absent/partial coverage — never an error.
//
// The impact map is used by risk_hub as an optional multiplicative factor that
// refines, but does not replace, the primary surface-breadth ranking.
//
// Assumed gitnexus CLI contract (adjust if the real binary differs):
//
//	gitnexus impact --json <root>
//
// Output: a JSON array of objects with "module" (string) and "impact" (int)
// fields, e.g.:
//
//	[{"module":"internal/store","impact":42},{"module":"internal/util","impact":3}]
//
// The module key must match the module keys used in the symbol.Graph (i.e. the
// Go module-relative paths produced by the SCIP reader). If the real gitnexus
// output uses a different key space, add a normalisation step here.
package gitnexus

import (
	"context"
	"encoding/json"
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

// impactRecord mirrors one element of the gitnexus JSON output array.
type impactRecord struct {
	Module string `json:"module"`
	Impact int    `json:"impact"`
}

// Run invokes the gitnexus CLI over root and returns a module → historical
// impact count map.
//
// Returns empty + absent when enabled is false or the tool is not in PATH.
// Returns empty + partial on any execution or parse failure.
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
		Args:    []string{"impact", "--json", root},
		WorkDir: root,
		Timeout: gitnexusTimeout,
	})
	if err != nil || out.ExitCode != 0 {
		return nil, partial, nil
	}

	impact, perr := parseImpact(out.Stdout)
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

// parseImpact parses the gitnexus JSON array into a module→impact map.
func parseImpact(data []byte) (map[string]int, error) {
	var records []impactRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	m := make(map[string]int, len(records))
	for _, r := range records {
		if r.Module != "" {
			m[r.Module] = r.Impact
		}
	}
	return m, nil
}
