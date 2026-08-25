package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ConfigEnrichKind identifies one config metadata workflow.
type ConfigEnrichKind string

const (
	// ConfigEnrichSubdomain selects the module subdomain workflow.
	ConfigEnrichSubdomain ConfigEnrichKind = "subdomain"
	// ConfigEnrichOwner selects the module owner workflow.
	ConfigEnrichOwner ConfigEnrichKind = "owner"
	// ConfigEnrichVolatility selects the module volatility workflow.
	ConfigEnrichVolatility ConfigEnrichKind = "volatility"
)

// ConfigEnrichAction tells the command which user-facing result to render.
type ConfigEnrichAction string

const (
	// ConfigEnrichDraftsWritten means fresh proposals were saved for review.
	ConfigEnrichDraftsWritten ConfigEnrichAction = "drafts_written"
	// ConfigEnrichAllSet means every module already has the target field.
	ConfigEnrichAllSet ConfigEnrichAction = "all_set"
	// ConfigEnrichNoApproved means apply found no human-approved drafts.
	ConfigEnrichNoApproved ConfigEnrichAction = "no_approved"
	// ConfigEnrichNoChanges means approved drafts cannot change live fields.
	ConfigEnrichNoChanges ConfigEnrichAction = "no_changes"
	// ConfigEnrichPinned means approved values were committed to config.
	ConfigEnrichPinned ConfigEnrichAction = "pinned"
)

const (
	// ConfigEnrichDraftStatusDraft marks an unreviewed proposal.
	ConfigEnrichDraftStatusDraft = "draft"
	// ConfigEnrichDraftStatusApproved marks a human-approved proposal.
	ConfigEnrichDraftStatusApproved = "approved"

	configEnrichVolatilityLow    = "low"
	configEnrichVolatilityMedium = "medium"
	configEnrichVolatilityHigh   = "high"
)

// ConfigEnrichModule is the application projection of one configured module.
type ConfigEnrichModule struct {
	Name       string
	Paths      []string
	Subdomain  string
	Volatility string
	Owner      string
}

// ConfigEnrichConfig is the config subset used by metadata enrichment.
type ConfigEnrichConfig struct {
	Modules      map[string]ConfigEnrichModule
	Layers       []string
	AIConfigured bool
}

// ConfigEnrichDraft is the application-owned review-file entry. It is broad
// enough for subdomain and single-value drafts so stores can preserve every
// protected metadata field during a merge.
type ConfigEnrichDraft struct {
	Module       string
	Subdomain    string
	Volatility   string
	Value        string
	Rationale    string
	EvidenceRefs []string
	Basis        string
	Status       string
	Confidence   string
	Provenance   string
}

// ConfigEnrichDraftFile is the application-owned draft-store document.
type ConfigEnrichDraftFile struct {
	Version int
	Field   string
	Drafts  []ConfigEnrichDraft
}

// ConfigEnrichPin is one approved edit stamped with review metadata.
type ConfigEnrichPin struct {
	Module     string
	Subdomain  string
	Volatility string
	Value      string
	ReviewedAt time.Time
	ReviewedBy string
}

// ConfigEnrichRequest selects draft or apply mode for one metadata field.
type ConfigEnrichRequest struct {
	ConfigPath string
	Root       string
	DraftPath  string
	Kind       ConfigEnrichKind
	Apply      bool
	ReviewedBy string
}

// ConfigEnrichResult summarizes the application decision without prescribing output.
type ConfigEnrichResult struct {
	Action     ConfigEnrichAction
	Count      int
	ReviewedBy string
	Missing    []string
}

// ConfigEnrichJudgmentRequest contains only prompt-relevant target data.
type ConfigEnrichJudgmentRequest struct {
	Kind    ConfigEnrichKind
	Root    string
	Modules []ConfigEnrichModule
	Layers  []string
}

// ConfigEnrichEditRequest contains source bytes and already-reviewed pins.
type ConfigEnrichEditRequest struct {
	Kind    ConfigEnrichKind
	Source  []byte
	Current map[string]string
	Pins    []ConfigEnrichPin
}

// ConfigEnrichConfigLoader projects concrete config into application data.
type ConfigEnrichConfigLoader interface {
	LoadConfigEnrich(context.Context, string) (ConfigEnrichConfig, error)
}

// ConfigEnrichJudgmentPort validates provider setup and obtains draft judgments.
type ConfigEnrichJudgmentPort interface {
	ValidateConfigEnrichJudgment(context.Context, ConfigEnrichKind) error
	DraftConfigEnrich(context.Context, ConfigEnrichJudgmentRequest) ([]ConfigEnrichDraft, error)
}

// ConfigEnrichDraftStore persists review files without exposing YAML types.
type ConfigEnrichDraftStore interface {
	LoadConfigEnrichDrafts(context.Context, string, ConfigEnrichKind) (ConfigEnrichDraftFile, error)
	SaveConfigEnrichDrafts(context.Context, string, ConfigEnrichKind, ConfigEnrichDraftFile) error
}

// ConfigEnrichEditor applies reviewed pins while preserving untouched YAML bytes.
type ConfigEnrichEditor interface {
	EditConfigEnrich(context.Context, ConfigEnrichEditRequest) ([]byte, int, error)
}

// ConfigEnrichFileSystem supplies source bytes at the application boundary.
type ConfigEnrichFileSystem interface {
	ReadConfigEnrichFile(context.Context, string) ([]byte, error)
}

// ConfigEnrichWriter commits validated config bytes.
type ConfigEnrichWriter interface {
	WriteConfigEnrich(context.Context, string, []byte, []byte) error
}

// ConfigEnrichClock timestamps approved edits.
type ConfigEnrichClock interface{ Now() time.Time }

// ConfigEnrichService owns target selection, review merge, and apply sequencing.
type ConfigEnrichService struct {
	Configs ConfigEnrichConfigLoader
	Judge   ConfigEnrichJudgmentPort
	Drafts  ConfigEnrichDraftStore
	Editor  ConfigEnrichEditor
	Files   ConfigEnrichFileSystem
	Writer  ConfigEnrichWriter
	Clock   ConfigEnrichClock
}

// Execute runs exactly one config metadata draft or apply workflow.
func (s ConfigEnrichService) Execute(ctx context.Context, req ConfigEnrichRequest) (ConfigEnrichResult, error) {
	if err := validateConfigEnrichRequest(req); err != nil {
		return ConfigEnrichResult{}, err
	}
	if req.Apply {
		return s.apply(ctx, req)
	}
	return s.draft(ctx, req)
}

func validateConfigEnrichRequest(req ConfigEnrichRequest) error {
	if req.ConfigPath == "" {
		return errors.New("config enrich config path is required")
	}
	if req.DraftPath == "" {
		return errors.New("config enrich draft path is required")
	}
	switch req.Kind {
	case ConfigEnrichSubdomain, ConfigEnrichOwner, ConfigEnrichVolatility:
		return nil
	default:
		return fmt.Errorf("unsupported config enrich kind %q", req.Kind)
	}
}

func (s ConfigEnrichService) draft(ctx context.Context, req ConfigEnrichRequest) (ConfigEnrichResult, error) {
	if s.Configs == nil || s.Judge == nil || s.Drafts == nil {
		return ConfigEnrichResult{}, errors.New("config enrich draft workflow is not fully configured")
	}
	cfg, err := s.Configs.LoadConfigEnrich(ctx, req.ConfigPath)
	if err != nil {
		return ConfigEnrichResult{}, err
	}
	if !cfg.AIConfigured {
		return ConfigEnrichResult{}, configEnrichAIError(req.Kind)
	}
	// Validate provider configuration before the no-target decision, preserving
	// the CLI's fail-fast setup semantics without exposing provider types here.
	if err := s.Judge.ValidateConfigEnrichJudgment(ctx, req.Kind); err != nil {
		return ConfigEnrichResult{}, err
	}
	targets := configEnrichTargets(cfg, req.Kind)
	if len(targets) == 0 {
		return ConfigEnrichResult{Action: ConfigEnrichAllSet}, nil
	}
	drafts, err := s.Judge.DraftConfigEnrich(ctx, ConfigEnrichJudgmentRequest{
		Kind: req.Kind, Root: req.Root, Modules: targets, Layers: append([]string(nil), cfg.Layers...),
	})
	if err != nil {
		if req.Kind == ConfigEnrichSubdomain {
			return ConfigEnrichResult{}, fmt.Errorf("classify failed: %w", err)
		}
		return ConfigEnrichResult{}, fmt.Errorf("draft %s failed: %w", req.Kind, err)
	}
	drafts = validConfigEnrichDrafts(targets, drafts, req.Kind)
	missing := missingConfigEnrichTargets(targets, drafts, req.Kind)
	existing, err := s.Drafts.LoadConfigEnrichDrafts(ctx, req.DraftPath, req.Kind)
	if err != nil {
		return ConfigEnrichResult{}, err
	}
	merged := mergeConfigEnrichDrafts(existing, drafts)
	if err := s.Drafts.SaveConfigEnrichDrafts(ctx, req.DraftPath, req.Kind, merged); err != nil {
		return ConfigEnrichResult{}, err
	}
	return ConfigEnrichResult{Action: ConfigEnrichDraftsWritten, Count: len(drafts), Missing: missing}, nil
}

func (s ConfigEnrichService) apply(ctx context.Context, req ConfigEnrichRequest) (ConfigEnrichResult, error) {
	if s.Configs == nil || s.Drafts == nil || s.Editor == nil || s.Files == nil || s.Writer == nil || s.Clock == nil {
		return ConfigEnrichResult{}, errors.New("config enrich apply workflow is not fully configured")
	}
	draftFile, err := s.Drafts.LoadConfigEnrichDrafts(ctx, req.DraftPath, req.Kind)
	if err != nil {
		return ConfigEnrichResult{}, err
	}
	approved := approvedConfigEnrichDrafts(draftFile.Drafts)
	if len(approved) == 0 {
		return ConfigEnrichResult{Action: ConfigEnrichNoApproved}, nil
	}
	source, err := s.Files.ReadConfigEnrichFile(ctx, req.ConfigPath)
	if err != nil {
		return ConfigEnrichResult{}, fmt.Errorf("reading config: %w", err)
	}
	cfg, err := s.Configs.LoadConfigEnrich(ctx, req.ConfigPath)
	if err != nil {
		return ConfigEnrichResult{}, err
	}
	current := configEnrichCurrent(cfg, req.Kind)
	reviewedBy := req.ReviewedBy
	if reviewedBy == "" {
		reviewedBy = "config enrich " + string(req.Kind)
	}
	pins := actionableConfigEnrichPins(approved, req.Kind, current, reviewedBy, s.Clock.Now().UTC())
	if len(pins) == 0 {
		return ConfigEnrichResult{Action: ConfigEnrichNoChanges, ReviewedBy: reviewedBy}, nil
	}
	edited, patched, err := s.Editor.EditConfigEnrich(ctx, ConfigEnrichEditRequest{Kind: req.Kind, Source: source, Current: current, Pins: pins})
	if err != nil {
		scope := string(req.Kind)
		if req.Kind == ConfigEnrichSubdomain {
			scope = "subdomains"
		}
		return ConfigEnrichResult{}, fmt.Errorf("pin %s: %w", scope, err)
	}
	if patched == 0 {
		return ConfigEnrichResult{Action: ConfigEnrichNoChanges, ReviewedBy: reviewedBy}, nil
	}
	if err := s.Writer.WriteConfigEnrich(ctx, req.ConfigPath, edited, source); err != nil {
		return ConfigEnrichResult{}, err
	}
	return ConfigEnrichResult{Action: ConfigEnrichPinned, Count: patched, ReviewedBy: reviewedBy}, nil
}

func configEnrichAIError(kind ConfigEnrichKind) error {
	if kind == ConfigEnrichSubdomain {
		return errors.New("config enrich subdomain needs ai configured (provider + model); see docs/guide/llm-enrich.md")
	}
	return fmt.Errorf("enrich --%s needs ai configured (provider + model); see docs/guide/llm-enrich.md", kind)
}

func configEnrichTargets(cfg ConfigEnrichConfig, kind ConfigEnrichKind) []ConfigEnrichModule {
	out := make([]ConfigEnrichModule, 0, len(cfg.Modules))
	for name, mod := range cfg.Modules {
		mod.Name = name
		if configEnrichModuleValue(mod, kind) == "" {
			mod.Paths = append([]string(nil), mod.Paths...)
			out = append(out, mod)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func configEnrichModuleValue(mod ConfigEnrichModule, kind ConfigEnrichKind) string {
	switch kind {
	case ConfigEnrichSubdomain:
		return mod.Subdomain
	case ConfigEnrichOwner:
		return mod.Owner
	case ConfigEnrichVolatility:
		return mod.Volatility
	default:
		return ""
	}
}

func validConfigEnrichDrafts(targets []ConfigEnrichModule, drafts []ConfigEnrichDraft, kind ConfigEnrichKind) []ConfigEnrichDraft {
	requested := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		requested[target.Name] = struct{}{}
	}
	out := make([]ConfigEnrichDraft, 0, len(drafts))
	for _, draft := range drafts {
		if _, ok := requested[draft.Module]; !ok {
			continue
		}
		switch kind {
		case ConfigEnrichSubdomain:
			if !validConfigEnrichSubdomain(draft.Subdomain) || !validConfigEnrichVolatility(draft.Volatility) {
				continue
			}
		case ConfigEnrichVolatility:
			draft.Value = strings.TrimSpace(draft.Value)
			if !validConfigEnrichVolatility(draft.Value) {
				continue
			}
		case ConfigEnrichOwner:
			draft.Value = strings.TrimSpace(draft.Value)
			if draft.Value == "" {
				continue
			}
		}
		draft.Status = ConfigEnrichDraftStatusDraft
		out = append(out, draft)
	}
	return out
}

func validConfigEnrichSubdomain(value string) bool {
	return value == "core" || value == "supporting" || value == "generic"
}

func validConfigEnrichVolatility(value string) bool {
	return value == configEnrichVolatilityLow || value == configEnrichVolatilityMedium || value == configEnrichVolatilityHigh
}

func missingConfigEnrichTargets(targets []ConfigEnrichModule, drafts []ConfigEnrichDraft, kind ConfigEnrichKind) []string {
	got := make(map[string]struct{}, len(drafts))
	for _, draft := range drafts {
		if kind != ConfigEnrichSubdomain || draft.Subdomain != "" {
			got[draft.Module] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, target := range targets {
		if _, ok := got[target.Name]; !ok {
			missing = append(missing, target.Name)
		}
	}
	sort.Strings(missing)
	return missing
}

func mergeConfigEnrichDrafts(existing ConfigEnrichDraftFile, incoming []ConfigEnrichDraft) ConfigEnrichDraftFile {
	byModule := make(map[string]ConfigEnrichDraft, len(existing.Drafts)+len(incoming))
	for _, draft := range existing.Drafts {
		byModule[draft.Module] = draft
	}
	for _, draft := range incoming {
		if current, ok := byModule[draft.Module]; ok && current.Status == ConfigEnrichDraftStatusApproved {
			continue
		}
		byModule[draft.Module] = draft
	}
	merged := make([]ConfigEnrichDraft, 0, len(byModule))
	for _, draft := range byModule {
		merged = append(merged, draft)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Module < merged[j].Module })
	if existing.Version == 0 {
		existing.Version = 1
	}
	existing.Drafts = merged
	return existing
}

func approvedConfigEnrichDrafts(drafts []ConfigEnrichDraft) []ConfigEnrichDraft {
	approved := make([]ConfigEnrichDraft, 0, len(drafts))
	for _, draft := range drafts {
		if draft.Status == ConfigEnrichDraftStatusApproved {
			approved = append(approved, draft)
		}
	}
	return approved
}

func configEnrichCurrent(cfg ConfigEnrichConfig, kind ConfigEnrichKind) map[string]string {
	current := make(map[string]string, len(cfg.Modules))
	for name, mod := range cfg.Modules {
		current[name] = configEnrichModuleValue(mod, kind)
	}
	return current
}

func actionableConfigEnrichPins(drafts []ConfigEnrichDraft, kind ConfigEnrichKind, current map[string]string, reviewedBy string, reviewedAt time.Time) []ConfigEnrichPin {
	pins := make([]ConfigEnrichPin, 0, len(drafts))
	for _, draft := range drafts {
		if current[draft.Module] != "" {
			continue
		}
		pin := ConfigEnrichPin{Module: draft.Module, ReviewedAt: reviewedAt, ReviewedBy: reviewedBy}
		if kind == ConfigEnrichSubdomain {
			if draft.Subdomain == "" {
				continue
			}
			pin.Subdomain = draft.Subdomain
			pin.Volatility = draft.Volatility
		} else {
			if draft.Value == "" {
				continue
			}
			pin.Value = draft.Value
		}
		pins = append(pins, pin)
	}
	return pins
}
