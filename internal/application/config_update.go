package application

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// ConfigUpdateAction tells the command which output contract to render.
type ConfigUpdateAction string

const (
	// ConfigUpdateReviewReady means a preview document was rendered.
	ConfigUpdateReviewReady ConfigUpdateAction = "review_ready"
	// ConfigUpdateNoChanges means apply found no edits or review items.
	ConfigUpdateNoChanges ConfigUpdateAction = "no_changes"
	// ConfigUpdateReviewOnly means apply found disclosure-only review items.
	ConfigUpdateReviewOnly ConfigUpdateAction = "review_only"
	// ConfigUpdateApplied means structural or settings edits were committed.
	ConfigUpdateApplied ConfigUpdateAction = "applied"
)

// ConfigUpdateModule is the application projection used for target selection.
type ConfigUpdateModule struct {
	Name   string
	Paths  []string
	Public []string
}

// ConfigUpdateConfig is the config subset needed by the update use case.
type ConfigUpdateConfig struct {
	Modules map[string]ConfigUpdateModule
	Layers  []string
}

// ConfigUpdatePlan is the application-owned summary of the adapter's current
// projected review. The concrete review document remains behind the renderer port.
type ConfigUpdatePlan struct {
	Added               []ConfigUpdateModule
	Suggested           []ConfigUpdateModule
	Unclassified        []string
	PendingModuleEdits  bool
	PendingSettingEdits bool
	ReviewItems         bool
	PathDrift           bool
}

// ConfigUpdateClassification records only completeness data needed by the use case.
type ConfigUpdateClassification struct {
	Module     string
	Subdomain  string
	Volatility string
}

// ConfigUpdateRequest selects review, JSON review, AI judgment, or apply mode.
type ConfigUpdateRequest struct {
	ConfigPath string
	Root       string
	AIClassify bool
	Apply      bool
	JSON       bool
	Refresh    bool
}

// ConfigUpdateDiscoveryRequest supplies discovery options without concrete config types.
type ConfigUpdateDiscoveryRequest struct {
	Root       string
	AIClassify bool
	Refresh    bool
}

// ConfigUpdateClassificationRequest contains application-selected targets.
type ConfigUpdateClassificationRequest struct {
	Root    string
	Modules []ConfigUpdateModule
	Layers  []string
}

// ConfigUpdateResult reports the decision and adapter-rendered review bytes.
type ConfigUpdateResult struct {
	Action                 ConfigUpdateAction
	Output                 []byte
	MissingClassifications []string
	PathDrift              bool
}

// InvalidConfigUpdateRequestError is mapped to the CLI usage exit code.
type InvalidConfigUpdateRequestError struct{ Message string }

func (e *InvalidConfigUpdateRequestError) Error() string { return e.Message }

// ConfigUpdateConfigLoader loads and projects the current config.
type ConfigUpdateConfigLoader interface {
	LoadConfigUpdate(context.Context, string) (ConfigUpdateConfig, error)
}

// ConfigUpdateFileSystem supplies the source bytes preserved by the editor.
type ConfigUpdateFileSystem interface {
	ReadConfigUpdateFile(context.Context, string) ([]byte, error)
}

// ConfigUpdateDiscovery performs concrete repository discovery and suggestion collection.
type ConfigUpdateDiscovery interface {
	DiscoverConfigUpdate(context.Context, ConfigUpdateDiscoveryRequest) error
}

// ConfigUpdateProjector exposes the current review as application decisions.
type ConfigUpdateProjector interface {
	ProjectConfigUpdate(context.Context) (ConfigUpdatePlan, error)
}

// ConfigUpdateClassifier validates provider setup and judges selected modules.
type ConfigUpdateClassifier interface {
	ValidateConfigUpdateClassifier(context.Context) error
	ClassifyConfigUpdate(context.Context, ConfigUpdateClassificationRequest) ([]ConfigUpdateClassification, error)
}

// ConfigUpdateReviewer renders the concrete review at the command adapter edge.
type ConfigUpdateReviewer interface {
	RenderConfigUpdateReview(context.Context, bool) ([]byte, error)
	RenderAppliedConfigUpdateReview(context.Context) ([]byte, error)
}

// ConfigUpdateEditor applies the current projected structural/settings edits.
type ConfigUpdateEditor interface {
	EditConfigUpdate(context.Context, []byte) ([]byte, error)
}

// ConfigUpdateWriter commits validated config bytes.
type ConfigUpdateWriter interface {
	WriteConfigUpdate(context.Context, string, []byte, []byte) error
}

// ConfigUpdateService owns validation, target selection, and review/apply sequencing.
type ConfigUpdateService struct {
	Configs    ConfigUpdateConfigLoader
	Files      ConfigUpdateFileSystem
	Discovery  ConfigUpdateDiscovery
	Projection ConfigUpdateProjector
	Classifier ConfigUpdateClassifier
	Reviewer   ConfigUpdateReviewer
	Editor     ConfigUpdateEditor
	Writer     ConfigUpdateWriter
}

// Execute runs the config update use case.
func (s ConfigUpdateService) Execute(ctx context.Context, req ConfigUpdateRequest) (ConfigUpdateResult, error) {
	if err := validateConfigUpdateRequest(req); err != nil {
		return ConfigUpdateResult{}, err
	}
	if s.Configs == nil || s.Files == nil || s.Discovery == nil || s.Projection == nil || s.Reviewer == nil {
		return ConfigUpdateResult{}, errors.New("config update workflow is not fully configured")
	}
	cfg, err := s.Configs.LoadConfigUpdate(ctx, req.ConfigPath)
	if err != nil {
		return ConfigUpdateResult{}, err
	}
	original, err := s.Files.ReadConfigUpdateFile(ctx, req.ConfigPath)
	if err != nil {
		return ConfigUpdateResult{}, err
	}
	if err := s.Discovery.DiscoverConfigUpdate(ctx, ConfigUpdateDiscoveryRequest{Root: req.Root, AIClassify: req.AIClassify, Refresh: req.Refresh}); err != nil {
		return ConfigUpdateResult{}, err
	}
	plan, err := s.Projection.ProjectConfigUpdate(ctx)
	if err != nil {
		return ConfigUpdateResult{}, err
	}

	classificationAvailable := false
	var missing []string
	if req.AIClassify {
		if s.Classifier == nil {
			return ConfigUpdateResult{}, errors.New("config update classifier is not configured")
		}
		// Preserve provider validation before the empty-target decision.
		if err := s.Classifier.ValidateConfigUpdateClassifier(ctx); err != nil {
			return ConfigUpdateResult{}, err
		}
		targets := selectConfigUpdateTargets(cfg, plan)
		if len(targets) > 0 {
			classified, err := s.Classifier.ClassifyConfigUpdate(ctx, ConfigUpdateClassificationRequest{
				Root: req.Root, Modules: targets, Layers: append([]string(nil), cfg.Layers...),
			})
			if err != nil {
				return ConfigUpdateResult{}, err
			}
			classificationAvailable = classified != nil
			missing = missingConfigUpdateClassifications(targets, classified)
			plan, err = s.Projection.ProjectConfigUpdate(ctx)
			if err != nil {
				return ConfigUpdateResult{}, err
			}
		}
	}

	result := ConfigUpdateResult{MissingClassifications: missing, PathDrift: plan.PathDrift}
	if !req.Apply {
		result.Action = ConfigUpdateReviewReady
		result.Output, err = s.Reviewer.RenderConfigUpdateReview(ctx, req.JSON)
		return result, err
	}

	hasEdits := plan.PendingModuleEdits || plan.PendingSettingEdits
	if !hasEdits {
		if classificationAvailable || plan.ReviewItems {
			result.Action = ConfigUpdateReviewOnly
			result.Output, err = s.Reviewer.RenderConfigUpdateReview(ctx, false)
			return result, err
		}
		result.Action = ConfigUpdateNoChanges
		return result, nil
	}
	if s.Editor == nil || s.Writer == nil {
		return ConfigUpdateResult{}, errors.New("config update apply workflow is not fully configured")
	}
	edited, err := s.Editor.EditConfigUpdate(ctx, original)
	if err != nil {
		return ConfigUpdateResult{}, err
	}
	if err := s.Writer.WriteConfigUpdate(ctx, req.ConfigPath, edited, original); err != nil {
		return ConfigUpdateResult{}, err
	}
	result.Action = ConfigUpdateApplied
	if classificationAvailable || plan.ReviewItems {
		result.Output, err = s.Reviewer.RenderAppliedConfigUpdateReview(ctx)
	}
	return result, err
}

func validateConfigUpdateRequest(req ConfigUpdateRequest) error {
	if req.JSON {
		conflicts := make([]string, 0, 3)
		if req.Apply {
			conflicts = append(conflicts, "--apply")
		}
		if req.AIClassify {
			conflicts = append(conflicts, "--ai-classify")
		}
		if req.Refresh {
			conflicts = append(conflicts, "--refresh")
		}
		if len(conflicts) > 0 {
			return &InvalidConfigUpdateRequestError{Message: "error: --json cannot be combined with " + strings.Join(conflicts, ", ")}
		}
	}
	if req.ConfigPath == "" {
		return &InvalidConfigUpdateRequestError{Message: "error: config path is required"}
	}
	if req.Root == "" {
		return &InvalidConfigUpdateRequestError{Message: "error: project root is required"}
	}
	return nil
}

func selectConfigUpdateTargets(cfg ConfigUpdateConfig, plan ConfigUpdatePlan) []ConfigUpdateModule {
	targets := make([]ConfigUpdateModule, 0, len(plan.Added)+len(plan.Suggested)+len(plan.Unclassified))
	added := make(map[string]struct{}, len(plan.Added))
	for _, mod := range plan.Added {
		added[mod.Name] = struct{}{}
		targets = append(targets, cloneConfigUpdateModule(mod))
	}
	for _, mod := range plan.Suggested {
		targets = append(targets, cloneConfigUpdateModule(mod))
	}
	for _, name := range plan.Unclassified {
		if _, isAdded := added[name]; isAdded {
			continue
		}
		if mod, ok := cfg.Modules[name]; ok {
			mod.Name = name
			targets = append(targets, cloneConfigUpdateModule(mod))
		}
	}
	return targets
}

func cloneConfigUpdateModule(mod ConfigUpdateModule) ConfigUpdateModule {
	mod.Paths = append([]string(nil), mod.Paths...)
	mod.Public = append([]string(nil), mod.Public...)
	return mod
}

func missingConfigUpdateClassifications(targets []ConfigUpdateModule, classified []ConfigUpdateClassification) []string {
	complete := make(map[string]struct{}, len(classified))
	for _, item := range classified {
		if item.Subdomain != "" && item.Volatility != "" {
			complete[item.Module] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, target := range targets {
		if _, ok := complete[target.Name]; !ok {
			missing = append(missing, target.Name)
		}
	}
	sort.Strings(missing)
	return missing
}
