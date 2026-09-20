package evidence

// MeasurementProfile identifies the producers and settings behind a measurement.
// It deliberately excludes source paths, observed counts, and process metadata.
type MeasurementProfile struct {
	Version      string                `json:"version"`
	SettingsHash string                `json:"settings_hash"`
	Producers    []MeasurementProducer `json:"producers"`
	Unknowns     []string              `json:"unknowns"`
}

// MeasurementProducer records an extractor contract and its actual availability.
type MeasurementProducer struct {
	Tool             string                  `json:"tool"`
	SemanticsVersion string                  `json:"semantics_version"`
	ToolVersion      string                  `json:"tool_version,omitempty"`
	Status           string                  `json:"status"`
	PartialBasis     MeasurementPartialBasis `json:"partial_basis,omitempty"`
}

// MeasurementPartialBasis names a completed acquisition's remaining precision gap.
type MeasurementPartialBasis string

const (
	// PartialUnresolvedSpecifiers means the full input tree was scanned.
	PartialUnresolvedSpecifiers MeasurementPartialBasis = "unresolved_specifiers"
	// PartialDegradedPrecision means imports are complete but type precision is not.
	PartialDegradedPrecision MeasurementPartialBasis = "degraded_precision"
)

// PartialFromUnresolvedSpecifiers identifies a completed TS/Python import scan.
func PartialFromUnresolvedSpecifiers(row Coverage) bool {
	return row.Status == StatusPartial && row.Unresolved > 0 && (row.Tool == "dependency-cruiser" || row.Tool == "grimp")
}

// PartialFromDegradedPrecision identifies complete inputs with reduced precision.
func PartialFromDegradedPrecision(row Coverage) bool {
	return row.Status == StatusPartial && row.UnresolvedInputsMissing == 0 && row.UnresolvedPrecisionOnly > 0
}

// MeasurementProfileVersion is the supported measurement-identity contract.
const MeasurementProfileVersion = "archfit.measurement.v1"

// MeasurementContract is the supported extractor manifest. Changes to facts or
// normalization require a semantics bump; external versions remain exact-match.
func MeasurementContract(tool string) (semantics string, external bool) {
	switch tool {
	case "go/packages", "dependency-cruiser", "grimp", "cargo", "cargo-modules", "scip", "scip-symbols", "ast-grep", "ast-grep/syntax", "jscpd":
		return tool + ".v1", true
	case "git-history":
		return "git-history.recent500-full-fallback.v1", true
	case "loc", "deploy-unit", "supplied-coverage", "runtime", "dynamic-imports", "manifests", "ownership", "history":
		return tool + ".v1", false
	default:
		return "", false
	}
}
