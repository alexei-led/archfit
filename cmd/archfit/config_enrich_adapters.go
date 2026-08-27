package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
)

type configEnrichAdapter struct {
	flags    *enrichFlags
	deps     *appDeps
	spec     valueSpec
	cfg      config.Config
	provider llm.Provider
}

func runConfigEnrichWorkflow(ctx context.Context, flags *enrichFlags, deps *appDeps, kind application.ConfigEnrichKind, spec valueSpec, apply bool, reviewedBy string) error {
	adapter := &configEnrichAdapter{flags: flags, deps: deps, spec: spec}
	service := application.ConfigEnrichService{
		Configs: adapter, Judge: adapter, Drafts: adapter, Editor: adapter,
		Files: adapter, Writer: adapter, Clock: adapter,
	}
	draftName := spec.draftPath
	if kind == application.ConfigEnrichSubdomain {
		draftName = defaultSubdomainsPath
	}
	draftPath := filepath.Join(filepath.Dir(flags.Config), draftName)
	out, err := service.Execute(ctx, application.ConfigEnrichRequest{
		ConfigPath: flags.Config, Root: flags.Root, DraftPath: draftPath,
		Kind: kind, Apply: apply, ReviewedBy: reviewedBy,
	})
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	renderConfigEnrichMissing(deps, kind, out.Missing)
	switch out.Action {
	case application.ConfigEnrichAllSet:
		if kind == application.ConfigEnrichSubdomain {
			_, _ = fmt.Fprintln(deps.Stdout, "config enrich subdomain: all modules already have subdomain set — nothing to draft")
		} else {
			_, _ = fmt.Fprintf(deps.Stdout, "enrich --%s: all modules already have %s set — nothing to draft\n", kind, kind)
		}
	case application.ConfigEnrichDraftsWritten:
		_, _ = fmt.Fprintf(deps.Stdout,
			"enrich: %d draft %s(s) written to %s — review, set status: approved, then run config enrich %s --apply\n",
			out.Count, kind, draftPath, kind)
	case application.ConfigEnrichNoApproved:
		if kind == application.ConfigEnrichSubdomain {
			_, _ = fmt.Fprintln(deps.Stdout, "no approved subdomain drafts found — set status: approved in "+draftPath+" and re-run")
		} else {
			_, _ = fmt.Fprintf(deps.Stdout, "no approved %s drafts found — set status: approved in %s and re-run\n", kind, draftPath)
		}
	case application.ConfigEnrichNoChanges:
		_, _ = fmt.Fprintf(deps.Stdout, "no changes — all approved modules already have %s set\n", kind)
	case application.ConfigEnrichPinned:
		_, _ = fmt.Fprintf(deps.Stdout, "pinned %d %s(s) into %s (reviewed_by: %s)\n", out.Count, kind, flags.Config, out.ReviewedBy)
	}
	return nil
}

func renderConfigEnrichMissing(deps *appDeps, kind application.ConfigEnrichKind, missing []string) {
	if len(missing) == 0 {
		return
	}
	if kind == application.ConfigEnrichSubdomain {
		_, _ = fmt.Fprintf(deps.Stdout, "warning: LLM did not classify %d module(s): %s — they were left unclassified\n", len(missing), strings.Join(missing, ", "))
		return
	}
	_, _ = fmt.Fprintf(deps.Stdout, "warning: LLM did not draft %s for %d module(s): %s\n", kind, len(missing), strings.Join(missing, ", "))
}

func (a *configEnrichAdapter) LoadConfigEnrich(ctx context.Context, path string) (application.ConfigEnrichConfig, error) {
	cfg, err := loadConfig(ctx, path)
	if err != nil {
		return application.ConfigEnrichConfig{}, err
	}
	a.cfg = cfg
	_, configured := cfg.LLM()
	out := application.ConfigEnrichConfig{
		Modules: make(map[string]application.ConfigEnrichModule, len(cfg.Modules)),
		Layers:  append([]string(nil), cfg.Layers...), AIConfigured: configured,
	}
	for name, mod := range cfg.Modules {
		out.Modules[name] = application.ConfigEnrichModule{
			Name: name, Paths: append([]string(nil), mod.Paths...), Subdomain: mod.Subdomain,
			Volatility: mod.Volatility, Owner: mod.Owner,
		}
	}
	return out, nil
}

func (a *configEnrichAdapter) ValidateConfigEnrichJudgment(_ context.Context, _ application.ConfigEnrichKind) error {
	llmCfg, configured := a.cfg.LLM()
	if !configured {
		return nil // application reports the user-facing validation error first
	}
	provider, err := buildCachedProvider(a.flags.providerOverride, llmCfg, llmCacheDir(filepath.Dir(a.flags.Config)), a.flags.Refresh)
	if err != nil {
		return fmt.Errorf("%w (set the key and re-run; see `archfit doctor`)", err)
	}
	a.provider = provider
	return nil
}

func (a *configEnrichAdapter) DraftConfigEnrich(ctx context.Context, req application.ConfigEnrichJudgmentRequest) ([]application.ConfigEnrichDraft, error) {
	modules := make([]initcfg.ModuleDef, 0, len(req.Modules))
	for _, mod := range req.Modules {
		modules = append(modules, initcfg.ModuleDef{Name: mod.Name, Paths: append([]string(nil), mod.Paths...)})
	}
	root := req.Root
	if root == "" {
		root = filepath.Dir(a.flags.Config)
	}
	targets := initcfg.BuildClassifyTargets(root, modules)
	repoEvidence := architectureEvidenceLines(root, modules, a.flags.Config, enrichEvidenceDiagnostics("enrich-"+string(req.Kind), len(targets)))
	if req.Kind == application.ConfigEnrichSubdomain {
		annotations, err := classifyModulesWithEvidence(ctx, a.provider, targets, req.Layers, repoEvidence)
		if err != nil {
			return nil, err
		}
		out := make([]application.ConfigEnrichDraft, 0, len(annotations))
		for _, target := range targets {
			annotation, ok := annotations[target.Name]
			if !ok || annotation.Subdomain == "" {
				continue
			}
			out = append(out, application.ConfigEnrichDraft{
				Module: target.Name, Subdomain: annotation.Subdomain, Volatility: annotation.Volatility,
				Rationale: annotation.Rationale, EvidenceRefs: append([]string(nil), annotation.EvidenceRefs...),
				Basis: annotation.Basis, Status: application.ConfigEnrichDraftStatusDraft,
			})
		}
		return out, nil
	}
	codeowners := ""
	if a.spec.withCodeowners {
		codeowners = readCodeowners(root)
	}
	drafts, err := draftModuleValues(ctx, a.provider, a.spec, targets, codeowners, repoEvidence)
	if err != nil {
		return nil, err
	}
	out := make([]application.ConfigEnrichDraft, 0, len(drafts))
	for _, draft := range drafts {
		out = append(out, application.ConfigEnrichDraft{
			Module: draft.Module, Value: draft.Value, Rationale: draft.Rationale,
			EvidenceRefs: append([]string(nil), draft.EvidenceRefs...), Basis: draft.Basis,
			Status: draft.Status, Confidence: draft.Confidence, Provenance: draft.Provenance,
		})
	}
	return out, nil
}

func (a *configEnrichAdapter) LoadConfigEnrichDrafts(_ context.Context, path string, kind application.ConfigEnrichKind) (application.ConfigEnrichDraftFile, error) {
	if kind == application.ConfigEnrichSubdomain {
		file, err := initcfg.LoadSubdomainDrafts(path)
		if err != nil {
			return application.ConfigEnrichDraftFile{}, err
		}
		out := application.ConfigEnrichDraftFile{Version: file.Version, Field: string(kind), Drafts: make([]application.ConfigEnrichDraft, 0, len(file.Drafts))}
		for _, draft := range file.Drafts {
			out.Drafts = append(out.Drafts, application.ConfigEnrichDraft{
				Module: draft.Module, Subdomain: draft.Subdomain, Volatility: draft.Volatility,
				Rationale: draft.Rationale, EvidenceRefs: append([]string(nil), draft.EvidenceRefs...),
				Basis: draft.Basis, Status: draft.Status,
			})
		}
		return out, nil
	}
	file, err := initcfg.LoadValueDrafts(path, string(kind))
	if err != nil {
		return application.ConfigEnrichDraftFile{}, err
	}
	out := application.ConfigEnrichDraftFile{Version: file.Version, Field: file.Field, Drafts: make([]application.ConfigEnrichDraft, 0, len(file.Drafts))}
	for _, draft := range file.Drafts {
		out.Drafts = append(out.Drafts, application.ConfigEnrichDraft{
			Module: draft.Module, Value: draft.Value, Rationale: draft.Rationale,
			EvidenceRefs: append([]string(nil), draft.EvidenceRefs...), Basis: draft.Basis,
			Status: draft.Status, Confidence: draft.Confidence, Provenance: draft.Provenance,
		})
	}
	return out, nil
}

func (a *configEnrichAdapter) SaveConfigEnrichDrafts(_ context.Context, path string, kind application.ConfigEnrichKind, file application.ConfigEnrichDraftFile) error {
	if kind == application.ConfigEnrichSubdomain {
		out := initcfg.SubdomainDraftFile{Version: file.Version, Drafts: make([]initcfg.SubdomainDraft, 0, len(file.Drafts))}
		for _, draft := range file.Drafts {
			out.Drafts = append(out.Drafts, initcfg.SubdomainDraft{
				Module: draft.Module, Subdomain: draft.Subdomain, Volatility: draft.Volatility,
				Rationale: draft.Rationale, EvidenceRefs: append([]string(nil), draft.EvidenceRefs...),
				Basis: draft.Basis, Status: draft.Status,
			})
		}
		return initcfg.WriteSubdomainDrafts(path, out)
	}
	out := initcfg.ValueDraftFile{Version: file.Version, Field: file.Field, Drafts: make([]initcfg.ValueDraft, 0, len(file.Drafts))}
	for _, draft := range file.Drafts {
		out.Drafts = append(out.Drafts, initcfg.ValueDraft{
			Module: draft.Module, Value: draft.Value, Rationale: draft.Rationale,
			EvidenceRefs: append([]string(nil), draft.EvidenceRefs...), Basis: draft.Basis,
			Status: draft.Status, Confidence: draft.Confidence, Provenance: draft.Provenance,
		})
	}
	return initcfg.WriteValueDrafts(path, out)
}

func (a *configEnrichAdapter) EditConfigEnrich(_ context.Context, req application.ConfigEnrichEditRequest) ([]byte, int, error) {
	if req.Kind == application.ConfigEnrichSubdomain {
		pins := make([]initcfg.SubdomainPin, 0, len(req.Pins))
		for _, pin := range req.Pins {
			pins = append(pins, initcfg.SubdomainPin{
				Module: pin.Module, Subdomain: pin.Subdomain, Volatility: pin.Volatility,
				ReviewedAt: pin.ReviewedAt, ReviewedBy: pin.ReviewedBy,
			})
		}
		return initcfg.PinSubdomains(req.Source, req.Current, pins)
	}
	field := initcfg.FieldOwner
	if req.Kind == application.ConfigEnrichVolatility {
		field = initcfg.FieldVolatility
	}
	pins := make([]initcfg.ValuePin, 0, len(req.Pins))
	for _, pin := range req.Pins {
		pins = append(pins, initcfg.ValuePin{Module: pin.Module, Value: pin.Value, ReviewedAt: pin.ReviewedAt, ReviewedBy: pin.ReviewedBy})
	}
	return initcfg.PinModuleValues(req.Source, field, req.Current, pins)
}

func (a *configEnrichAdapter) ReadConfigEnrichFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path) //#nosec G304
}

func (a *configEnrichAdapter) WriteConfigEnrich(ctx context.Context, path string, edited, original []byte) error {
	return safeWriteConfig(ctx, a.deps, path, edited, original)
}

func (a *configEnrichAdapter) Now() time.Time { return time.Now() }

// valueResponse mirrors one element of the model's JSON answer.
type valueResponse struct {
	Module       string   `json:"module"`
	Value        string   `json:"value"`
	Rationale    string   `json:"rationale"`
	EvidenceRefs []string `json:"evidence_refs"`
	Basis        string   `json:"basis"`
}

// draftModuleValues sends targets to the LLM in batches and returns adapter drafts.
func draftModuleValues(ctx context.Context, p llm.Provider, spec valueSpec, targets []initcfg.ClassifyTarget, codeowners string, repoEvidence []string) ([]initcfg.ValueDraft, error) {
	var out []initcfg.ValueDraft
	allowedRefs := evidenceRefSet(repoEvidence)
	for start := 0; start < len(targets); start += valueBatchSize {
		batch := targets[start:min(start+valueBatchSize, len(targets))]
		resp, err := p.Complete(ctx, llm.Request{System: spec.systemPrompt, User: valueUserPrompt(batch, codeowners, repoEvidence)})
		if err != nil {
			return nil, err
		}
		drafts, err := parseValueResponse(resp.Text, spec, batch, len(repoEvidence) > 0, allowedRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, drafts...)
	}
	return out, nil
}

func valueUserPrompt(batch []initcfg.ClassifyTarget, codeowners string, repoEvidence []string) string {
	var b strings.Builder
	if codeowners != "" {
		b.WriteString("CODEOWNERS:\n")
		b.WriteString(codeowners)
		b.WriteString("\n\n")
	}
	if len(repoEvidence) > 0 {
		b.WriteString(repositoryEvidenceHeader + "\n")
		for _, evidence := range repoEvidence {
			fmt.Fprintf(&b, "- %s\n", evidence)
		}
		b.WriteString("\nEvery proposed value must cite repository evidence IDs in evidence_refs and set basis to deterministic_fact or semantic_judgment.\n\n")
	}
	b.WriteString("Modules:\n")
	for _, target := range batch {
		fmt.Fprintf(&b, "\n- module: %s\n", target.Name)
		if len(target.Paths) > 0 {
			fmt.Fprintf(&b, "  paths: %s\n", strings.Join(target.Paths, ", "))
		}
		if len(target.Files) > 0 {
			fmt.Fprintf(&b, "  files: %s\n", strings.Join(target.Files, ", "))
		}
	}
	return b.String()
}

func parseValueResponse(text string, spec valueSpec, batch []initcfg.ClassifyTarget, requireEvidence bool, allowedRefs ...map[string]struct{}) ([]initcfg.ValueDraft, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")

	var entries []valueResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &entries); err != nil {
		return nil, fmt.Errorf("enrich --%s: model response is not the required JSON array: %w", spec.name, err)
	}
	requested := make(map[string]struct{}, len(batch))
	for _, target := range batch {
		requested[target.Name] = struct{}{}
	}
	out := make([]initcfg.ValueDraft, 0, len(entries))
	for _, entry := range entries {
		if _, ok := requested[entry.Module]; !ok {
			continue
		}
		value := strings.TrimSpace(entry.Value)
		if value == "" {
			continue
		}
		// Order is the contract: malformed JSON is an error, an out-of-enum value
		// is skipped, and only a VALID value missing its rationale is an error.
		if spec.valid != nil && !spec.valid(value) {
			continue
		}
		rationale := strings.TrimSpace(entry.Rationale)
		if rationale == "" {
			return nil, fmt.Errorf("enrich --%s: entry %q missing rationale", spec.name, entry.Module)
		}
		basis, refs, err := draftMetadata("enrich --"+spec.name+" entry", entry.Module, entry.Basis, entry.EvidenceRefs, requireEvidence, firstAllowedEvidenceRefs(allowedRefs))
		if err != nil {
			return nil, err
		}
		out = append(out, initcfg.ValueDraft{
			Module: entry.Module, Value: value, Rationale: rationale, EvidenceRefs: refs,
			Basis: basis, Status: initcfg.DraftStatusDraft, Provenance: "llm",
		})
	}
	return out, nil
}

func readCodeowners(root string) string {
	for _, rel := range []string{"CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"} {
		data, err := os.ReadFile(filepath.Join(root, rel)) //#nosec G304
		if err != nil {
			continue
		}
		const maxCodeowners = 8000
		return cutAtRuneBoundary(string(data), maxCodeowners)
	}
	return ""
}
