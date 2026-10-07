package application

import (
	"sort"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/report"
)

// StoredBaseline is the accepted debt a re-anchor may carry into a new
// measurement or scoring epoch. Only stable identities cross: accepted
// fingerprints, qualifying seam IDs, the metric snapshot it disclosed, and the
// fingerprints that say why the old reference stopped comparing.
type StoredBaseline struct {
	Accepted          []BaselineFinding
	QualifyingSeamIDs []string
	Metrics           report.MetricSnapshot
	// State is nil when the stored file has no architecture-state reference.
	State *BaselineStateSnapshot
}

// ReanchorFinding names one current finding a re-anchor left unaccepted.
type ReanchorFinding struct {
	ID         string
	RuleID     string
	FromModule string
	ToModule   string
	// Path is the first location's file, else the source path; empty when the
	// finding has neither.
	Path string
}

// MetricChange is one metric whose value worsened against the stored snapshot.
type MetricChange struct {
	Name   string
	Before float64
	After  float64
}

// ReanchorReport is what a re-anchor carried forward and what it did not, so
// the reviewer of the baseline change sees every difference.
type ReanchorReport struct {
	// Kept counts accepted entries carried forward, one per edge.
	Kept int
	// Dropped are stored entries no current finding matched: fixed debt, or
	// debt whose ID changed. They are never carried forward. Debt a waiver
	// also covers is matched and kept: the stored file already accepted it.
	Dropped []BaselineFinding
	// NotAccepted are the current edges (a rollup member, else the finding) the
	// stored file did not accept, one entry per edge. They stay new.
	NotAccepted []ReanchorFinding
	// DroppedSeams are stored qualifying seams that no longer qualify.
	DroppedSeams []string
	// NewSeams are seams that qualify only now. They stay out of the new file,
	// so the seam gate counts them as new.
	NewSeams []string
	// WorsenedMetrics are metrics whose current value is worse than the stored
	// one. The new file records the current value, so a ratchet resets here.
	WorsenedMetrics []MetricChange
	// DriftReasons name each input that made the stored reference
	// non-comparable; empty when it still compares.
	DriftReasons []string
}

// reanchor keeps from the capture only the debt the stored file already
// accepted. A current finding's keys are its rollup members, else its own ID;
// each key is accepted only when the stored file accepted it, so a new edge in
// an old group stays new. waived holds the keys of findings a waiver covers:
// a plain capture skips them, but a re-anchor keeps the ones the stored file
// accepted, since a baselined status outranks a waiver in check. Qualifying
// seams are the intersection, so a seam that qualifies only in the new epoch
// counts as new, and the hard-gate IDs keep only accepted blockers.
func reanchor(capture BaselineSnapshot, waived []BaselineFinding, current []findingKeys, metrics []report.MetricResult, stored StoredBaseline, drift []string) (BaselineSnapshot, ReanchorReport) {
	storedIDs := make(map[string]bool, len(stored.Accepted))
	for _, a := range stored.Accepted {
		storedIDs[a.Fingerprint] = true
	}
	rep := ReanchorReport{Dropped: []BaselineFinding{}, NotAccepted: []ReanchorFinding{}, DroppedSeams: []string{},
		NewSeams: []string{}, WorsenedMetrics: []MetricChange{}, DriftReasons: append([]string{}, drift...)}

	out := capture
	out.Accepted = nil
	matched := map[string]bool{}
	for _, a := range append(append([]BaselineFinding(nil), capture.Accepted...), waived...) {
		if storedIDs[a.Fingerprint] && !matched[a.Fingerprint] {
			out.Accepted = append(out.Accepted, a)
			matched[a.Fingerprint] = true
		}
	}
	rep.Kept = len(out.Accepted)
	for _, f := range current {
		for _, k := range f.keys {
			if !storedIDs[k] {
				edge := f.ReanchorFinding
				edge.ID = k
				rep.NotAccepted = append(rep.NotAccepted, edge)
			}
		}
	}
	for _, a := range stored.Accepted {
		if !matched[a.Fingerprint] {
			rep.Dropped = append(rep.Dropped, a)
		}
	}

	if capture.State != nil {
		state := *capture.State
		storedSeams := make(map[string]bool, len(stored.QualifyingSeamIDs))
		for _, id := range stored.QualifyingSeamIDs {
			storedSeams[id] = true
		}
		currentSeams := map[string]bool{}
		state.QualifyingSeamIDs = []string{}
		for _, id := range capture.State.QualifyingSeamIDs {
			currentSeams[id] = true
			if storedSeams[id] {
				state.QualifyingSeamIDs = append(state.QualifyingSeamIDs, id)
			} else {
				rep.NewSeams = append(rep.NewSeams, id)
			}
		}
		for _, id := range stored.QualifyingSeamIDs {
			if !currentSeams[id] {
				rep.DroppedSeams = append(rep.DroppedSeams, id)
			}
		}
		state.HardGateFindingIDs = []string{}
		for _, id := range capture.State.HardGateFindingIDs {
			if matched[id] {
				state.HardGateFindingIDs = append(state.HardGateFindingIDs, id)
			}
		}
		out.State = &state
	}

	// The same rule as the metric delta: only a measured value of the same
	// metric version, with a direction, compares with the stored one.
	for _, m := range metrics {
		before, ok := stored.Metrics[m.Name]
		if !ok || before.Version != m.Version || m.Band == string(report.ScoreBandNA) || m.Direction == "" {
			continue
		}
		if worse(m.Direction, before.Value, m.Value) {
			rep.WorsenedMetrics = append(rep.WorsenedMetrics, MetricChange{Name: m.Name, Before: before.Value, After: m.Value})
		}
	}
	sort.Slice(rep.NotAccepted, func(i, j int) bool { return rep.NotAccepted[i].ID < rep.NotAccepted[j].ID })
	sort.Slice(rep.Dropped, func(i, j int) bool { return rep.Dropped[i].Fingerprint < rep.Dropped[j].Fingerprint })
	sort.Strings(rep.DroppedSeams)
	sort.Strings(rep.NewSeams)
	return out, rep
}

// worse reports whether after is worse than before for a metric's direction.
func worse(d report.Direction, before, after float64) bool {
	if d == report.DirectionHigherIsWorse {
		return after > before
	}
	return after < before
}

// findingKeys is one current finding a capture could accept, with the edge
// identities a re-anchor matches on.
type findingKeys struct {
	ReanchorFinding
	keys []string
}

// storedDrift names each input that makes the stored reference differ from
// this run, through the comparison the gate reference itself uses.
func storedDrift(r result.Result, stored StoredBaseline) []string {
	head := decision.Fingerprints{ClassificationHash: r.ClassificationHash, ModelHash: r.ModelHash, LabelsHash: r.LabelsHash,
		RubricVersion: report.ScoreVersion, MeasurementProfile: r.MeasurementProfile}
	return storedComparison(Baseline{Present: true, State: stored.State}, head).Reasons
}
