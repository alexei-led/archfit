package pipeline

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

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
