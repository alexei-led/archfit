package initcfg

import (
	"fmt"
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
	HasOwner      bool
}

// PathDelta records a module present in both config and discovery whose paths differ.
type PathDelta struct {
	Name            string
	ConfigPaths     []string
	DiscoveredPaths []string
}

// NameDrift records a configured module that discovery re-emitted under a
// different NAME while owning exactly the same paths. It is a naming difference,
// not a structural change: the code did not move, only the key discovery would
// choose for it. `update --apply` never resolves one, because rewriting the key
// would discard the owner, subdomain, volatility, layer, and public settings the
// configured stanza carries.
type NameDrift struct {
	ConfigName     string
	DiscoveredName string
	Paths          []string
}

// UpdateReport is the result of DiffModules.
type UpdateReport struct {
	Added                     []ModuleDef
	Suggested                 []ModuleDef
	Removed                   []ExistingModule
	NameDrift                 []NameDrift
	PathDrift                 []PathDelta
	Unclassified              []string
	Issues                    []ModuleIssue
	Settings                  []SettingChange
	DeployUnitSuggestions     []DeployUnitSuggestion
	DistanceConfigCandidates  []DistanceConfigCandidate
	RuleSuggestions           []RuleSuggestion
	ExternalSystemSuggestions []ExternalSystemSuggestion
	StructuralInSync          bool
}

// DistanceConfigCandidate is one review-only config hint derived from runtime or
// dynamic evidence. It is a display copy of diagnostic.DistanceConfigCandidate;
// config update never applies it automatically.
type DistanceConfigCandidate struct {
	SourceBlock           string
	Module                string
	Target                string
	IntegrationKind       string
	Count                 int
	EvidenceRefs          []string
	SuggestedReviewAction string
}

// DeployUnitSuggestion is one deterministic deploy_unit proposal from checked-in
// deploy-unit evidence (Go main packages, package manifests, Dockerfiles, or k8s
// manifests). It is rendered as a human-reviewed config hint; update --apply does
// not write it automatically because deploy topology is an architecture decision.
type DeployUnitSuggestion struct {
	Module string
	Unit   string
	Source string
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
//     ResolveNameDrift recomputes it over its reclassified buckets.
//   - Unclassified: non-removed existing modules that cannot be classified —
//     neither subdomain nor volatility is set (either one supplies volatility), or
//     layer is missing while requireLayer says an active layer policy needs it.
//     Modules in Removed are excluded from Unclassified.
//   - Issues: the Unclassified reasons plus missing owner, one entry per gap,
//     sorted by module then code. Modules with no paths classify nothing and are
//     skipped, mirroring config.Config.Lint; Unclassified keeps them.
//
// Narrowing Unclassified also narrows what `--ai-classify` targets, deliberately:
// `layer:` is read only by the forbidden_layer_direction rule, so proposing one
// for a config without that rule adds a field nothing consumes.
//
// requireLayer must be true only when a forbidden_layer_direction rule is active
// (gate other than "off"); layer is optional for every other config.
func DiffModules(existing []ExistingModule, fresh []ModuleDef, requireLayer bool) UpdateReport {
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
	var issues []ModuleIssue
	for _, e := range existing {
		if _, isRemoved := removedSet[e.Name]; isRemoved {
			continue
		}
		missingVolatilityInput := !e.HasSubdomain && !e.HasVolatility
		missingLayer := requireLayer && !e.HasLayer
		if missingVolatilityInput || missingLayer {
			unclassified = append(unclassified, e.Name)
		}
		if len(normalizePaths(e.Paths)) == 0 {
			// A pathless entry classifies nothing, so it raises no issue — this
			// mirrors config.Config.Lint. Unclassified deliberately does NOT skip
			// it: a pathless module still needs classification input before it
			// can do any work, and the AI pass targets that list.
			continue
		}
		// Owner and subdomain/volatility are also checked by config.Config.Lint for
		// the analyze pipeline; here they are scoped to modules discovery still sees.
		if missingLayer {
			issues = append(issues, newModuleIssue(IssueMissingLayer, e.Name))
		}
		if !e.HasOwner {
			issues = append(issues, newModuleIssue(IssueMissingOwner, e.Name))
		}
		if missingVolatilityInput {
			issues = append(issues, newModuleIssue(IssueMissingVolatilityInput, e.Name))
		}
	}
	sort.Strings(unclassified)
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Module != issues[j].Module {
			return issues[i].Module < issues[j].Module
		}
		return issues[i].Code < issues[j].Code
	})

	return UpdateReport{
		Added:            added,
		Removed:          removed,
		PathDrift:        drift,
		Unclassified:     unclassified,
		Issues:           issues,
		StructuralInSync: len(added) == 0 && len(removed) == 0 && len(drift) == 0,
	}
}

// ResolveNameDrift reclassifies every Added/Removed pair that owns exactly the
// same paths as a NAMING difference rather than a structural change.
//
// DiffModules matches config modules to discovered modules by NAME, and the two
// naming conventions do not have to agree: this repo configures
// `internal/agenttask` while discovery emits `agenttask` for the identical
// `internal/agenttask/**` path set. Read as add + remove, that is 75 phantom
// "changes" whose only honest resolution — rewriting every key — would discard
// the owner, subdomain, volatility, layer, and public settings on 44 stanzas.
//
// Pairing is by normalized path set and strictly 1:1. A path set claimed by more
// than one added or removed module is ambiguous and stays in its original
// bucket: guessing which stanza a key belongs to is exactly the failure mode
// this pass exists to prevent.
//
// The returned report keeps every other field; Added, Removed, NameDrift, and
// StructuralInSync are recomputed. Pure: no I/O, deterministic order.
func ResolveNameDrift(r UpdateReport) UpdateReport {
	addedByPaths := uniquePathKeyIndex(len(r.Added), func(i int) []string { return r.Added[i].Paths })
	removedByPaths := uniquePathKeyIndex(len(r.Removed), func(i int) []string { return r.Removed[i].Paths })

	paired := make(map[int]int, len(r.Added)) // added index → removed index
	for key, ai := range addedByPaths {
		if ri, ok := removedByPaths[key]; ok {
			paired[ai] = ri
		}
	}
	if len(paired) == 0 {
		return r
	}

	pairedRemoved := make(map[int]struct{}, len(paired))
	drift := make([]NameDrift, 0, len(paired))
	added := make([]ModuleDef, 0, len(r.Added))
	for i, def := range r.Added {
		ri, ok := paired[i]
		if !ok {
			added = append(added, def)
			continue
		}
		pairedRemoved[ri] = struct{}{}
		drift = append(drift, NameDrift{
			ConfigName:     r.Removed[ri].Name,
			DiscoveredName: def.Name,
			Paths:          normalizePaths(def.Paths),
		})
	}

	removed := make([]ExistingModule, 0, len(r.Removed))
	for i, e := range r.Removed {
		if _, isPaired := pairedRemoved[i]; !isPaired {
			removed = append(removed, e)
		}
	}

	out := r
	out.Added = added
	out.Removed = removed
	out.NameDrift = drift
	out.StructuralInSync = len(added) == 0 && len(removed) == 0 && len(drift) == 0 && len(r.PathDrift) == 0
	return out
}

// uniquePathKeyIndex maps each normalized path set to the single index that owns
// it. A path set claimed by two or more entries is dropped: it cannot be paired
// unambiguously.
func uniquePathKeyIndex(n int, pathsAt func(int) []string) map[string]int {
	index := make(map[string]int, n)
	duplicate := make(map[string]struct{})
	for i := range n {
		paths := normalizePaths(pathsAt(i))
		if len(paths) == 0 {
			continue // a pathless stanza owns nothing to pair on
		}
		key := strings.Join(paths, "\x00")
		if _, seen := index[key]; seen {
			duplicate[key] = struct{}{}
			continue
		}
		index[key] = i
	}
	for key := range duplicate {
		delete(index, key)
	}
	return index
}

// writeUnappliedModuleSections renders the two module sections `update --apply`
// deliberately leaves alone: naming differences (same paths, different key) and
// configured modules discovery did not emit. Both would need a stanza rewritten
// or deleted, which discards the settings it carries, so both stay human
// decisions and the section text says so.
func writeUnappliedModuleSections(b *strings.Builder, r UpdateReport) {
	if len(r.NameDrift) > 0 {
		fmt.Fprintf(b, "NAME DRIFT (%d module(s) discovery names differently — same paths, review only):\n", len(r.NameDrift))
		b.WriteString("  NOTE: `update --apply` does NOT rename these; rewriting the key would discard owner, subdomain, volatility, layer, and public settings.\n")
		for _, d := range r.NameDrift {
			fmt.Fprintf(b, "  - config %q, discovery %q: %s\n", d.ConfigName, d.DiscoveredName, joinPaths(d.Paths))
		}
	}

	if len(r.Removed) > 0 {
		fmt.Fprintf(b, "UNMATCHED (%d configured module(s) discovery did not emit — review only):\n", len(r.Removed))
		b.WriteString("  NOTE: `update --apply` does NOT remove these; deleting a stanza discards its settings, so it stays a human decision.\n")
		for _, e := range r.Removed {
			fmt.Fprintf(b, "  - %s: not found in discovery — verify or remove by hand\n", e.Name)
		}
	}
}

// RenderUpdateReport produces a human-readable plan-mode drift report.
// Sections are omitted when their slice is empty.
//
//   - ADDED: paste-ready YAML stanzas via writeModuleStanza (apply=true so fields are
//     visible to copy; out-of-set layers still render as comments per writeModuleStanza rules).
//   - SUGGESTED: paste-ready review-only module override stanzas. These are not
//     structural discovery results and `config update --apply` never writes them.
//   - NAME DRIFT: config/discovery key pairs over one path set, review-only.
//   - UNMATCHED: each configured module discovery did not emit, review-only —
//     `--apply` never deletes a stanza.
//   - PATH DRIFT: config vs discovered paths, with an explicit note that --apply replaces
//     module paths with the discovered paths and writes a backup.
//   - SETTINGS: non-module config settings `update --apply` would write, so the
//     preview shows every edit apply makes.
//   - ISSUES: per-module config gaps with their reason and next action.
//   - UNCLASSIFIED: modules missing classification; shows LLM suggestion from ann when
//     present, otherwise suggests running with --ai-classify.
//   - DEPLOY UNIT HINTS: deterministic deploy_unit proposals from checked-in
//     deploy evidence; review-only because runtime topology is an architecture decision.
//   - DISTANCE CONFIG CANDIDATES: runtime/dynamic review hints for external_systems
//     or deploy_unit; update/apply never writes them.
//   - RULE SUGGESTIONS: review-only deterministic rule or coupling.gate proposals
//     with rationale, basis, and evidence refs; update/apply never writes them.
//   - EXTERNAL SYSTEM SUGGESTIONS: review-only external_systems proposals with
//     targets, volatility, rationale, basis, and evidence refs; update/apply never writes them.
//   - When r.StructuralInSync is true, emits a "structurally in sync" line.
//
// Output is deterministic. Must not import internal/config or internal/llm.
func RenderUpdateReport(r UpdateReport, ann map[string]ModuleAnnotation, allowedLayers []string) string {
	var b strings.Builder

	if len(r.Added) > 0 {
		fmt.Fprintf(&b, "ADDED (%d new module(s) — paste into .archfit.yaml):\n", len(r.Added))
		for _, m := range r.Added {
			var moduleAnn *ModuleAnnotation
			if ann != nil {
				if a, ok := ann[m.Name]; ok {
					moduleAnn = &a
				}
			}
			writeModuleStanza(&b, m.Name, m, allowedLayers, moduleAnn, true)
			writeModuleAnnotationComments(&b, moduleAnn)
		}
	}

	if len(r.Suggested) > 0 {
		fmt.Fprintf(&b, "SUGGESTED (%d review-only module override(s) — paste into .archfit.yaml after review):\n", len(r.Suggested))
		for _, m := range r.Suggested {
			var moduleAnn *ModuleAnnotation
			if ann != nil {
				if a, ok := ann[m.Name]; ok {
					moduleAnn = &a
				}
			}
			writeModuleStanza(&b, m.Name, m, allowedLayers, moduleAnn, true)
			writeModuleAnnotationComments(&b, moduleAnn)
		}
	}

	writeUnappliedModuleSections(&b, r)

	if len(r.PathDrift) > 0 {
		fmt.Fprintf(&b, "PATH DRIFT (%d module(s) — paths changed since last sync):\n", len(r.PathDrift))
		fmt.Fprintf(&b, "  NOTE: `update --apply` REPLACES module paths with the discovered paths; a backup of .archfit.yaml is written before any change.\n")
		for _, d := range r.PathDrift {
			fmt.Fprintf(&b, "  %s:\n", d.Name)
			fmt.Fprintf(&b, "    config:     %s\n", joinPaths(d.ConfigPaths))
			fmt.Fprintf(&b, "    discovered: %s\n", joinPaths(d.DiscoveredPaths))
		}
	}

	if len(r.Settings) > 0 {
		fmt.Fprintf(&b, "SETTINGS (%d config setting(s) `update --apply` would write):\n", len(r.Settings))
		for _, s := range r.Settings {
			fmt.Fprintf(&b, "  - %s: %s\n", sanitizeComment(s.Code), sanitizeComment(s.Reason))
		}
	}

	if len(r.Issues) > 0 {
		fmt.Fprintf(&b, "ISSUES (%d module gap(s) needing a decision):\n", len(r.Issues))
		for _, issue := range r.Issues {
			fmt.Fprintf(&b, "  - %s [%s]: %s → %s\n",
				sanitizeComment(issue.Module), sanitizeComment(issue.Code),
				sanitizeComment(issue.Reason), sanitizeComment(issue.NextAction))
		}
	}

	if len(r.Unclassified) > 0 {
		fmt.Fprintf(&b, "UNCLASSIFIED (%d module(s) archfit cannot classify):\n", len(r.Unclassified))
		for _, name := range r.Unclassified {
			if ann != nil {
				if a, ok := ann[name]; ok {
					writeAnnotationDiff(&b, name, a)
					continue
				}
			}
			fmt.Fprintf(&b, "  - %s: run with --ai-classify to get classification suggestions\n", name)
		}
	}

	if len(r.DeployUnitSuggestions) > 0 {
		fmt.Fprintf(&b, "DEPLOY UNIT HINTS (%d deterministic config proposal(s) — apply manually after review):\n", len(r.DeployUnitSuggestions))
		for _, s := range r.DeployUnitSuggestions {
			writeDeployUnitSuggestion(&b, s)
		}
	}

	if len(r.DistanceConfigCandidates) > 0 {
		fmt.Fprintf(&b, "DISTANCE CONFIG CANDIDATES (%d review-only config proposal(s) — apply manually after review):\n", len(r.DistanceConfigCandidates))
		for _, c := range r.DistanceConfigCandidates {
			writeDistanceConfigCandidate(&b, c)
		}
	}

	if len(r.RuleSuggestions) > 0 {
		fmt.Fprintf(&b, "RULE SUGGESTIONS (%d review-only config proposal(s) — apply manually after review):\n", len(r.RuleSuggestions))
		for _, s := range r.RuleSuggestions {
			writeRuleSuggestion(&b, s)
		}
	}

	if len(r.ExternalSystemSuggestions) > 0 {
		fmt.Fprintf(&b, "EXTERNAL SYSTEM SUGGESTIONS (%d review-only config proposal(s) — apply manually after review):\n", len(r.ExternalSystemSuggestions))
		for _, s := range r.ExternalSystemSuggestions {
			writeExternalSystemSuggestion(&b, s)
		}
	}

	if r.StructuralInSync {
		b.WriteString("structurally in sync — no modules added, removed, or path-drifted\n")
	}

	return b.String()
}

// RenderAppliedLLMReview renders the review-only LLM appendix that still matters
// after update --apply has written structural drift. Structural edits are already
// in the file; this output preserves the semantic proposals that are deliberately
// NOT auto-applied.
func RenderAppliedLLMReview(r UpdateReport, ann map[string]ModuleAnnotation) string {
	var b strings.Builder

	if len(ann) > 0 {
		seen := map[string]struct{}{}
		names := make([]string, 0, len(r.Added)+len(r.Suggested)+len(r.Unclassified))
		collect := func(name string) {
			if _, done := seen[name]; done {
				return
			}
			a, ok := ann[name]
			if !ok {
				return
			}
			if a.Subdomain == "" && a.Volatility == "" && a.Layer == "" && a.Role == "" && a.Basis == "" && len(a.EvidenceRefs) == 0 && a.Rationale == "" {
				return
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
		for _, m := range r.Added {
			collect(m.Name)
		}
		for _, m := range r.Suggested {
			collect(m.Name)
		}
		for _, name := range r.Unclassified {
			collect(name)
		}
		if len(names) > 0 {
			fmt.Fprintf(&b, "LLM MODULE SUGGESTIONS (%d review-only classification proposal(s) — not applied):\n", len(names))
			for _, name := range names {
				writeAnnotationDiff(&b, name, ann[name])
			}
		}
	}

	if len(r.DeployUnitSuggestions) > 0 {
		fmt.Fprintf(&b, "DEPLOY UNIT HINTS (%d deterministic config proposal(s) — not applied):\n", len(r.DeployUnitSuggestions))
		for _, s := range r.DeployUnitSuggestions {
			writeDeployUnitSuggestion(&b, s)
		}
	}

	if len(r.DistanceConfigCandidates) > 0 {
		fmt.Fprintf(&b, "DISTANCE CONFIG CANDIDATES (%d review-only config proposal(s) — not applied):\n", len(r.DistanceConfigCandidates))
		for _, c := range r.DistanceConfigCandidates {
			writeDistanceConfigCandidate(&b, c)
		}
	}

	if len(r.RuleSuggestions) > 0 {
		fmt.Fprintf(&b, "RULE SUGGESTIONS (%d review-only config proposal(s) — not applied):\n", len(r.RuleSuggestions))
		for _, s := range r.RuleSuggestions {
			writeRuleSuggestion(&b, s)
		}
	}

	if len(r.ExternalSystemSuggestions) > 0 {
		fmt.Fprintf(&b, "EXTERNAL SYSTEM SUGGESTIONS (%d review-only config proposal(s) — not applied):\n", len(r.ExternalSystemSuggestions))
		for _, s := range r.ExternalSystemSuggestions {
			writeExternalSystemSuggestion(&b, s)
		}
	}

	return b.String()
}

func writeModuleAnnotationComments(b *strings.Builder, ann *ModuleAnnotation) {
	if ann == nil {
		return
	}
	if ann.Basis != "" {
		fmt.Fprintf(b, "    # basis: %s\n", sanitizeComment(ann.Basis))
	}
	if len(ann.EvidenceRefs) > 0 {
		fmt.Fprintf(b, "    # evidence_refs: %s\n", joinEvidenceRefs(ann.EvidenceRefs))
	}
	if ann.Rationale != "" {
		fmt.Fprintf(b, "    # rationale: %s\n", sanitizeComment(ann.Rationale))
	}
}

func writeAnnotationDiff(b *strings.Builder, name string, a ModuleAnnotation) {
	fmt.Fprintf(b, "  %s:\n", name)
	if a.Subdomain != "" {
		fmt.Fprintf(b, "    + subdomain: %s\n", sanitizeComment(a.Subdomain))
	}
	if a.Volatility != "" {
		fmt.Fprintf(b, "    + volatility: %s\n", sanitizeComment(a.Volatility))
	}
	if a.Layer != "" {
		fmt.Fprintf(b, "    + layer: %s\n", sanitizeComment(a.Layer))
	}
	if a.Role != "" {
		fmt.Fprintf(b, "    + role: %s\n", sanitizeComment(a.Role))
	}
	if a.Basis != "" {
		fmt.Fprintf(b, "    + basis: %s\n", sanitizeComment(a.Basis))
	}
	if len(a.EvidenceRefs) > 0 {
		fmt.Fprintf(b, "    + evidence_refs: %s\n", joinEvidenceRefs(a.EvidenceRefs))
	}
	if a.Rationale != "" {
		fmt.Fprintf(b, "    + rationale: %s\n", sanitizeComment(a.Rationale))
	}
}

func writeDeployUnitSuggestion(b *strings.Builder, s DeployUnitSuggestion) {
	fmt.Fprintf(b, "  %s:\n", sanitizeComment(yamlKey(s.Module)))
	fmt.Fprintf(b, "    deploy_unit: %s\n", yamlScalar(s.Unit))
	if s.Source != "" {
		fmt.Fprintf(b, "    source: %s\n", sanitizeComment(s.Source))
	}
}

func writeDistanceConfigCandidate(b *strings.Builder, c DistanceConfigCandidate) {
	fmt.Fprintf(b, "  - module: %s\n", sanitizeComment(c.Module))
	fmt.Fprintf(b, "    target: %s\n", sanitizeComment(c.Target))
	fmt.Fprintf(b, "    integration_kind: %s\n", sanitizeComment(c.IntegrationKind))
	fmt.Fprintf(b, "    source_block: %s\n", sanitizeComment(c.SourceBlock))
	fmt.Fprintf(b, "    count: %d\n", c.Count)
	fmt.Fprintf(b, "    suggested_review_action: %s\n", sanitizeComment(c.SuggestedReviewAction))
	if len(c.EvidenceRefs) > 0 {
		fmt.Fprintf(b, "    evidence_refs: %s\n", joinEvidenceRefs(c.EvidenceRefs))
	}
}

func writeRuleSuggestion(b *strings.Builder, s RuleSuggestion) {
	fmt.Fprintf(b, "  - type: %s\n", sanitizeComment(s.Type))
	if s.ID != "" {
		fmt.Fprintf(b, "    id: %s\n", sanitizeComment(s.ID))
	}
	if s.SourceModule != "" {
		fmt.Fprintf(b, "    source_module: %s\n", sanitizeComment(s.SourceModule))
	}
	if s.Gate != "" {
		fmt.Fprintf(b, "    gate: %s\n", sanitizeComment(s.Gate))
	}
	if s.From != "" {
		fmt.Fprintf(b, "    from: %s\n", sanitizeComment(s.From))
	}
	if s.To != "" {
		fmt.Fprintf(b, "    to: %s\n", sanitizeComment(s.To))
	}
	if s.Max != nil {
		fmt.Fprintf(b, "    max: %d\n", *s.Max)
	}
	if s.MinBand != "" {
		fmt.Fprintf(b, "    min_band: %s\n", sanitizeComment(s.MinBand))
	}
	if s.MaxDrop != nil {
		fmt.Fprintf(b, "    max_drop: %d\n", *s.MaxDrop)
	}
	if s.Basis != "" {
		fmt.Fprintf(b, "    basis: %s\n", sanitizeComment(s.Basis))
	}
	if len(s.EvidenceRefs) > 0 {
		fmt.Fprintf(b, "    evidence_refs: %s\n", joinEvidenceRefs(s.EvidenceRefs))
	}
	if s.Rationale != "" {
		fmt.Fprintf(b, "    rationale: %s\n", sanitizeComment(s.Rationale))
	}
}

func writeExternalSystemSuggestion(b *strings.Builder, s ExternalSystemSuggestion) {
	fmt.Fprintf(b, "  %s:\n", sanitizeComment(yamlKey(s.Name)))
	if s.SourceModule != "" {
		fmt.Fprintf(b, "    source_module: %s\n", sanitizeComment(s.SourceModule))
	}
	if len(s.Targets) > 0 {
		fmt.Fprintf(b, "    targets: %s\n", joinPaths(s.Targets))
	}
	if s.Volatility != "" {
		fmt.Fprintf(b, "    volatility: %s\n", sanitizeComment(s.Volatility))
	}
	if s.Basis != "" {
		fmt.Fprintf(b, "    basis: %s\n", sanitizeComment(s.Basis))
	}
	if len(s.EvidenceRefs) > 0 {
		fmt.Fprintf(b, "    evidence_refs: %s\n", joinEvidenceRefs(s.EvidenceRefs))
	}
	if s.Rationale != "" {
		fmt.Fprintf(b, "    rationale: %s\n", sanitizeComment(s.Rationale))
	}
}

func joinEvidenceRefs(refs []string) string {
	if len(refs) == 0 {
		return ""
	}
	cleaned := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			cleaned = append(cleaned, sanitizeComment(ref))
		}
	}
	return strings.Join(cleaned, ", ")
}

// joinPaths formats a path slice as a compact bracket list for report output.
func joinPaths(paths []string) string {
	if len(paths) == 0 {
		return "[]"
	}
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
