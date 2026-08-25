package pipeline

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// stripPrefix removes the "kind:" prefix from a node ID (e.g. "file:pkg/a" → "pkg/a").
func stripPrefix(id string) string {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			return id[i+1:]
		}
	}
	return id
}

// dynamicImportSiteCap bounds the number of sample sites stored per module in a
// DynamicImport rollup. Count carries the true total; the cap keeps a module with
// hundreds of lazy imports from bloating the output.
const dynamicImportSiteCap = 5

// BuildDynamicImports groups report-only dynamic/lazy import sites per module
// (module-map key, or the file's directory when unmapped) into DynamicImport
// rollups. Output is deterministic: modules sorted by name, sites already sorted
// by the detector, the per-module sample capped at dynamicImportSiteCap. Returns
// an empty (non-nil) slice when no sites were found. Never touches the graph,
// metrics, or the verdict — this is evidence only.
// It is exported so config update can show the same review-only distance hints
// as analyze without running the full engine pipeline.
func BuildDynamicImports(sites []evidence.DynamicImportSite, mm policy.ModuleMap) []evidence.DynamicImport {
	return buildDynamicImports(sites, mm)
}

func buildDynamicImports(sites []evidence.DynamicImportSite, mm policy.ModuleMap) []evidence.DynamicImport {
	byModule := make(map[string][]evidence.DynamicImportSite)
	for _, s := range sites {
		mod, ok := mm.ModuleForFile(s.File)
		if !ok || mod == "" {
			mod = pathDir(s.File)
		}
		byModule[mod] = append(byModule[mod], s)
	}
	mods := make([]string, 0, len(byModule))
	for m := range byModule {
		mods = append(mods, m)
	}
	sort.Strings(mods)

	out := make([]evidence.DynamicImport, 0, len(mods))
	for _, m := range mods {
		ms := byModule[m]
		sample := ms
		if len(sample) > dynamicImportSiteCap {
			sample = sample[:dynamicImportSiteCap]
		}
		out = append(out, evidence.DynamicImport{
			Module: m,
			Count:  len(ms),
			Sites:  sample,
		})
	}
	return out
}

// runtimeAsyncSiteCap bounds the number of concrete sample sites stored per
// relationship-level runtime edge. Count carries the true total.
const runtimeAsyncSiteCap = 5

// BuildRuntimeAsyncEdges groups async-integration sites by source module,
// runtime target, and integration kind. The result is relationship-level evidence
// for future runtime-distance scoring, but remains report-only today. It is
// exported so config update can show the same review-only distance hints as analyze.
func BuildRuntimeAsyncEdges(sites []evidence.RuntimeAsyncSite, confidence string, mm policy.ModuleMap) []evidence.RuntimeAsyncEdge {
	return buildRuntimeAsyncEdges(sites, confidence, mm)
}

func buildRuntimeAsyncEdges(sites []evidence.RuntimeAsyncSite, confidence string, mm policy.ModuleMap) []evidence.RuntimeAsyncEdge {
	type edgeKey struct {
		fromModule string
		target     string
		kind       string
	}
	byEdge := make(map[edgeKey][]evidence.RuntimeAsyncSite)
	for _, s := range sites {
		fromModule, ok := mm.ModuleForFile(s.File)
		if !ok || fromModule == "" {
			fromModule = pathDir(s.File)
		}
		target := s.Library
		if target == "" {
			target = s.IntegrationKind
		}
		k := edgeKey{fromModule: fromModule, target: target, kind: s.IntegrationKind}
		byEdge[k] = append(byEdge[k], s)
	}
	keys := make([]edgeKey, 0, len(byEdge))
	for k := range byEdge {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].fromModule != keys[j].fromModule {
			return keys[i].fromModule < keys[j].fromModule
		}
		if keys[i].target != keys[j].target {
			return keys[i].target < keys[j].target
		}
		return keys[i].kind < keys[j].kind
	})

	out := make([]evidence.RuntimeAsyncEdge, 0, len(keys))
	for _, k := range keys {
		ss := byEdge[k]
		sample := ss
		if len(sample) > runtimeAsyncSiteCap {
			sample = sample[:runtimeAsyncSiteCap]
		}
		out = append(out, evidence.RuntimeAsyncEdge{
			FromModule:      k.fromModule,
			Target:          k.target,
			IntegrationKind: k.kind,
			Count:           len(ss),
			Confidence:      confidence,
			Sites:           sample,
		})
	}
	return out
}

const (
	dynamicConnascenceKindRuntimeAsync  = "runtime_async"
	dynamicConnascenceKindDynamicImport = "dynamic_import"
	dynamicConnascenceReportOnlyReason  = "static site evidence only; deterministic runtime ordering/value/identity trace evidence is absent"
)

var dynamicConnascenceRelated = []string{
	string(coupling.ConnascenceExecution),
	string(coupling.ConnascenceTiming),
}

var dynamicConnascenceKindsToDiscloseWhenUnmeasured = []string{
	string(coupling.ConnascenceExecution),
	string(coupling.ConnascenceTiming),
	string(coupling.ConnascenceValue),
	string(coupling.ConnascenceIdentity),
}

func dynamicConnascenceUnmeasured(unmeasured []string) []string {
	if len(unmeasured) == 0 {
		return nil
	}
	present := make(map[string]struct{}, len(unmeasured))
	for _, kind := range unmeasured {
		present[kind] = struct{}{}
	}
	out := make([]string, 0, len(dynamicConnascenceKindsToDiscloseWhenUnmeasured))
	for _, kind := range dynamicConnascenceKindsToDiscloseWhenUnmeasured {
		if _, ok := present[kind]; ok {
			out = append(out, kind)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// BuildDynamicConnascenceSignals maps dynamic/lazy imports and runtime async
// edges to report-only dynamic connascence review signals. It is exported so
// config update can derive the same distance-config candidate sources as analyze.
func BuildDynamicConnascenceSignals(dyn []evidence.DynamicImport, runtimeEdges []evidence.RuntimeAsyncEdge, unmeasured []string) *evidence.DynamicConnascenceSignals {
	return buildDynamicConnascenceSignals(dyn, runtimeEdges, unmeasured)
}

func buildDynamicConnascenceSignals(dyn []evidence.DynamicImport, runtimeEdges []evidence.RuntimeAsyncEdge, unmeasured []string) *evidence.DynamicConnascenceSignals {
	if len(dyn) == 0 && len(runtimeEdges) == 0 {
		return nil
	}
	out := &evidence.DynamicConnascenceSignals{
		Signals:          make([]evidence.DynamicConnascenceSignal, 0, len(runtimeEdges)+len(dyn)),
		Unmeasured:       dynamicConnascenceUnmeasured(unmeasured),
		ReportOnlyReason: dynamicConnascenceReportOnlyReason,
	}
	for _, e := range runtimeEdges {
		out.Signals = append(out.Signals, evidence.DynamicConnascenceSignal{
			Kind:               dynamicConnascenceKindRuntimeAsync,
			RelatedConnascence: append([]string(nil), dynamicConnascenceRelated...),
			Measured:           false,
			ReportOnlyReason:   dynamicConnascenceReportOnlyReason,
			Module:             e.FromModule,
			Target:             e.Target,
			IntegrationKind:    e.IntegrationKind,
			Count:              e.Count,
			Sites:              runtimeAsyncDynamicConnascenceSites(e.Sites),
		})
	}
	for _, d := range dyn {
		out.Signals = append(out.Signals, evidence.DynamicConnascenceSignal{
			Kind:               dynamicConnascenceKindDynamicImport,
			RelatedConnascence: append([]string(nil), dynamicConnascenceRelated...),
			Measured:           false,
			ReportOnlyReason:   dynamicConnascenceReportOnlyReason,
			Module:             d.Module,
			Count:              d.Count,
			Sites:              dynamicImportDynamicConnascenceSites(d.Sites),
		})
	}
	return out
}

func runtimeAsyncDynamicConnascenceSites(sites []evidence.RuntimeAsyncSite) []evidence.DynamicConnascenceSite {
	out := make([]evidence.DynamicConnascenceSite, 0, len(sites))
	for _, s := range sites {
		target := s.Library
		if target == "" {
			target = s.IntegrationKind
		}
		out = append(out, evidence.DynamicConnascenceSite{
			File:     s.File,
			Line:     s.Line,
			Kind:     s.IntegrationKind,
			Language: s.Language,
			Target:   target,
		})
	}
	return out
}

func dynamicImportDynamicConnascenceSites(sites []evidence.DynamicImportSite) []evidence.DynamicConnascenceSite {
	out := make([]evidence.DynamicConnascenceSite, 0, len(sites))
	for _, s := range sites {
		out = append(out, evidence.DynamicConnascenceSite{
			File:     s.File,
			Line:     s.Line,
			Kind:     s.Kind,
			Language: s.Language,
		})
	}
	return out
}

const (
	distanceCandidateSourceStaticExternalEdges      = "classified_external_edges"
	distanceCandidateSourceRuntimeAsyncEdges        = "runtime_async_edges"
	distanceCandidateSourceDynamicImports           = "dynamic_imports"
	distanceCandidateSourceDynamicConnascenceSignal = "dynamic_connascence_signals"
	distanceCandidateActionExternalSystems          = "external_systems"
	distanceCandidateActionDeployUnit               = "deploy_unit"
)

// BuildStaticExternalDistanceCandidates turns today’s disclosed-exclusion bucket
// (classified DistanceUnknown external/library edges) into review-only
// external_systems hints. The candidates deliberately do not alter distance,
// scoring, findings, baselines, or gate verdicts: an external seam enters the
// BC model only after a human declares it in config.
func BuildStaticExternalDistanceCandidates(g *graph.Graph, idx coupling.Index, mm policy.ModuleMap) []evidence.DistanceConfigCandidate {
	return buildStaticExternalDistanceCandidates(g, idx, mm)
}

func buildStaticExternalDistanceCandidates(g *graph.Graph, idx coupling.Index, mm policy.ModuleMap) []evidence.DistanceConfigCandidate {
	if g == nil || len(idx) == 0 {
		return nil
	}
	goModules := g.GoModules()
	type candidateKey struct {
		module string
		target string
		kind   string
	}
	type candidateGroup struct {
		count int
		sites []evidence.DistanceConfigEvidenceSite
		seen  map[evidence.DistanceConfigEvidenceSite]struct{}
	}
	groups := make(map[candidateKey]*candidateGroup)
	for _, e := range g.Edges() {
		cl, ok := idx[indexKeyForEdge(e)]
		if !ok || cl.Distance != coupling.DistanceUnknown {
			continue
		}
		fromModule, ok := moduleForNodeID(e.From, mm)
		if !ok || fromModule == "" {
			continue
		}
		if toModule, ok := moduleForNodeID(e.To, mm); ok && toModule != "" {
			continue
		}
		target, rawTarget, ok := normalizeStaticExternalTarget(e.To, e.Language, goModules)
		if !ok || target == "" {
			continue
		}
		k := candidateKey{module: fromModule, target: target, kind: string(e.Kind)}
		grp := groups[k]
		if grp == nil {
			grp = &candidateGroup{seen: make(map[evidence.DistanceConfigEvidenceSite]struct{})}
			groups[k] = grp
		}
		grp.count++
		for _, loc := range e.Locations {
			site := evidence.DistanceConfigEvidenceSite{
				File:     loc.File,
				Line:     loc.Line,
				Kind:     string(e.Kind),
				Language: e.Language,
				Target:   rawTarget,
			}
			if _, exists := grp.seen[site]; exists {
				continue
			}
			grp.seen[site] = struct{}{}
			grp.sites = append(grp.sites, site)
		}
	}
	if len(groups) == 0 {
		return nil
	}
	keys := make([]candidateKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].module != keys[j].module {
			return keys[i].module < keys[j].module
		}
		if keys[i].target != keys[j].target {
			return keys[i].target < keys[j].target
		}
		return keys[i].kind < keys[j].kind
	})
	out := make([]evidence.DistanceConfigCandidate, 0, len(keys))
	for _, k := range keys {
		grp := groups[k]
		out = append(out, evidence.DistanceConfigCandidate{
			SourceBlock:           distanceCandidateSourceStaticExternalEdges,
			Module:                k.module,
			Target:                k.target,
			IntegrationKind:       k.kind,
			Count:                 grp.count,
			EvidenceSites:         grp.sites,
			SuggestedReviewAction: distanceCandidateActionExternalSystems,
		})
	}
	sortDistanceConfigCandidates(out)
	return out
}

// BuildDistanceConfigCandidates turns report-only runtime/dynamic evidence into
// report-only config review hints. The candidates deliberately do not feed
// classify, score, findings, baselines, or gate verdicts.
func BuildDistanceConfigCandidates(
	dyn []evidence.DynamicImport,
	runtimeEdges []evidence.RuntimeAsyncEdge,
	dynamicSignals *evidence.DynamicConnascenceSignals,
) []evidence.DistanceConfigCandidate {
	out := make([]evidence.DistanceConfigCandidate, 0, len(runtimeEdges)+len(dyn))
	for _, e := range runtimeEdges {
		out = append(out, evidence.DistanceConfigCandidate{
			SourceBlock:           distanceCandidateSourceRuntimeAsyncEdges,
			Module:                e.FromModule,
			Target:                e.Target,
			IntegrationKind:       e.IntegrationKind,
			Count:                 e.Count,
			EvidenceSites:         runtimeAsyncDistanceSites(e.Sites),
			SuggestedReviewAction: distanceCandidateActionExternalSystems,
		})
	}
	for _, d := range dyn {
		out = append(out, evidence.DistanceConfigCandidate{
			SourceBlock:           distanceCandidateSourceDynamicImports,
			Module:                d.Module,
			Target:                d.Module,
			IntegrationKind:       dominantDynamicImportKind(d.Sites),
			Count:                 d.Count,
			EvidenceSites:         dynamicImportDistanceSites(d.Sites),
			SuggestedReviewAction: distanceCandidateActionDeployUnit,
		})
	}
	if dynamicSignals != nil {
		for _, s := range dynamicSignals.Signals {
			action := distanceCandidateActionDeployUnit
			target := s.Target
			if target == "" {
				target = s.Module
			} else {
				action = distanceCandidateActionExternalSystems
			}
			kind := s.IntegrationKind
			if kind == "" {
				kind = s.Kind
			}
			out = append(out, evidence.DistanceConfigCandidate{
				SourceBlock:           distanceCandidateSourceDynamicConnascenceSignal,
				Module:                s.Module,
				Target:                target,
				IntegrationKind:       kind,
				Count:                 s.Count,
				EvidenceSites:         dynamicConnascenceDistanceSites(s.Sites),
				SuggestedReviewAction: action,
			})
		}
	}
	sortDistanceConfigCandidates(out)
	return out
}

func sortDistanceConfigCandidates(out []evidence.DistanceConfigCandidate) {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SourceBlock != out[j].SourceBlock {
			return out[i].SourceBlock < out[j].SourceBlock
		}
		if out[i].Module != out[j].Module {
			return out[i].Module < out[j].Module
		}
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return out[i].IntegrationKind < out[j].IntegrationKind
	})
}

func indexKeyForEdge(e graph.Edge) string {
	return e.From + "\x00" + e.To + "\x00" + string(e.Kind)
}

func moduleForNodeID(id string, mm policy.ModuleMap) (string, bool) {
	kind, path, ok := splitNodeID(id)
	if !ok || path == "" {
		return "", false
	}
	if mod, ok := mm.ModuleFor(path); ok {
		return mod, true
	}
	if kind == string(graph.NodeKindFile) {
		return mm.ModuleForFile(path)
	}
	if mm.Has(path) {
		return path, true
	}
	return "", false
}

func splitNodeID(id string) (kind, path string, ok bool) {
	kind, path, ok = strings.Cut(id, ":")
	return kind, path, ok
}

func normalizeStaticExternalTarget(id, lang string, goModules []graph.GoModule) (target, raw string, ok bool) {
	kind, path, ok := splitNodeID(id)
	if !ok || path == "" {
		return "", "", false
	}
	raw = path
	switch kind {
	case string(graph.NodeKindPackage):
		if lang != graph.LangGo {
			return "", raw, false
		}
		target, ok = normalizeGoExternalTarget(path, goModules)
		return target, raw, ok
	case string(graph.NodeKindExternal):
		switch lang {
		case graph.LangTypeScript:
			target, ok = normalizeTypeScriptExternalTarget(path)
			return target, raw, ok
		case graph.LangPython:
			target, ok = normalizePythonExternalTarget(path)
			return target, raw, ok
		case graph.LangRust:
			return path, raw, true
		default:
			return "", raw, false
		}
	default:
		return "", raw, false
	}
}

func normalizeGoExternalTarget(importPath string, goModules []graph.GoModule) (string, bool) {
	parts := strings.Split(importPath, "/")
	if len(parts) == 0 || !strings.Contains(parts[0], ".") {
		return "", false
	}
	for _, mod := range goModules {
		if mod.Path != "" && (importPath == mod.Path || strings.HasPrefix(importPath, mod.Path+"/")) {
			return "", false
		}
	}
	rootLen := 3
	if len(parts) < rootLen {
		rootLen = len(parts)
	}
	root := strings.Join(parts[:rootLen], "/")
	return root + "/**", true
}

func normalizeTypeScriptExternalTarget(target string) (string, bool) {
	if !strings.HasPrefix(target, "node_modules/") {
		return "", false
	}
	trimmed := strings.TrimPrefix(target, "node_modules/")
	root, ok := npmPackageRoot(trimmed)
	if !ok {
		return "", false
	}
	return "node_modules/" + root + "/**", true
}

func normalizePythonExternalTarget(target string) (string, bool) {
	if target == "" || strings.HasPrefix(target, ".") || strings.Contains(target, "..") {
		return "", false
	}
	parts := strings.Split(target, ".")
	for _, part := range parts {
		if !pythonModuleSegment(part) {
			return "", false
		}
	}
	root := parts[0]
	return "{" + root + "," + root + ".*}", true
}

func pythonModuleSegment(seg string) bool {
	if seg == "" {
		return false
	}
	for i, r := range seg {
		switch {
		case r == '_':
			continue
		case r >= 'a' && r <= 'z':
			continue
		case r >= 'A' && r <= 'Z':
			continue
		case i > 0 && r >= '0' && r <= '9':
			continue
		default:
			return false
		}
	}
	return true
}

func npmPackageRoot(target string) (string, bool) {
	parts := strings.Split(target, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", false
	}
	if strings.HasPrefix(parts[0], "@") {
		if len(parts) < 2 || parts[0] == "@" {
			return "", false
		}
		return parts[0] + "/" + parts[1], true
	}
	return parts[0], true
}

func runtimeAsyncDistanceSites(sites []evidence.RuntimeAsyncSite) []evidence.DistanceConfigEvidenceSite {
	out := make([]evidence.DistanceConfigEvidenceSite, 0, len(sites))
	for _, s := range sites {
		target := s.Library
		if target == "" {
			target = s.IntegrationKind
		}
		out = append(out, evidence.DistanceConfigEvidenceSite{
			File:     s.File,
			Line:     s.Line,
			Kind:     s.IntegrationKind,
			Language: s.Language,
			Target:   target,
		})
	}
	return out
}

func dynamicImportDistanceSites(sites []evidence.DynamicImportSite) []evidence.DistanceConfigEvidenceSite {
	out := make([]evidence.DistanceConfigEvidenceSite, 0, len(sites))
	for _, s := range sites {
		out = append(out, evidence.DistanceConfigEvidenceSite{
			File:     s.File,
			Line:     s.Line,
			Kind:     s.Kind,
			Language: s.Language,
		})
	}
	return out
}

func dynamicConnascenceDistanceSites(sites []evidence.DynamicConnascenceSite) []evidence.DistanceConfigEvidenceSite {
	out := make([]evidence.DistanceConfigEvidenceSite, 0, len(sites))
	for _, s := range sites {
		out = append(out, evidence.DistanceConfigEvidenceSite(s))
	}
	return out
}

func dominantDynamicImportKind(sites []evidence.DynamicImportSite) string {
	counts := make(map[string]int, len(sites))
	for _, s := range sites {
		counts[s.Kind]++
	}
	best, bestN := "dynamic_import", 0
	for k, n := range counts {
		if n > bestN || (n == bestN && k < best) {
			best, bestN = k, n
		}
	}
	return best
}

// pathDir returns the directory portion of a repo-relative slash path, or "."
// when the path has no directory. Used as the dynamic-import module key when the
// module map does not cover a file.
func pathDir(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return "."
}
