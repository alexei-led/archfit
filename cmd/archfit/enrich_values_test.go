package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/initcfg"
)

// valueJSONFor builds a scripted provider response assigning each module the
// given value, in the {"module","value","rationale"} shape the value drafters expect.
func valueJSONFor(pairs map[string]string) string {
	parts := make([]string, 0, len(pairs))
	for mod, val := range pairs {
		parts = append(parts, fmt.Sprintf(`{"module":%q,"value":%q,"rationale":"test"}`, mod, val))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestEnrichOwnerDraft(t *testing.T) {
	provider := &scriptedProvider{
		responses: []string{valueJSONFor(map[string]string{enrichModAuth: "@team-auth", enrichModNotify: "@team-notify"})},
	}
	cfgPath, dir := writeEnrichSubdomainFixture(t)
	ownersPath := filepath.Join(dir, defaultOwnersPath)

	cmd := &EnrichCmd{Config: cfgPath, Owner: true, providerOverride: provider}
	var buf bytes.Buffer
	if err := cmd.Run(&appDeps{Stdout: &buf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "draft owner") {
		t.Errorf("expected draft owner message, got: %s", buf.String())
	}

	df, err := initcfg.LoadValueDrafts(ownersPath, "owner")
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
	}
}

func TestEnrichOwnerPin(t *testing.T) {
	cfgPath, dir := writeEnrichSubdomainFixture(t)
	ownersPath := filepath.Join(dir, defaultOwnersPath)

	df := initcfg.ValueDraftFile{Version: 1, Field: "owner", Drafts: []initcfg.ValueDraft{
		{Module: enrichModAuth, Value: "@team-auth", Status: initcfg.DraftStatusApproved},
		{Module: enrichModNotify, Value: "@team-notify", Status: initcfg.DraftStatusDraft}, // not approved → skipped
	}}
	if err := initcfg.WriteValueDrafts(ownersPath, df); err != nil {
		t.Fatal(err)
	}

	cmd := &EnrichCmd{Config: cfgPath, Owner: true, Pin: true, ReviewedBy: "rev"}
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
	provider := &scriptedProvider{
		responses: []string{valueJSONFor(map[string]string{enrichModAuth: volatilityLow, enrichModNotify: volatilityMedium})},
	}
	cfgPath, dir := writeEnrichSubdomainFixture(t)
	volPath := filepath.Join(dir, defaultVolatilityPath)

	// Draft.
	draftCmd := &EnrichCmd{Config: cfgPath, Volatility: true, providerOverride: provider}
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
	pinCmd := &EnrichCmd{Config: cfgPath, Volatility: true, Pin: true, ReviewedBy: "rev"}
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

func TestEnrichVolatilityDraft_RejectsInvalidValue(t *testing.T) {
	// "huge" is not a valid volatility — the parser must drop it, leaving no drafts.
	provider := &scriptedProvider{
		responses: []string{valueJSONFor(map[string]string{enrichModAuth: "huge", enrichModNotify: "huge"})},
	}
	cfgPath, dir := writeEnrichSubdomainFixture(t)
	volPath := filepath.Join(dir, defaultVolatilityPath)

	cmd := &EnrichCmd{Config: cfgPath, Volatility: true, providerOverride: provider}
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
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".archfit.yaml")
	content := "version: 1\nmodules:\n  auth:\n    paths:\n      - \"internal/auth/**\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if code := Run([]string{"enrich", "--owner", "-c", cfgPath}, &buf); code != 3 {
		t.Errorf("exit = %d, want 3 (llm not configured)\noutput: %s", code, buf.String())
	}
}
