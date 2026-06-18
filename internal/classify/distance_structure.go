// Package classify — distance_structure.go provides codeStructureDistance,
// the always-available deterministic baseline for the distance composite (§4.2).
package classify

import (
	"strings"

	"github.com/alexei-led/archfit/internal/model/coupling"
)

// codeStructureDistance returns the structural distance between two module names
// based on their position in the package/directory tree. Module names are
// slash-separated path strings (e.g. "internal/metrics/boundary").
//
// Distance mapping (capped at cross_module_diff_owner — cross_deploy_unit is
// reserved for the deploy-unit signal in deployDistance):
//
//   - Modules sharing all but at most one trailing segment (siblings or
//     parent-child) → DistanceCrossModuleSameOwner.
//   - Modules in different subtrees (fewer shared segments) →
//     DistanceCrossModuleDiffOwner.
//
// Single-segment module names (flat namespaces) always return SameOwner because
// depth=1 and shared=0 satisfies shared >= depth-1 (0 >= 0).
// Returns DistanceUnknown when either name is empty.
func codeStructureDistance(fromMod, toMod string) coupling.Distance {
	if fromMod == "" || toMod == "" {
		return coupling.DistanceUnknown
	}

	fromParts := strings.Split(fromMod, "/")
	toParts := strings.Split(toMod, "/")

	shared := 0
	for i := 0; i < len(fromParts) && i < len(toParts); i++ {
		if fromParts[i] != toParts[i] {
			break
		}
		shared++
	}

	depth := len(fromParts)
	if len(toParts) > depth {
		depth = len(toParts)
	}

	// Siblings or parent-child: all but at most one trailing segment is shared.
	if shared >= depth-1 {
		return coupling.DistanceCrossModuleSameOwner
	}
	return coupling.DistanceCrossModuleDiffOwner
}

// ownershipDistance returns the distance contribution from module ownership.
// Returns DistanceSameModule (no signal) when both owners are empty, deferring
// to code-structure distance in ownerless repos.
// Returns DistanceCrossModuleSameOwner when owners match.
// Returns DistanceCrossModuleDiffOwner when owners differ.
// Note: degenerate single-owner suppression is Task 6; this sees raw owner values.
func ownershipDistance(fromOwner, toOwner string) coupling.Distance {
	if fromOwner == "" && toOwner == "" {
		return coupling.DistanceSameModule
	}
	if fromOwner == toOwner {
		return coupling.DistanceCrossModuleSameOwner
	}
	return coupling.DistanceCrossModuleDiffOwner
}

// deployDistance returns the distance contribution from deploy-unit membership.
// Returns DistanceCrossDeployUnit when both modules have a non-empty deploy unit
// that differs. Returns DistanceSameModule (no signal) otherwise, so max()
// defers to other signals when deploy units are absent or equal.
func deployDistance(fromUnit, toUnit string) coupling.Distance {
	if fromUnit != "" && toUnit != "" && fromUnit != toUnit {
		return coupling.DistanceCrossDeployUnit
	}
	return coupling.DistanceSameModule
}

// distanceLevelOrdinal maps Distance to a comparable ordinal for maxDistance.
// Ordinals mirror the scorer's distance point table (scorer.go:56-65) so that
// max() and scoring use a consistent ordering. DistanceUnknown sits between
// SameOwner and DiffOwner (ordinal 2): it is conservative but concrete evidence
// of diff-ownership beats it.
var distanceLevelOrdinal = map[coupling.Distance]int{
	coupling.DistanceSameModule:           0,
	coupling.DistanceCrossModuleSameOwner: 1,
	coupling.DistanceUnknown:              2,
	coupling.DistanceCrossModuleDiffOwner: 3,
	coupling.DistanceCrossDeployUnit:      5,
}

// maxDistance returns the highest-ordinal Distance among the given values.
// Used to combine independent distance signals into the composite.
func maxDistance(distances ...coupling.Distance) coupling.Distance {
	best := coupling.DistanceSameModule
	for _, d := range distances {
		if distanceLevelOrdinal[d] > distanceLevelOrdinal[best] {
			best = d
		}
	}
	return best
}
