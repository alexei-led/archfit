package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

const (
	configWorkflowRead       = "read"
	configWorkflowLoadDrafts = "load-drafts"
	configWorkflowCore       = "core"
)

type configEnrichFake struct {
	order       []string
	config      ConfigEnrichConfig
	draftFile   ConfigEnrichDraftFile
	judgments   []ConfigEnrichDraft
	saved       ConfigEnrichDraftFile
	source      []byte
	edited      []byte
	pins        []ConfigEnrichPin
	patched     int
	written     []byte
	validateErr error
}

func (f *configEnrichFake) LoadConfigEnrich(context.Context, string) (ConfigEnrichConfig, error) {
	f.order = append(f.order, workflowConfig)
	return f.config, nil
}
func (f *configEnrichFake) ValidateConfigEnrichJudgment(context.Context, ConfigEnrichKind) error {
	f.order = append(f.order, "validate-judge")
	return f.validateErr
}
func (f *configEnrichFake) DraftConfigEnrich(_ context.Context, req ConfigEnrichJudgmentRequest) ([]ConfigEnrichDraft, error) {
	f.order = append(f.order, "judge")
	if len(req.Modules) > 1 && req.Modules[0].Name > req.Modules[1].Name {
		return nil, errors.New("targets not sorted")
	}
	return f.judgments, nil
}
func (f *configEnrichFake) LoadConfigEnrichDrafts(context.Context, string, ConfigEnrichKind) (ConfigEnrichDraftFile, error) {
	f.order = append(f.order, configWorkflowLoadDrafts)
	return f.draftFile, nil
}
func (f *configEnrichFake) SaveConfigEnrichDrafts(_ context.Context, _ string, _ ConfigEnrichKind, in ConfigEnrichDraftFile) error {
	f.order = append(f.order, "save-drafts")
	f.saved = in
	return nil
}
func (f *configEnrichFake) EditConfigEnrich(_ context.Context, req ConfigEnrichEditRequest) ([]byte, int, error) {
	f.order = append(f.order, "edit")
	f.pins = append([]ConfigEnrichPin(nil), req.Pins...)
	return f.edited, f.patched, nil
}
func (f *configEnrichFake) ReadConfigEnrichFile(context.Context, string) ([]byte, error) {
	f.order = append(f.order, configWorkflowRead)
	return f.source, nil
}
func (f *configEnrichFake) WriteConfigEnrich(_ context.Context, _ string, edited, _ []byte) error {
	f.order = append(f.order, "write")
	f.written = append([]byte(nil), edited...)
	return nil
}
func (f *configEnrichFake) Now() time.Time {
	f.order = append(f.order, "clock")
	return time.Date(2026, 8, 25, 12, 0, 0, 0, time.FixedZone("offset", 3600))
}

func configEnrichServiceWith(f *configEnrichFake) ConfigEnrichService {
	return ConfigEnrichService{Configs: f, Judge: f, Drafts: f, Editor: f, Files: f, Writer: f, Clock: f}
}

func TestConfigEnrichDraftSelectsTargetsAndPreservesApprovedMetadata(t *testing.T) {
	f := &configEnrichFake{
		config: ConfigEnrichConfig{AIConfigured: true, Modules: map[string]ConfigEnrichModule{
			"z": {Owner: "team-z"}, "b": {}, "a": {},
		}},
		draftFile: ConfigEnrichDraftFile{Version: 1, Field: "owner", Drafts: []ConfigEnrichDraft{
			{Module: "a", Value: "human-owner", Status: ConfigEnrichDraftStatusApproved, Confidence: EnrichmentLabelConfidenceHigh, Provenance: EnrichmentLabelProvenanceHuman, EvidenceRefs: []string{"doc:approved"}},
		}},
		judgments: []ConfigEnrichDraft{
			{Module: "a", Value: "replacement", Status: ConfigEnrichDraftStatusDraft},
			{Module: "b", Value: "team-b", Status: ConfigEnrichDraftStatusDraft, Confidence: "medium", Provenance: "llm"},
		},
	}
	out, err := configEnrichServiceWith(f).Execute(context.Background(), ConfigEnrichRequest{
		ConfigPath: workflowConfig, DraftPath: "owners", Kind: ConfigEnrichOwner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Action != ConfigEnrichDraftsWritten || out.Count != 2 {
		t.Fatalf("out = %+v", out)
	}
	if !reflect.DeepEqual(f.order, []string{workflowConfig, "validate-judge", "judge", configWorkflowLoadDrafts, "save-drafts"}) {
		t.Fatalf("order = %v", f.order)
	}
	if len(f.saved.Drafts) != 2 || f.saved.Drafts[0].Value != "human-owner" || f.saved.Drafts[0].Confidence != EnrichmentLabelConfidenceHigh || f.saved.Drafts[0].EvidenceRefs[0] != "doc:approved" {
		t.Fatalf("approved draft metadata changed: %+v", f.saved.Drafts)
	}
}

func TestConfigEnrichDraftValidatesVolatilityJudgments(t *testing.T) {
	f := &configEnrichFake{
		config: ConfigEnrichConfig{AIConfigured: true, Modules: map[string]ConfigEnrichModule{"a": {}}},
		judgments: []ConfigEnrichDraft{
			{Module: "a", Value: "huge", Status: ConfigEnrichDraftStatusApproved},
			{Module: "outside", Value: configEnrichVolatilityLow, Status: ConfigEnrichDraftStatusApproved},
		},
		draftFile: ConfigEnrichDraftFile{Version: 1, Field: "volatility"},
	}
	out, err := configEnrichServiceWith(f).Execute(context.Background(), ConfigEnrichRequest{
		ConfigPath: workflowConfig, DraftPath: "volatility", Kind: ConfigEnrichVolatility,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 0 || !reflect.DeepEqual(out.Missing, []string{"a"}) || len(f.saved.Drafts) != 0 {
		t.Fatalf("out=%+v saved=%+v", out, f.saved)
	}
}

func TestConfigEnrichDraftNoTargetsStillValidatesProvider(t *testing.T) {
	f := &configEnrichFake{config: ConfigEnrichConfig{AIConfigured: true, Modules: map[string]ConfigEnrichModule{"a": {Subdomain: configWorkflowCore}}}}
	out, err := configEnrichServiceWith(f).Execute(context.Background(), ConfigEnrichRequest{ConfigPath: workflowConfig, DraftPath: "drafts", Kind: ConfigEnrichSubdomain})
	if err != nil || out.Action != ConfigEnrichAllSet {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if !reflect.DeepEqual(f.order, []string{workflowConfig, "validate-judge"}) {
		t.Fatalf("order = %v", f.order)
	}
}

func TestConfigEnrichApplyNoApprovedStopsBeforeConfigRead(t *testing.T) {
	f := &configEnrichFake{draftFile: ConfigEnrichDraftFile{Version: 1}}
	out, err := configEnrichServiceWith(f).Execute(context.Background(), ConfigEnrichRequest{ConfigPath: workflowConfig, DraftPath: "drafts", Kind: ConfigEnrichOwner, Apply: true})
	if err != nil || out.Action != ConfigEnrichNoApproved {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if !reflect.DeepEqual(f.order, []string{configWorkflowLoadDrafts}) {
		t.Fatalf("side effects before no-approved decision: %v", f.order)
	}
}

func TestConfigEnrichApplyPinsOnlyUnsetValuesWithReviewedDefaults(t *testing.T) {
	f := &configEnrichFake{
		draftFile: ConfigEnrichDraftFile{Version: 1, Drafts: []ConfigEnrichDraft{
			{Module: "a", Value: "team-a", Status: ConfigEnrichDraftStatusApproved},
			{Module: "b", Value: "team-b", Status: ConfigEnrichDraftStatusApproved},
		}},
		config: ConfigEnrichConfig{Modules: map[string]ConfigEnrichModule{"a": {}, "b": {Owner: "human"}}},
		source: []byte("original"), edited: []byte("edited"), patched: 1,
	}
	out, err := configEnrichServiceWith(f).Execute(context.Background(), ConfigEnrichRequest{ConfigPath: workflowConfig, DraftPath: "owners", Kind: ConfigEnrichOwner, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Action != ConfigEnrichPinned || out.Count != 1 || out.ReviewedBy != "config enrich owner" {
		t.Fatalf("out = %+v", out)
	}
	if !reflect.DeepEqual(f.order, []string{configWorkflowLoadDrafts, configWorkflowRead, workflowConfig, "clock", "edit", "write"}) {
		t.Fatalf("order = %v", f.order)
	}
	if len(f.pins) != 1 || f.pins[0].Module != "a" || f.pins[0].ReviewedBy != "config enrich owner" || f.pins[0].ReviewedAt.Location() != time.UTC {
		t.Fatalf("pins = %+v", f.pins)
	}
	if string(f.written) != "edited" {
		t.Fatalf("written = %q", f.written)
	}
}

func TestConfigEnrichApplyApprovedButAlreadySetIsNoop(t *testing.T) {
	f := &configEnrichFake{
		draftFile: ConfigEnrichDraftFile{Drafts: []ConfigEnrichDraft{{Module: "a", Subdomain: configWorkflowCore, Status: ConfigEnrichDraftStatusApproved}}},
		config:    ConfigEnrichConfig{Modules: map[string]ConfigEnrichModule{"a": {Subdomain: "supporting"}}},
		source:    []byte("same"),
	}
	out, err := configEnrichServiceWith(f).Execute(context.Background(), ConfigEnrichRequest{ConfigPath: workflowConfig, DraftPath: "subdomains", Kind: ConfigEnrichSubdomain, Apply: true, ReviewedBy: "alice"})
	if err != nil || out.Action != ConfigEnrichNoChanges {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if !reflect.DeepEqual(f.order, []string{configWorkflowLoadDrafts, configWorkflowRead, workflowConfig, "clock"}) {
		t.Fatalf("order = %v", f.order)
	}
}
