// Package calibrate compares two coupling scorers over a coupling.Index and
// reports per-edge agreement between them.
//
// It is pure: no I/O, no os/exec, no YAML — only coupling model types.
package calibrate

import (
	"github.com/alexei-led/archfit/internal/model/coupling"
)

// EdgeAgreement records one edge's scores from both scorers.
type EdgeAgreement struct {
	// EdgeKey is the coupling.Index key (from\x00to\x00kind).
	EdgeKey    string
	Strength   coupling.Strength
	Distance   coupling.Distance
	Volatility coupling.Volatility
	// ScoreA is the result from scorerA (AdditiveScorer).
	ScoreA coupling.EdgeScore
	// ScoreB is the result from scorerB (MultiplicativeScorer).
	ScoreB    coupling.EdgeScore
	BandAgree bool // ScoreA.Band == ScoreB.Band
}

// Report is the calibration result for one repo.
type Report struct {
	RepoPath   string
	EdgeCount  int
	AgreeCount int
	Edges      []EdgeAgreement
}

// BandAgreementRate returns AgreeCount/EdgeCount as a float64.
// Returns 0 when EdgeCount is 0.
func (r Report) BandAgreementRate() float64 {
	if r.EdgeCount == 0 {
		return 0
	}
	return float64(r.AgreeCount) / float64(r.EdgeCount)
}

// Compare runs both scorers over idx and returns a Report.
//
// Only cross-boundary edges are included — same_module and unknown distance
// edges are skipped, matching what classify.Run scores.
func Compare(repoPath string, idx coupling.Index, scorerA, scorerB coupling.Scorer) Report {
	edges := make([]EdgeAgreement, 0, len(idx))
	agreeCount := 0

	for key, cl := range idx {
		// Skip same-module and unknown-distance edges (not scored by classify.Run).
		if cl.Distance == coupling.DistanceSameModule || cl.Distance == coupling.DistanceUnknown {
			continue
		}

		sa := scorerA.Score(cl)
		sb := scorerB.Score(cl)
		agree := sa.Band == sb.Band
		if agree {
			agreeCount++
		}

		edges = append(edges, EdgeAgreement{
			EdgeKey:    key,
			Strength:   cl.Strength,
			Distance:   cl.Distance,
			Volatility: cl.Volatility,
			ScoreA:     sa,
			ScoreB:     sb,
			BandAgree:  agree,
		})
	}

	return Report{
		RepoPath:   repoPath,
		EdgeCount:  len(edges),
		AgreeCount: agreeCount,
		Edges:      edges,
	}
}
