package initcfg

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

// SourceFile is one file of the rule-scope source inventory: the files
// `archfit config lint` and `archfit check` judge module ownership over. The
// composition root reads it with the same inventory reader lint uses, so a
// module init proposes is one lint and check can see.
type SourceFile struct {
	// Path is the repo-relative slash path.
	Path string
	// Language is the source language the file's extension names.
	Language string
	// Selector is the graph-node path a public: glob matches for the file;
	// empty when the inventory cannot name it (Rust without cargo metadata).
	Selector string
	// Production is false for test, generated and vendored files.
	Production bool
}

// keepModulesWithSource drops every module in mods[:judged] that owns no
// production file of sources, and every public entry of a kept module that
// names no production node. Modules at judged and beyond are kept unjudged:
// they are Rust crates, whose node identity needs cargo metadata the inventory
// does not carry, so lint and check resolve them where this cannot.
//
// A module owning no production source is a mocks/, generated, test-only or
// out-of-scope tree (loc skips mocks/ and target/; reports/, testdata/,
// build/ and dist/ are excluded by default): check finds no file for it, so a
// module-wide starter rule could never establish its scope, and its public
// entry would fail lint as matching nothing.
//
// One pass is exact: a dropped module owns no production file, so dropping it
// hands none to another module. Its package nodes can still fall under a kept
// ancestor's glob (api/v1 under api/**), so its edges are re-targeted there
// (foldDroppedEdges), not lost.
func keepModulesWithSource(mods []ModuleDef, origins []string, judged int, sources []SourceFile) ([]ModuleDef, []string) {
	owned := inventoryOwners(mods, sources)
	keptMods := make([]ModuleDef, 0, len(mods))
	keptOrigins := make([]string, 0, len(mods))
	for i, m := range mods {
		if i < judged {
			if !slices.ContainsFunc(owned[i], func(f SourceFile) bool { return f.Production }) {
				continue
			}
			m.Public = publicWithSource(m.Public, sources)
		}
		keptMods = append(keptMods, m)
		keptOrigins = append(keptOrigins, origins[i])
	}
	return keptMods, keptOrigins
}

// foldDroppedEdges re-targets every edge into a module keepModulesWithSource
// dropped onto the kept modules whose paths globs own the dropped module's
// sources: the written config keeps them under that ancestor (api/v1 under
// api/**), so check reads server -> api/v1 as server -> api. An edge out of a
// dropped module goes, because the module owns no production file and check
// drops edges from non-production source; so does an edge whose target no kept
// module owns, or one that folds onto its own source.
func foldDroppedEdges(edges []ModuleEdge, discovered, kept []ModuleDef) []ModuleEdge {
	keptNames := make(map[string]struct{}, len(kept))
	defs := make(map[string]policy.ModuleDef, len(kept))
	for i, m := range kept {
		keptNames[m.Name] = struct{}{}
		defs[ownerKey(i)] = policy.ModuleDef{Paths: m.Paths}
	}
	mm := policy.BuildModuleMap(defs)
	owners := make(map[string][]string)
	for _, m := range discovered {
		if _, ok := keptNames[m.Name]; ok {
			continue
		}
		for _, src := range m.Sources {
			key, ok := mm.ModuleFor(src)
			if !ok {
				continue
			}
			if i, err := strconv.Atoi(key); err == nil && !slices.Contains(owners[m.Name], kept[i].Name) {
				owners[m.Name] = append(owners[m.Name], kept[i].Name)
			}
		}
	}
	seen := make(map[ModuleEdge]struct{}, len(edges))
	var out []ModuleEdge
	for _, e := range edges {
		if _, ok := keptNames[e.From]; !ok {
			continue
		}
		targets := owners[e.To]
		if _, ok := keptNames[e.To]; ok {
			targets = []string{e.To}
		}
		for _, to := range targets {
			edge := ModuleEdge{From: e.From, To: to}
			if _, dup := seen[edge]; dup || edge.From == edge.To {
				continue
			}
			seen[edge] = struct{}{}
			out = append(out, edge)
		}
	}
	return out
}

// publicWithSource keeps the public entries that match the selector of at
// least one production file: what config lint's public_matches_nothing check
// asks of an entry, narrowed to code that is not a test or generated stub.
func publicWithSource(public []string, sources []SourceFile) []string {
	var out []string
	for _, p := range public {
		if slices.ContainsFunc(sources, func(f SourceFile) bool {
			if !f.Production || f.Selector == "" {
				return false
			}
			matched, _ := doublestar.Match(p, f.Selector)
			return matched
		}) {
			out = append(out, p)
		}
	}
	return out
}

// inventoryOwners returns, per module index, the inventory files the module owns
// under rule scope's ownership (moduleRuleScope): the most specific paths glob
// matching the file's path, else its selector. Modules are keyed by index, so
// two discovered modules that share a name before disambiguation stay apart.
func inventoryOwners(mods []ModuleDef, sources []SourceFile) [][]SourceFile {
	defs := make(map[string]policy.ModuleDef, len(mods))
	for i, m := range mods {
		defs[ownerKey(i)] = policy.ModuleDef{Paths: m.Paths}
	}
	mm := policy.BuildModuleMap(defs)
	owned := make([][]SourceFile, len(mods))
	for _, f := range sources {
		key, ok := mm.ModuleFor(f.Path)
		if !ok && f.Selector != "" {
			key, ok = mm.ModuleFor(f.Selector)
		}
		if !ok {
			continue
		}
		i, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		owned[i] = append(owned[i], f)
	}
	return owned
}

// ownerKey is a module's ModuleMap key: its index, zero-padded so the map's
// alphabetical tie-break follows discovery order.
func ownerKey(i int) string {
	return fmt.Sprintf("%08d", i)
}

// Reasons Render gives for a starter rule left at gate: warn because init
// cannot prove the rule evaluable over a complete graph.
const (
	gapPython     = "Python discovery builds no import graph"
	gapTypeScript = "TypeScript discovery builds no import graph"
	gapRust       = "Rust analysis adds intra-crate modules discovery cannot see"
)

// maxGapModules bounds how many module names a graph-gap reason lists.
const maxGapModules = 3

// graphGap explains why the discovered edges are not the module graph check
// will judge the starter rules over; empty when they are. Only Go discovery
// builds an import graph, so the graph is complete only when every kept module
// came from Go discovery, no Rust project is present, and no kept module owns
// non-Go source: check scopes a module-wide rule over every file a module
// owns, and a language whose producer init did not run (TypeScript under a
// Go module's web/ui/, with no root package.json) leaves the rule unevaluated.
func graphGap(mods []ModuleDef, origins []string, rustPresent bool, sources []SourceFile) string {
	var reasons []string
	if slices.Contains(origins, langPython) {
		reasons = append(reasons, gapPython)
	}
	if slices.Contains(origins, langTypeScript) {
		reasons = append(reasons, gapTypeScript)
	}
	if rustPresent {
		reasons = append(reasons, gapRust)
	}
	if sources != nil {
		if r := foreignSourceGap(mods, origins, sources); r != "" {
			reasons = append(reasons, r)
		}
	}
	return strings.Join(reasons, "; ")
}

// foreignSourceGap names the Go modules that own source in another language,
// which init's Go-only import graph does not cover.
func foreignSourceGap(mods []ModuleDef, origins []string, sources []SourceFile) string {
	owned := inventoryOwners(mods, sources)
	var names []string
	foreign := map[string]struct{}{}
	for i, files := range owned {
		if origins[i] != langGo {
			continue
		}
		found := false
		for _, f := range files {
			if f.Language != langGo {
				foreign[f.Language] = struct{}{}
				found = true
			}
		}
		if found {
			names = append(names, mods[i].Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	langs := make([]string, 0, len(foreign))
	for l := range foreign {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	listed := names
	if len(listed) > maxGapModules {
		listed = append(slices.Clone(listed[:maxGapModules]), fmt.Sprintf("%d more", len(names)-maxGapModules))
	}
	return fmt.Sprintf("Go module(s) %s also own %s source the Go import graph omits",
		strings.Join(listed, ", "), strings.Join(langs, ", "))
}
