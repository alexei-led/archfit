// Package metrics defines the Metric interface and the deterministic metric
// implementations (13 as of the completion plan): boundary health
// (encapsulation, unbalanced_edge, cycle, coverage), modularity
// (blast_radius, change_amplification, hidden_coupling, structural_weight,
// complexity), report-only structural risk (risk_hub, architecture_fitness,
// functional_candidates), and the per-change drift signal (change_locality).
// Every metric is a pure function of MetricInput; absent inputs yield n/a,
// never a false zero.
package metrics
