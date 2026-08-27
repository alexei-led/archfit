package score

import (
	"fmt"
	"sort"

	"github.com/alexei-led/archfit/internal/assessment/result"
)

// Distributed-monolith gate modes. They mirror the config vocabulary without
// importing policy: score sits below policy in the layer order.
const (
	SeamGateWarn = "warn"
	SeamGateFail = "fail"
)

// SeamGate is the projected `coupling.gate.distributed_monolith:` view.
//
// There is no Enabled flag by design. The rule always evaluates; an absent
// config block is the warn default, so a missing stanza reports the seams
// diagnostically rather than silently switching the only coupling gate off.
type SeamGate struct {
	// Mode is warn (diagnostic) or fail (blocking). Anything else reads as warn.
	Mode string
	// MaxNewSeams is the tolerated count of newly introduced qualifying seams.
	MaxNewSeams int
}

// SeamReference is the comparison reference for a new-seam claim.
//
// Comparable is the whole point: without a reference whose config, module map,
// labels, and rubric all match, "newly introduced" is not a fact this tool can
// state, so it must not be stated. A legacy baseline is never comparable.
type SeamReference struct {
	Comparable bool
	// Reasons explain a non-comparable reference. They are disclosed, never
	// swallowed: an unrated gate has to say why it abstained.
	Reasons []string
	// QualifyingSeamIDs are the reference's own qualifying seam IDs.
	QualifyingSeamIDs map[string]struct{}
}

// SeamGateResult is the outcome of EvaluateSeamGate.
type SeamGateResult struct {
	// Qualifying is every seam currently meeting the distributed-monolith
	// condition, in stable seam-ID order. It is reported in both modes.
	Qualifying []result.Seam
	// New is the qualifying seams absent from a comparable reference. It is
	// empty and meaningless unless Rated is true.
	New []result.Seam
	// Rated is false when no comparable reference exists. The seam count is
	// still reported; the "newly introduced" claim is not made.
	Rated bool
	// Blocked is true only in fail mode, only when Rated, and only when the
	// new-seam count exceeds MaxNewSeams.
	Blocked bool
	// Reasons carry the trip explanation, or the abstention explanation when
	// the reference is not comparable.
	Reasons []string
}

// EvaluateSeamGate decides the distributed-monolith seam policy.
//
// It is a pure function of the seam ledger, the policy, and the reference, so
// the fail-mode fixture can inject a fabricated comparable reference without a
// baseline file. It reads seams only — never a score, never a band average, and
// never a per-seam number. A seam qualifies because of what it is, not because
// of where it ranks.
func EvaluateSeamGate(seams []result.Seam, gate SeamGate, ref SeamReference) SeamGateResult {
	out := SeamGateResult{Qualifying: qualifyingSeams(seams)}
	if !ref.Comparable {
		// Silence when there is nothing to abstain about: a repository with no
		// qualifying seam has no claim to withhold, and printing the
		// abstention anyway would train readers to ignore the line that
		// matters.
		if len(out.Qualifying) > 0 {
			out.Reasons = append(out.Reasons, fmt.Sprintf(
				"%d distributed-monolith seam(s) present; no comparable reference, so no newly-introduced count is claimed",
				len(out.Qualifying)))
			out.Reasons = append(out.Reasons, ref.Reasons...)
		}
		return out
	}
	out.Rated = true
	for _, s := range out.Qualifying {
		if _, known := ref.QualifyingSeamIDs[s.ID]; !known {
			out.New = append(out.New, s)
		}
	}
	if gate.Mode != SeamGateFail || len(out.New) <= gate.MaxNewSeams {
		return out
	}
	out.Blocked = true
	out.Reasons = append(out.Reasons, fmt.Sprintf(
		"%d newly introduced distributed-monolith seam(s) exceed coupling.gate.distributed_monolith.max_new_seams %d",
		len(out.New), gate.MaxNewSeams))
	for _, s := range out.New {
		out.Reasons = append(out.Reasons, fmt.Sprintf(
			"%s -> %s: %s coupling at %s (%d critical of %d scored edges)",
			s.FromModule, s.ToModule, s.Strength, s.Distance, s.CriticalEdges, s.ScoredEdges))
	}
	return out
}

// qualifyingSeams selects the distributed-monolith seams in stable ID order.
// The ledger is already ordered by module pair; sorting by ID here makes the
// gate's own output independent of that, so a future ledger reordering cannot
// silently reorder gate findings.
func qualifyingSeams(seams []result.Seam) []result.Seam {
	var out []result.Seam
	for _, s := range seams {
		if s.DistributedMonolith {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
