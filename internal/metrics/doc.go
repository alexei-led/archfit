// Package metrics defines the Metric interface, the generic Calculator[In]
// adapter, and the New() registry. The deterministic metric implementations
// live in family sub-packages:
//
//	boundary     encapsulation, unbalanced_edge, cycle, coverage
//	modularity   blast_radius
//
// Each metric implements Calculator[In] for exactly one per-family input
// (signal.CommonInput) and is adapted to the uniform Metric in New(); the
// compiler thus forbids a metric from reading a signal outside its declared
// family. Every metric is a pure function of its
// input; absent inputs yield n/a, never a false zero. Shared scoring helpers
// live in internal/metrics/internal/result; shared module-graph helpers in
// internal/metrics/internal/modgraph.
package metrics
