package decision

import (
	"fmt"

	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// Fingerprints are the policy hashes and measurement profile a delta must agree on.
//
// They are separate values, not one combined hash, so a mismatch can say WHICH
// input moved. "Not comparable" with no reason is indistinguishable from a bug.
type Fingerprints struct {
	// ClassificationHash covers the policy leaves that change the compared
	// facts and are not modules (acquisition.ClassificationHash). The raw config
	// bytes are deliberately NOT a comparison input: comments, waivers, rules and
	// `reviewed_at` are governance and must never make a run non-comparable.
	ClassificationHash string
	// ModelHash covers the canonical module map and its surface globs. Seam
	// identity is derived from module names, so a rename here would otherwise
	// read as one resolved seam plus one new seam.
	ModelHash string
	// LabelsHash covers the approved label entries. Labels override integration
	// strength, which moves seam severity and the distributed-monolith
	// qualification.
	LabelsHash string
	// RubricVersion is the scoring rubric the run was produced under.
	RubricVersion      string
	MeasurementProfile *evidence.MeasurementProfile
}

// DriftClass names the kind of input that moved between two runs. A consumer
// that may cross some drift (a re-anchor crosses a scoring epoch or a profile
// change, never a policy edit) reads the class, not the reason text.
type DriftClass string

// Drift classes. The first four are the fingerprint names, so a class is also
// the prefix of its reason.
const (
	DriftClassification      DriftClass = "classification_hash"
	DriftModel               DriftClass = "model_hash"
	DriftLabels              DriftClass = "labels_hash"
	DriftRubric              DriftClass = "rubric_version"
	DriftProfile             DriftClass = "measurement_profile"
	DriftReferenceIncomplete DriftClass = "reference_incomplete"
)

// fields names each fingerprint for the mismatch reason.
func (f Fingerprints) fields() [4]struct {
	class DriftClass
	value string
} {
	return [4]struct {
		class DriftClass
		value string
	}{
		{DriftClassification, f.ClassificationHash},
		{DriftModel, f.ModelHash},
		{DriftLabels, f.LabelsHash},
		{DriftRubric, f.RubricVersion},
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
		class := headFields[i].class
		if baseFields[i].value == "" && class == DriftClassification {
			// A reference written before the classification hash existed says
			// nothing about the policy it was measured under.
			out.Drift = append(out.Drift, string(DriftReferenceIncomplete))
			out.Reasons = append(out.Reasons, "classification_hash is missing from the reference: it predates comparability v2")
			continue
		}
		out.Drift = append(out.Drift, string(class))
		out.Reasons = append(out.Reasons, fmt.Sprintf(
			"%s differs between the two runs (%s vs %s): a policy change is not a code change",
			class, shortHash(headFields[i].value), shortHash(baseFields[i].value)))
	}
	if reasons := CompareMeasurementProfiles(head.MeasurementProfile, base.MeasurementProfile); len(reasons) > 0 {
		out.Status = result.StateComparisonNonComparable
		out.Drift = append(out.Drift, string(DriftProfile))
		out.Reasons = append(out.Reasons, reasons...)
	}
	return out
}

// NonComparableState reports a comparison that could not be attempted at all,
// carrying the caller's own reason.
func NonComparableState(baseRef, reason string) *result.StateComparison {
	return &result.StateComparison{
		Status: result.StateComparisonNonComparable, BaseRef: baseRef, Reasons: []string{reason},
		Drift: []string{string(DriftReferenceIncomplete)},
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
