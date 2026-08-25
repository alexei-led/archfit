package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

const (
	configUpdateTestRoot     = "root"
	configUpdateTestNew      = "new"
	configUpdateTestOld      = "old"
	configWorkflowDiscover   = "discover"
	configWorkflowProject    = "project"
	configUpdateTestOriginal = "original"
	configUpdateTestEdited   = "edited"
)

type configUpdateFake struct {
	order       []string
	config      ConfigUpdateConfig
	plans       []ConfigUpdatePlan
	projectCall int
	classified  []ConfigUpdateClassification
	original    []byte
	edited      []byte
	written     []byte
	review      []byte
	applied     []byte
}

func (f *configUpdateFake) LoadConfigUpdate(context.Context, string) (ConfigUpdateConfig, error) {
	f.order = append(f.order, workflowConfig)
	return f.config, nil
}
func (f *configUpdateFake) ReadConfigUpdateFile(context.Context, string) ([]byte, error) {
	f.order = append(f.order, configWorkflowRead)
	return f.original, nil
}
func (f *configUpdateFake) DiscoverConfigUpdate(context.Context, ConfigUpdateDiscoveryRequest) error {
	f.order = append(f.order, configWorkflowDiscover)
	return nil
}
func (f *configUpdateFake) ProjectConfigUpdate(context.Context) (ConfigUpdatePlan, error) {
	f.order = append(f.order, configWorkflowProject)
	idx := f.projectCall
	f.projectCall++
	if idx >= len(f.plans) {
		idx = len(f.plans) - 1
	}
	return f.plans[idx], nil
}
func (f *configUpdateFake) ValidateConfigUpdateClassifier(context.Context) error {
	f.order = append(f.order, "validate-classifier")
	return nil
}
func (f *configUpdateFake) ClassifyConfigUpdate(_ context.Context, req ConfigUpdateClassificationRequest) ([]ConfigUpdateClassification, error) {
	f.order = append(f.order, "classify")
	if len(req.Modules) != 2 || req.Modules[0].Name != configUpdateTestNew || req.Modules[1].Name != configUpdateTestOld ||
		!reflect.DeepEqual(req.Modules[0].Paths, []string{"new/**"}) || !reflect.DeepEqual(req.Modules[1].Paths, []string{"old/**"}) {
		return nil, errors.New("application selected wrong targets")
	}
	return f.classified, nil
}
func (f *configUpdateFake) RenderConfigUpdateReview(_ context.Context, json bool) ([]byte, error) {
	if json {
		f.order = append(f.order, "render-json")
	} else {
		f.order = append(f.order, "render-review")
	}
	return f.review, nil
}
func (f *configUpdateFake) RenderAppliedConfigUpdateReview(context.Context) ([]byte, error) {
	f.order = append(f.order, "render-applied")
	return f.applied, nil
}
func (f *configUpdateFake) EditConfigUpdate(context.Context, []byte) ([]byte, error) {
	f.order = append(f.order, "edit")
	return f.edited, nil
}
func (f *configUpdateFake) WriteConfigUpdate(_ context.Context, _ string, edited, _ []byte) error {
	f.order = append(f.order, "write")
	f.written = append([]byte(nil), edited...)
	return nil
}

func configUpdateServiceWith(f *configUpdateFake) ConfigUpdateService {
	return ConfigUpdateService{Configs: f, Files: f, Discovery: f, Projection: f, Classifier: f, Reviewer: f, Editor: f, Writer: f}
}

func TestConfigUpdateRejectsJSONSideEffectsBeforePorts(t *testing.T) {
	for _, req := range []ConfigUpdateRequest{
		{ConfigPath: workflowConfig, Root: configUpdateTestRoot, JSON: true, Apply: true},
		{ConfigPath: workflowConfig, Root: configUpdateTestRoot, JSON: true, AIClassify: true},
		{ConfigPath: workflowConfig, Root: configUpdateTestRoot, JSON: true, Refresh: true},
	} {
		f := &configUpdateFake{}
		_, err := configUpdateServiceWith(f).Execute(context.Background(), req)
		var invalid *InvalidConfigUpdateRequestError
		if !errors.As(err, &invalid) {
			t.Fatalf("err = %v", err)
		}
		if len(f.order) != 0 {
			t.Fatalf("ports called for invalid request: %v", f.order)
		}
	}
}

func TestConfigUpdateAISelectsTargetsBeforeReview(t *testing.T) {
	f := &configUpdateFake{
		config: ConfigUpdateConfig{Modules: map[string]ConfigUpdateModule{
			configUpdateTestOld: {Paths: []string{"old/**"}},
		}},
		plans: []ConfigUpdatePlan{
			{Added: []ConfigUpdateModule{{Name: configUpdateTestNew, Paths: []string{"new/**"}}}, Unclassified: []string{configUpdateTestNew, configUpdateTestOld}},
			{Added: []ConfigUpdateModule{{Name: configUpdateTestNew}}, Unclassified: []string{configUpdateTestNew, configUpdateTestOld}, ReviewItems: true},
		},
		classified: []ConfigUpdateClassification{{Module: configUpdateTestNew, Subdomain: configWorkflowCore, Volatility: configEnrichVolatilityLow}},
		review:     []byte("review"), original: []byte(configUpdateTestOriginal),
	}
	out, err := configUpdateServiceWith(f).Execute(context.Background(), ConfigUpdateRequest{ConfigPath: workflowConfig, Root: configUpdateTestRoot, AIClassify: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Action != ConfigUpdateReviewReady || string(out.Output) != "review" || !reflect.DeepEqual(out.MissingClassifications, []string{configUpdateTestOld}) {
		t.Fatalf("out = %+v", out)
	}
	want := []string{workflowConfig, configWorkflowRead, configWorkflowDiscover, configWorkflowProject, "validate-classifier", "classify", configWorkflowProject, "render-review"}
	if !reflect.DeepEqual(f.order, want) {
		t.Fatalf("order = %v, want %v", f.order, want)
	}
}

func TestConfigUpdateApplyWritesThenRendersDisclosure(t *testing.T) {
	f := &configUpdateFake{
		config:   ConfigUpdateConfig{Modules: map[string]ConfigUpdateModule{}},
		plans:    []ConfigUpdatePlan{{PendingModuleEdits: true, ReviewItems: true, PathDrift: true}},
		original: []byte(configUpdateTestOriginal), edited: []byte(configUpdateTestEdited), applied: []byte("review-only disclosure"),
	}
	out, err := configUpdateServiceWith(f).Execute(context.Background(), ConfigUpdateRequest{ConfigPath: workflowConfig, Root: configUpdateTestRoot, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Action != ConfigUpdateApplied || !out.PathDrift || string(out.Output) != "review-only disclosure" || string(f.written) != configUpdateTestEdited {
		t.Fatalf("out=%+v written=%q", out, f.written)
	}
	want := []string{workflowConfig, configWorkflowRead, configWorkflowDiscover, configWorkflowProject, "edit", "write", "render-applied"}
	if !reflect.DeepEqual(f.order, want) {
		t.Fatalf("order = %v, want %v", f.order, want)
	}
}

func TestConfigUpdateApplyReviewOnlyNeverEdits(t *testing.T) {
	f := &configUpdateFake{
		config: ConfigUpdateConfig{}, plans: []ConfigUpdatePlan{{ReviewItems: true}},
		original: []byte(configUpdateTestOriginal), review: []byte("status and review"),
	}
	out, err := configUpdateServiceWith(f).Execute(context.Background(), ConfigUpdateRequest{ConfigPath: workflowConfig, Root: configUpdateTestRoot, Apply: true})
	if err != nil || out.Action != ConfigUpdateReviewOnly || string(out.Output) != "status and review" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	want := []string{workflowConfig, configWorkflowRead, configWorkflowDiscover, configWorkflowProject, "render-review"}
	if !reflect.DeepEqual(f.order, want) {
		t.Fatalf("order = %v, want %v", f.order, want)
	}
}

func TestConfigUpdateApplyNoopHasNoRenderOrWrite(t *testing.T) {
	f := &configUpdateFake{config: ConfigUpdateConfig{}, plans: []ConfigUpdatePlan{{}}, original: []byte(configUpdateTestOriginal)}
	out, err := configUpdateServiceWith(f).Execute(context.Background(), ConfigUpdateRequest{ConfigPath: workflowConfig, Root: configUpdateTestRoot, Apply: true})
	if err != nil || out.Action != ConfigUpdateNoChanges {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	want := []string{workflowConfig, configWorkflowRead, configWorkflowDiscover, configWorkflowProject}
	if !reflect.DeepEqual(f.order, want) {
		t.Fatalf("order = %v, want %v", f.order, want)
	}
}
