package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/relationship"
)

func severityFor(strength relationship.Strength, distance relationship.Distance) finding.Severity {
	if strength == relationship.StrengthIntrusive && distance != relationship.DistanceSameModule && distance != relationship.DistanceUnknown && distance != "" {
		return finding.SeverityHigh
	}
	return finding.SeverityMedium
}
