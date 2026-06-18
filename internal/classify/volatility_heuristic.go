package classify

import (
	"strings"

	"github.com/alexei-led/archfit/internal/model/coupling"
)

// pathPatternEntry maps a path prefix to a domain-volatility band.
// Patterns are matched as exact prefix (after stripping a leading slash).
// Order is significant: first match wins.
type pathPatternEntry struct {
	prefix string
	vol    coupling.Volatility
}

// domainVolatilityTable is the ordered path-pattern → domain-volatility table.
//
// Source: Balanced Coupling §4.3 generic/supporting subdomain defaults.
//   - generic paths (vendor, lib, util, pkg/common): low domain volatility —
//     stable interface contracts, provider-switchable implementation.
//   - supporting paths (infra, platform, storage, db, cache, queue, messaging):
//     medium domain volatility — infrastructure churn but stable at the API boundary.
//
// The table is conservative: only two levels are assigned from paths; unknown
// is returned when no pattern matches (never guess core/high from a path alone).
// Configuration-supplied volatility and subdomain always take precedence (see
// classifyVolatility); this heuristic fires only as the last resort before unknown.
var domainVolatilityTable = []pathPatternEntry{
	// Generic-subdomain indicators → low domain volatility.
	{prefix: "vendor/", vol: coupling.VolatilityLow},
	{prefix: "lib/", vol: coupling.VolatilityLow},
	{prefix: "util/", vol: coupling.VolatilityLow},
	{prefix: "utils/", vol: coupling.VolatilityLow},
	{prefix: "pkg/common/", vol: coupling.VolatilityLow},
	{prefix: "pkg/util/", vol: coupling.VolatilityLow},
	{prefix: "shared/", vol: coupling.VolatilityLow},
	{prefix: "common/", vol: coupling.VolatilityLow},

	// Supporting-subdomain indicators → medium domain volatility.
	{prefix: "infra/", vol: coupling.VolatilityMedium},
	{prefix: "platform/", vol: coupling.VolatilityMedium},
	{prefix: "storage/", vol: coupling.VolatilityMedium},
	{prefix: "db/", vol: coupling.VolatilityMedium},
	{prefix: "database/", vol: coupling.VolatilityMedium},
	{prefix: "cache/", vol: coupling.VolatilityMedium},
	{prefix: "queue/", vol: coupling.VolatilityMedium},
	{prefix: "messaging/", vol: coupling.VolatilityMedium},
}

// domainVolatilityFromPath returns the domain-volatility band implied by the
// module path, or VolatilityUnknown when no pattern matches.
//
// Matching uses path-prefix semantics: the module's first path segment is
// compared against each table entry's prefix. Both the module path and the
// prefix are lowercased before comparison so the table stays case-insensitive.
//
// This heuristic is a source-priority-3 fallback, used only when no explicit
// volatility or subdomain is set on the module definition (see classifyVolatility).
func domainVolatilityFromPath(modulePath string) coupling.Volatility {
	lower := strings.ToLower(modulePath)
	// Normalize: strip a leading slash if present.
	lower = strings.TrimPrefix(lower, "/")

	for _, entry := range domainVolatilityTable {
		if strings.HasPrefix(lower, entry.prefix) {
			return entry.vol
		}
	}
	return coupling.VolatilityUnknown
}
