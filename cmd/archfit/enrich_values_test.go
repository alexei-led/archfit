package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/initcfg"
)

// ownerField is the YAML field / draft-file kind identifier used by the owner drafter.
const ownerField = "owner"

// valueJSONFor builds a scripted provider response assigning each module the
// given value in the cited shape the value drafters expect.
func valueJSONFor(pairs map[string]string) string {
	parts := make([]string, 0, len(pairs))
	for mod, val := range pairs {
		parts = append(parts, fmt.Sprintf(`{"module":%q,"value":%q,"rationale":"test cites config:.archfit.yaml","evidence_refs":["config:.archfit.yaml"],"basis":"semantic_judgment"}`, mod, val))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestEnrichOwnerDraft(t *testing.T) {
	t.Parallel()
	provider := &scriptedProvider{
		responses: []string{valueJSONFor(map[string]string{enrichModAuth: "@team-auth", enrichModNotify: "@team-notify"})},
	}
	cfgPath, dir := writeEnrichSubdomainFixture(t)
	ownersPath := filepath.Join(dir, defaultOwnersPath)

	before, err := os.ReadFile(cfgPath) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}

	cmd := &EnrichOwnerCmd{enrichFlags: enrichFlags{Config: cfgPath, providerOverride: provider}}
	var buf bytes.Buffer
	if err := cmd.Run(&appDeps{Stdout: &buf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "draft owner") {
		t.Errorf("expected draft owner message, got: %s", buf.String())
	}

	df, err := initcfg.LoadValueDrafts(ownersPath, ownerField)
	if err != nil {
		t.Fatalf("load owner drafts: %v", err)
	}
	if len(df.Drafts) != 2 {
		t.Fatalf("want 2 owner drafts, got %d: %+v", len(df.Drafts), df.Drafts)
	}
	for _, d := range df.Drafts {
		if d.Status != initcfg.DraftStatusDraft || d.Value == "" {
			t.Errorf("bad draft %+v", d)
		}
		if len(d.EvidenceRefs) == 0 || d.Basis == "" {
			t.Errorf("draft missing evidence metadata %+v", d)
		}
	}
	after, err := os.ReadFile(cfgPath) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("owner draft mode must leave .archfit.yaml byte-unchanged")
	}
}

func TestValueUserPrompt_IncludesRepositoryEvidenceIDs(t *testing.T) {
	t.Parallel()
	prompt := valueUserPrompt(
		[]initcfg.ClassifyTarget{{Name: enrichModAuth, Paths: []string{"internal/auth/**"}}},
		"",
		[]string{"doc:README.md (doc) README.md: Auth boundary"},
	)
	for _, want := range []string{repositoryEvidenceHeader, "doc:README.md", "Auth boundary", "module: auth"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestEnrichOwnerPin(t *testing.T) {
	t.Parallel()
	cfgPath, dir := writeEnrichSubdomainFixture(t)
	ownersPath := filepath.Join(dir, defaultOwnersPath)

	df := initcfg.ValueDraftFile{Version: 1, Field: ownerField, Drafts: []initcfg.ValueDraft{
		{Module: enrichModAuth, Value: "@team-auth", Status: initcfg.DraftStatusApproved},
		{Module: enrichModNotify, Value: "@team-notify", Status: initcfg.DraftStatusDraft}, // not approved → skipped
	}}
	if err := initcfg.WriteValueDrafts(ownersPath, df); err != nil {
		t.Fatal(err)
	}

	cmd := &EnrichOwnerCmd{enrichFlags: enrichFlags{Config: cfgPath}, Apply: true, ReviewedBy: "rev"}
	var buf bytes.Buffer
	if err := cmd.Run(&appDeps{Stdout: &buf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "pinned") {
		t.Errorf("expected pinned message, got: %s", buf.String())
	}

	got, err := os.ReadFile(cfgPath) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, "owner:") || !strings.Contains(s, "@team-auth") {
		t.Errorf("auth owner not pinned:\n%s", s)
	}
	if !strings.Contains(s, "reviewed_by: rev") {
		t.Errorf("reviewed_by not written:\n%s", s)
	}
	if strings.Contains(s, "@team-notify") {
		t.Errorf("unapproved notify owner was pinned:\n%s", s)
	}
}

func TestEnrichVolatilityDraftAndPin(t *testing.T) {
	t.Parallel()
	provider := &scriptedProvider{
		responses: []string{valueJSONFor(map[string]string{enrichModAuth: volatilityLow, enrichModNotify: volatilityMedium})},
	}
	cfgPath, dir := writeEnrichSubdomainFixture(t)
	volPath := filepath.Join(dir, defaultVolatilityPath)

	// Draft.
	draftCmd := &EnrichVolatilityCmd{enrichFlags: enrichFlags{Config: cfgPath, providerOverride: provider}}
	var buf bytes.Buffer
	if err := draftCmd.Run(&appDeps{Stdout: &buf}); err != nil {
		t.Fatalf("draft Run: %v", err)
	}
	df, err := initcfg.LoadValueDrafts(volPath, "volatility")
	if err != nil {
		t.Fatalf("load volatility drafts: %v", err)
	}
	if len(df.Drafts) != 2 {
		t.Fatalf("want 2 volatility drafts, got %d", len(df.Drafts))
	}

	// Approve all and pin.
	for i := range df.Drafts {
		df.Drafts[i].Status = initcfg.DraftStatusApproved
	}
	if err := initcfg.WriteValueDrafts(volPath, df); err != nil {
		t.Fatal(err)
	}
	pinCmd := &EnrichVolatilityCmd{enrichFlags: enrichFlags{Config: cfgPath}, Apply: true, ReviewedBy: "rev"}
	buf.Reset()
	if err := pinCmd.Run(&appDeps{Stdout: &buf}); err != nil {
		t.Fatalf("pin Run: %v", err)
	}
	got, err := os.ReadFile(cfgPath) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "volatility: "+volatilityLow) {
		t.Errorf("auth volatility not pinned:\n%s", string(got))
	}
}

func TestDraftModuleValues_MissingEvidenceRefsError(t *testing.T) {
	t.Parallel()
	provider := &scriptedProvider{
		responses: []string{`[{"module":"auth","value":"@team-auth","rationale":"test","basis":"semantic_judgment"}]`},
	}
	_, err := draftModuleValues(context.Background(), provider, ownerSpec, []initcfg.ClassifyTarget{{Name: enrichModAuth}}, "", []string{"doc:README.md (doc) README.md: Auth"})
	if err == nil {
		t.Fatal("missing evidence_refs must fail")
	}
	if !strings.Contains(err.Error(), "missing evidence_refs") {
		t.Fatalf("error = %v, want missing evidence_refs", err)
	}
}

func TestDraftModuleValues_UnsupportedEvidenceRefsError(t *testing.T) {
	t.Parallel()
	provider := &scriptedProvider{
		responses: []string{`[{"module":"auth","value":"@team-auth","rationale":"test","evidence_refs":["doc:missing.md"],"basis":"semantic_judgment"}]`},
	}
	_, err := draftModuleValues(context.Background(), provider, ownerSpec, []initcfg.ClassifyTarget{{Name: enrichModAuth}}, "", []string{"doc:README.md (doc) README.md: Auth"})
	if err == nil {
		t.Fatal("unsupported evidence_refs must fail")
	}
	if !strings.Contains(err.Error(), "unsupported evidence_refs") {
		t.Fatalf("error = %v, want unsupported evidence_refs", err)
	}
}

func TestDraftModuleValues_AcceptsAPIEvidenceRefAlias(t *testing.T) {
	t.Parallel()
	provider := &scriptedProvider{
		responses: []string{`[{"module":"auth","value":"@team-auth","rationale":"test cites api:ownership","evidence_refs":["api:ownership"],"basis":"semantic_judgment"}]`},
	}
	drafts, err := draftModuleValues(context.Background(), provider, ownerSpec, []initcfg.ClassifyTarget{{Name: enrichModAuth}}, "", []string{"api:internal-ownership (api) internal/ownership: owner map"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(drafts) != 1 || len(drafts[0].EvidenceRefs) != 1 || drafts[0].EvidenceRefs[0] != "api:internal-ownership" {
		t.Fatalf("drafts = %+v, want canonical api ref", drafts)
	}
}

func TestEnrichVolatilityDraft_RejectsInvalidValue(t *testing.T) {
	t.Parallel()
	// "huge" is not a valid volatility — the parser must drop it, leaving no drafts.
	provider := &scriptedProvider{
		responses: []string{valueJSONFor(map[string]string{enrichModAuth: "huge", enrichModNotify: "huge"})},
	}
	cfgPath, dir := writeEnrichSubdomainFixture(t)
	volPath := filepath.Join(dir, defaultVolatilityPath)

	cmd := &EnrichVolatilityCmd{enrichFlags: enrichFlags{Config: cfgPath, providerOverride: provider}}
	var buf bytes.Buffer
	if err := cmd.Run(&appDeps{Stdout: &buf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	df, err := initcfg.LoadValueDrafts(volPath, "volatility")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(df.Drafts) != 0 {
		t.Errorf("invalid volatility values should be dropped, got: %+v", df.Drafts)
	}
}

func TestEnrichOwner_LLMUnconfigured(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".archfit.yaml")
	content := "version: 1\nmodules:\n  auth:\n    paths:\n      - \"internal/auth/**\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if code := Run([]string{cmdConfig, cmdEnrich, ownerField, "-c", cfgPath}, &buf); code != 3 {
		t.Errorf("exit = %d, want 3 (llm not configured)\noutput: %s", code, buf.String())
	}
}
