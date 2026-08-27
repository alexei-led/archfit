package evidence

// CoverageFreshness records whether a supplied coverage artifact is attested to
// the source bytes currently under the scan root. Only FreshnessMatched can
// support a measured testability claim.
type CoverageFreshness string

// Supplied-coverage freshness outcomes. The vocabulary is deliberately closed:
// unverified means no usable attestation was supplied; stale means a usable
// attestation names bytes that differ; matched means every named source agrees.
const (
	FreshnessMatched    CoverageFreshness = "matched"
	FreshnessStale      CoverageFreshness = "stale"
	FreshnessUnverified CoverageFreshness = "unverified"
)

// CoverageFact is one normalized per-file fact parsed from a supplied coverage
// artifact. File is always a scan-root-relative slash path. SourcePath identifies
// the artifact the fact came from; SourceRef is producer metadata and never the
// freshness decision by itself.
type CoverageFact struct {
	File         string `json:"file"`
	CoveredUnits int    `json:"covered_units"`
	TotalUnits   int    `json:"total_units"`
	Unit         string `json:"unit"`
	Format       string `json:"format"`
	SourcePath   string `json:"source_path"`
	ToolVersion  string `json:"tool_version,omitempty"`
	SourceRef    string `json:"source_ref,omitempty"`
}

// CoverageIngest is the normalized result for one configured coverage source.
// UnresolvedPaths counts every parsed path that could not be reduced to a file
// within the scan root; such paths are never silently dropped. Freshness is
// recomputed from the sidecar on every run, including parsed-fact cache hits.
type CoverageIngest struct {
	Facts           []CoverageFact    `json:"facts"`
	UnresolvedPaths int               `json:"unresolved_paths"`
	Freshness       CoverageFreshness `json:"freshness"`
	Format          string            `json:"format"`
	ToolVersion     string            `json:"tool_version,omitempty"`
	Reason          string            `json:"reason,omitempty"`
}
