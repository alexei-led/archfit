package initcfg

import (
	"sort"
	"strings"
)

// ExistingModule represents a module already present in the config file.
type ExistingModule struct {
	Name          string
	Paths         []string
	HasSubdomain  bool
	HasVolatility bool
	HasLayer      bool
}

// PathDelta records a module present in both config and discovery whose paths differ.
type PathDelta struct {
	Name            string
	ConfigPaths     []string
	DiscoveredPaths []string
}

// UpdateReport is the result of DiffModules.
type UpdateReport struct {
	Added            []ModuleDef
	Removed          []ExistingModule
	PathDrift        []PathDelta
	Unclassified     []string
	StructuralInSync bool
}

// normalizePaths returns a sorted, deduplicated, non-empty slice copy for comparison.
func normalizePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// pathSetsEqual reports whether two path slices are equal as normalized sets.
func pathSetsEqual(a, b []string) bool {
	na := normalizePaths(a)
	nb := normalizePaths(b)
	if len(na) != len(nb) {
		return false
	}
	for i := range na {
		if na[i] != nb[i] {
			return false
		}
	}
	return true
}

// DiffModules computes the structural difference between the existing config modules
// and freshly discovered modules. Output slices are sorted by Name for determinism.
//
//   - Added: modules in fresh (by Name) not in existing.
//   - Removed: modules in existing (by Name) not in fresh.
//   - PathDrift: modules present in both whose paths differ as normalized sets.
//     ConfigPaths and DiscoveredPaths preserve their original ordering.
//   - StructuralInSync: true when Added, Removed, and PathDrift are all empty.
//   - Unclassified: non-removed existing modules missing any of subdomain/volatility/layer.
//     Modules in Removed are excluded from Unclassified.
func DiffModules(existing []ExistingModule, fresh []ModuleDef) UpdateReport {
	existingByName := make(map[string]ExistingModule, len(existing))
	for _, e := range existing {
		existingByName[e.Name] = e
	}

	freshByName := make(map[string]ModuleDef, len(fresh))
	for _, f := range fresh {
		freshByName[f.Name] = f
	}

	var added []ModuleDef
	for _, f := range fresh {
		if _, found := existingByName[f.Name]; !found {
			added = append(added, f)
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i].Name < added[j].Name })

	removedSet := make(map[string]struct{})
	var removed []ExistingModule
	for _, e := range existing {
		if _, found := freshByName[e.Name]; !found {
			removed = append(removed, e)
			removedSet[e.Name] = struct{}{}
		}
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].Name < removed[j].Name })

	var drift []PathDelta
	for _, e := range existing {
		f, found := freshByName[e.Name]
		if !found {
			continue
		}
		if !pathSetsEqual(e.Paths, f.Paths) {
			drift = append(drift, PathDelta{
				Name:            e.Name,
				ConfigPaths:     e.Paths,
				DiscoveredPaths: f.Paths,
			})
		}
	}
	sort.Slice(drift, func(i, j int) bool { return drift[i].Name < drift[j].Name })

	var unclassified []string
	for _, e := range existing {
		if _, isRemoved := removedSet[e.Name]; isRemoved {
			continue
		}
		if !e.HasSubdomain || !e.HasVolatility || !e.HasLayer {
			unclassified = append(unclassified, e.Name)
		}
	}
	sort.Strings(unclassified)

	return UpdateReport{
		Added:            added,
		Removed:          removed,
		PathDrift:        drift,
		Unclassified:     unclassified,
		StructuralInSync: len(added) == 0 && len(removed) == 0 && len(drift) == 0,
	}
}
