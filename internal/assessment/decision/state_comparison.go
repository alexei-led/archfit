package decision

import (
	"fmt"

	"github.com/alexei-led/archfit/internal/assessment/result"
)

// Fingerprints are the four inputs a numerical state comparison must agree on.
//
// They are separate values, not one combined hash, so a mismatch can say WHICH
// input moved. "Not comparable" with no reason is indistinguishable from a bug.
type Fingerprints struct {
	// ConfigHash covers the normalized configuration.
	ConfigHash string
	// ModelHash covers the canonical module map and its surface globs. Seam
	// identity is derived from module names, so a rename here would otherwise
	// read as one resolved seam plus one new seam.
	ModelHash string
	// LabelsHash covers the approved label entries. Labels override integration
	// strength, which moves seam severity and the distributed-monolith
	// qualification.
	LabelsHash string
	// RubricVersion is the scoring rubric the run was produced under.
	RubricVersion string
}

// fingerprintInputs names each fingerprint for the mismatch reason.
func (f Fingerprints) fields() [4]struct {
	name  string
	value string
} {
	return [4]struct {
		name  string
		value string
	}{
		{"config_hash", f.ConfigHash},
		{"model_hash", f.ModelHash},
		{"labels_hash", f.LabelsHash},
		{"rubric_version", f.RubricVersion},
	}
}

// CompareFingerprints decides whether two runs may be compared numerically.
//
// Comparison is strict by design and no project option may weaken it: a policy
// change that moves a number is not a code change that moves a number, and
// reporting the two the same way is how a config edit gets read as a
// regression. Any mismatch is non_comparable with a named reason — never a
// delta with a caveat attached.
func CompareFingerprints(baseRef string, head, base Fingerprints) *result.StateComparison {
	out := &result.StateComparison{
		Status: result.StateComparisonComparable, BaseRef: baseRef, Reasons: []string{},
	}
	headFields, baseFields := head.fields(), base.fields()
	for i := range headFields {
		if headFields[i].value == baseFields[i].value {
			continue
		}
		out.Status = result.StateComparisonNonComparable
		out.Reasons = append(out.Reasons, fmt.Sprintf(
			"%s differs between the two runs (%s vs %s): a policy change is not a code change",
			headFields[i].name, shortHash(headFields[i].value), shortHash(baseFields[i].value)))
	}
	return out
}

// NonComparableState reports a comparison that could not be attempted at all,
// carrying the caller's own reason.
func NonComparableState(baseRef, reason string) *result.StateComparison {
	return &result.StateComparison{
		Status: result.StateComparisonNonComparable, BaseRef: baseRef, Reasons: []string{reason},
	}
}

// shortHash keeps a mismatch reason readable. An empty value is named as such:
// "" and a 64-char digest are different facts and must not print the same.
func shortHash(v string) string {
	const shown = 12
	switch {
	case v == "":
		return "unset"
	case len(v) <= shown:
		return v
	default:
		return v[:shown]
	}
}
