// Package rust implements the Rust dependency extractor using `cargo metadata`.
// It is an out-of-process adapter that invokes cargo via the toolrun.Runner
// boundary, parses the JSON output, and normalises the result to graph.Facts.
//
// Granularity is crate-level: each workspace member becomes a `package:<crate>`
// node, each registry dependency an `external:<crate>` node, and direct
// dependencies become `depends_on` edges. Intra-crate module edges are out of
// scope; opt-in `rust-analyzer scip` strength is the upgrade path for finer
// resolution. A single-crate repo therefore yields exactly one package node and
// no intra-workspace edges.
package rust
