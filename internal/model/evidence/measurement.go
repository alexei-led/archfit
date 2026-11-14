package evidence

// PartialFromUnresolvedSpecifiers identifies a completed TypeScript import scan.
func PartialFromUnresolvedSpecifiers(row Coverage) bool {
	return row.Status == StatusPartial && row.Unresolved > 0 && row.UnresolvedInputsMissing == 0 && row.Tool == "dependency-cruiser"
}

// PartialFromDegradedPrecision identifies complete inputs with reduced precision.
func PartialFromDegradedPrecision(row Coverage) bool {
	return row.Status == StatusPartial && row.UnresolvedInputsMissing == 0 && row.UnresolvedPrecisionOnly > 0
}

// MeasurementContract is the supported extractor manifest. Changes to facts or
// normalization require a semantics bump; external versions remain exact-match.
func MeasurementContract(tool string) (semantics string, external bool) {
	switch tool {
	case "scip":
		return "scip.v3", true
	case "go/packages", "dependency-cruiser", "scip-symbols", "ast-grep/syntax", "cargo-modules":
		return tool + ".v2", true
	case "cargo", "ast-grep", "dotnet-msbuild", "roslyn":
		return tool + ".v1", true
	case "pyimports", "tsimports":
		return tool + ".v1", true
	case "git-history":
		return "git-history.recent500-full-fallback.v2", true
	case "deploy-unit":
		return "deploy-unit.v2", false
	case "loc", "supplied-coverage", "runtime", "dynamic-imports", "manifests", "ownership", "history":
		return tool + ".v1", false
	default:
		return "", false
	}
}
