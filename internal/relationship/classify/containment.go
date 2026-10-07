package classify

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/policy"
)

// Containment is the module containment tree, read from declared `paths:` only.
//
// A module's roots are its path globs without a trailing recursive wildcard. A
// glob that ends in `/**`, `.**` or `::**` owns everything below its root; an
// exact glob, `/*` or `/*.ext` contains nothing. Module P is the parent of M
// when every root of M sits strictly inside a root of a recursive P glob. Plain
// folders add no level: only declared modules do.
//
// The tree only measures how deep two modules sit. It never grades severity:
// every module boundary is the far end of the distance ladder.
type Containment struct {
	parent map[string]string
	depth  map[string]int
}

// containingRoot is one recursive glob of a module: the root and the separator
// that continues a path below it.
type containingRoot struct {
	root string
	sep  string
}

// recursiveSuffixes maps a recursive glob suffix to the separator of its
// language: slash paths (Go, TypeScript), dotted IDs (Python), `::` (Rust).
var recursiveSuffixes = []containingRoot{{"/**", "/"}, {".**", "."}, {"::**", "::"}}

// BuildContainment derives the tree from the module map.
func BuildContainment(modules map[string]policy.ModuleDef) Containment {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	containing := make(map[string][]containingRoot, len(names))
	for _, name := range names {
		for _, glob := range modules[name].Paths {
			for _, s := range recursiveSuffixes {
				if root, ok := strings.CutSuffix(glob, s.root); ok && root != "" {
					containing[name] = append(containing[name], containingRoot{root: root, sep: s.sep})
					break
				}
			}
		}
	}
	c := Containment{parent: map[string]string{}, depth: map[string]int{}}
	for _, name := range names {
		if p := nearestParent(name, modules, names, containing); p != "" {
			c.parent[name] = p
		}
	}
	for _, name := range names {
		c.depth[name] = c.depthOf(name, len(names))
	}
	return c
}

// nearestParent picks, among modules whose recursive root strictly contains
// every root of name, the one with the longest matching root. Ties fall to the
// module name so the choice is stable.
func nearestParent(name string, modules map[string]policy.ModuleDef, names []string, containing map[string][]containingRoot) string {
	roots := moduleRoots(modules[name])
	if len(roots) == 0 {
		return ""
	}
	best, bestLen := "", -1
	for _, cand := range names {
		if cand == name {
			continue
		}
		length, ok := containsAll(containing[cand], roots)
		if ok && length > bestLen {
			best, bestLen = cand, length
		}
	}
	return best
}

// moduleRoots returns the glob roots of a module: each path with any recursive
// suffix removed.
func moduleRoots(def policy.ModuleDef) []string {
	roots := make([]string, 0, len(def.Paths))
	for _, glob := range def.Paths {
		root := glob
		for _, s := range recursiveSuffixes {
			if trimmed, ok := strings.CutSuffix(glob, s.root); ok {
				root = trimmed
				break
			}
		}
		if root != "" {
			roots = append(roots, root)
		}
	}
	return roots
}

// containsAll reports whether every root lies strictly inside one of the
// containing roots, and the length of the shortest matching container root.
func containsAll(containing []containingRoot, roots []string) (int, bool) {
	if len(containing) == 0 {
		return 0, false
	}
	shortest := -1
	for _, root := range roots {
		matched := -1
		for _, c := range containing {
			if strings.HasPrefix(root, c.root+c.sep) && len(c.root) > matched {
				matched = len(c.root)
			}
		}
		if matched < 0 {
			return 0, false
		}
		if shortest < 0 || matched < shortest {
			shortest = matched
		}
	}
	return shortest, shortest >= 0
}

// depthOf counts the ancestors of name plus one. A top-level module has depth
// 1; the system, which contains every module, has depth 0. The limit stops a
// parent cycle, which the strict-containment rule cannot produce.
func (c Containment) depthOf(name string, limit int) int {
	d := 1
	for p, ok := c.parent[name]; ok && d <= limit; p, ok = c.parent[p] {
		d++
	}
	return d
}

// Parent returns the parent module, or "" for a top-level module.
func (c Containment) Parent(name string) string { return c.parent[name] }

// Depth returns the module's level: 1 for a top-level module, 0 for the system
// (the empty name). A module the tree does not know, such as a synthetic Rust or
// go.work module, sits directly under the system at level 1.
func (c Containment) Depth(name string) int {
	if name == "" {
		return 0
	}
	if d, ok := c.depth[name]; ok {
		return d
	}
	return 1
}

// Container returns the last node both modules share: the nearest module that
// is, or contains, both ends. An empty result means the system, the root above
// every top-level module.
func (c Containment) Container(a, b string) string {
	ancestors := map[string]struct{}{}
	for n := a; n != ""; n = c.parent[n] {
		ancestors[n] = struct{}{}
	}
	for n := b; n != ""; n = c.parent[n] {
		if _, ok := ancestors[n]; ok {
			return n
		}
	}
	return ""
}

// Span reports how many containment levels separate two modules. Crossings is
// the number of levels walked up from both ends to their container; Shared is
// the container's own depth (0 for the system).
func (c Containment) Span(a, b string) (crossings, shared int) {
	container := c.Container(a, b)
	shared = c.Depth(container)
	return (c.Depth(a) - shared) + (c.Depth(b) - shared), shared
}
