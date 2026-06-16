// Package modularity implements the modularity metrics (blast_radius,
// change_amplification, hidden_coupling, structural_weight) and the
// functional_candidates metric. All are report-only (band "info"). The shared
// module-graph/history helpers live in internal/metrics/internal/modgraph.
package modularity

// hubBlastThreshold is the relative-blast fraction at which a module is a "hub"
// whose change is expensive. Tunable; report-only until calibrated on more repos.
const hubBlastThreshold = 0.30

// ampHubThreshold is the per-repo change-amplification level (blast×volatility)
// above which a module is a volatile hub. Tunable; the metric is report-only and
// the ranked hub list — not a cross-repo count — is the intended signal.
const ampHubThreshold = 0.15

// hiddenLCThreshold is the logical-coupling level (co-change / min churn) at which
// a non-importing module pair counts as hidden coupling. Tunable, report-only.
const hiddenLCThreshold = 0.5

// hiddenMinSupport is the minimum co-change commits before a pair is considered
// (small samples are noise).
const hiddenMinSupport = 4

// godModuleMultiple is how many times the median module LOC a module must exceed
// (and godModuleFloor absolute LOC) to count as a size-skew god-module. Tunable,
// report-only until calibrated wider.
const (
	godModuleMultiple = 4
	godModuleFloor    = 400
)
