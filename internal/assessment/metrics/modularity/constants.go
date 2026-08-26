// Package modularity implements the modularity metrics. The shared
// module-graph/history helpers live in internal/assessment/metrics/internal/modgraph.
package modularity

// hubBlastThreshold is the relative-blast fraction at which a module is a "hub"
// whose change is expensive. Tunable; report-only until calibrated on more repos.
const hubBlastThreshold = 0.30
