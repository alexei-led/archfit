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

// Module name constants used across subdomain enrich tests.
const (
	enrichModAuth   = "auth"
	enrichModNotify = "notify"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeEnrichSubdomainFixture creates a minimal .archfit.yaml with two modules
// (no subdomain set) plus a tools.llm stanza using the ollama provider.
// Returns the config file path and the temp directory.
func writeEnrichSubdomainFixture(t *testing.T) (cfgPath, dir string) {
	t.Helper()
	dir = t.TempDir()
	cfgPath = filepath.Join(dir, ".archfit.yaml")
	content := `version: 1
layers:
  - core
  - adapter
modules:
  auth:
    paths:
      - "internal/auth/**"
    layer: core
  notify:
    paths:
      - "internal/notify/**"
    layer: adapter
tools:
  llm:
    provider: ollama
    model: test-model
    base_url: "http://unused"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath, dir
}

// classifyJSONFor builds the scripted provider JSON for a list of module names,
// assigning fixed subdomain/volatility values.
func classifyJSONFor(names []string) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf(
			`{"module":%q,"subdomain":"core","volatility":"low","layer":"core","name":%q,"rationale":"test"}`,
			n, n,
		))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// ---------------------------------------------------------------------------
// TestEnrichSubdomainDraft — draft workflow writes .archfit-subdomains.yaml
// ---------------------------------------------------------------------------

func TestEnrichSubdomainDraft(t *testing.T) {
	t.Parallel()
	provider := &scriptedProvider{
		responses: []string{classifyJSONFor([]string{enrichModAuth, enrichModNotify})},
	}

	cfgPath, dir := writeEnrichSubdomainFixture(t)
	subdomainsPath := filepath.Join(dir, defaultSubdomainsPath)

	cmd := &EnrichCmd{
		Config:           cfgPath,
		Subdomains:       true,
		Pin:              false,
		providerOverride: provider,
	}

	var buf bytes.Buffer
	deps := &appDeps{Runner: nil, Stdout: &buf}
	if err := cmd.Run(deps); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "draft subdomain") {
		t.Errorf("expected draft message, got: %s", out)
	}

	// The draft file must exist and contain status: draft entries.
	draftFile, err := initcfg.LoadSubdomainDrafts(subdomainsPath)
	if err != nil {
		t.Fatalf("load draft file: %v", err)
	}
	if len(draftFile.Drafts) == 0 {
		t.Fatal("expected at least one draft entry")
	}
	for _, d := range draftFile.Drafts {
		if d.Status != initcfg.SubdomainStatusDraft {
			t.Errorf("draft %q has status %q, want %q", d.Module, d.Status, initcfg.SubdomainStatusDraft)
		}
		if d.Subdomain == "" {
			t.Errorf("draft %q has empty subdomain", d.Module)
		}
	}
}

// ---------------------------------------------------------------------------
// TestEnrichSubdomainDraft_AllClassified — no modules to draft when all have subdomain
// ---------------------------------------------------------------------------

func TestEnrichSubdomainDraft_AllClassified(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".archfit.yaml")
	content := `version: 1
layers:
  - core
modules:
  auth:
    paths:
      - "internal/auth/**"
    subdomain: core
tools:
  llm:
    provider: ollama
    model: test-model
    base_url: "http://unused"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := &EnrichCmd{
		Config:           cfgPath,
		Subdomains:       true,
		providerOverride: &scriptedProvider{responses: []string{}},
	}

	var buf bytes.Buffer
	deps := &appDeps{Runner: nil, Stdout: &buf}
	if err := cmd.Run(deps); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "already have subdomain set") {
		t.Errorf("expected 'already have subdomain set' message, got: %s", buf.String())
	}
}

// ---------------------------------------------------------------------------
// TestEnrichSubdomainPin — pin workflow writes subdomain+reviewed_at+reviewed_by
// ---------------------------------------------------------------------------

func TestEnrichSubdomainPin(t *testing.T) {
	t.Parallel()
	cfgPath, dir := writeEnrichSubdomainFixture(t)
	subdomainsPath := filepath.Join(dir, defaultSubdomainsPath)

	// Pre-write an approved draft file.
	draftFile := initcfg.SubdomainDraftFile{
		Version: 1,
		Drafts: []initcfg.SubdomainDraft{
			{Module: enrichModAuth, Subdomain: "core", Volatility: "low", Status: initcfg.SubdomainStatusApproved},
			{Module: enrichModNotify, Subdomain: "supporting", Status: initcfg.SubdomainStatusApproved},
		},
	}
	if err := initcfg.WriteSubdomainDrafts(subdomainsPath, draftFile); err != nil {
		t.Fatal(err)
	}

	cmd := &EnrichCmd{
		Config:     cfgPath,
		Subdomains: true,
		Pin:        true,
		ReviewedBy: "alice",
	}

	var buf bytes.Buffer
	deps := &appDeps{Runner: nil, Stdout: &buf}
	if err := cmd.Run(deps); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "pinned") {
		t.Errorf("expected pinned message, got: %s", out)
	}

	// The .archfit.yaml must now contain subdomain, reviewed_at, reviewed_by.
	cfgBytes, err := os.ReadFile(cfgPath) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	got := string(cfgBytes)
	if !strings.Contains(got, "subdomain: core") {
		t.Errorf("auth subdomain not written\n%s", got)
	}
	if !strings.Contains(got, "subdomain: supporting") {
		t.Errorf("notify subdomain not written\n%s", got)
	}
	if !strings.Contains(got, "reviewed_at:") {
		t.Errorf("reviewed_at not written\n%s", got)
	}
	if !strings.Contains(got, "reviewed_by: alice") {
		t.Errorf("reviewed_by not written\n%s", got)
	}
	if !strings.Contains(got, "volatility: low") {
		t.Errorf("volatility not written\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// TestEnrichSubdomainPin_NoApproved — prints guidance when nothing is approved
// ---------------------------------------------------------------------------

func TestEnrichSubdomainPin_NoApproved(t *testing.T) {
	t.Parallel()
	cfgPath, dir := writeEnrichSubdomainFixture(t)
	subdomainsPath := filepath.Join(dir, defaultSubdomainsPath)

	draftFile := initcfg.SubdomainDraftFile{
		Version: 1,
		Drafts: []initcfg.SubdomainDraft{
			{Module: enrichModAuth, Subdomain: "core", Status: initcfg.SubdomainStatusDraft},
		},
	}
	if err := initcfg.WriteSubdomainDrafts(subdomainsPath, draftFile); err != nil {
		t.Fatal(err)
	}

	cmd := &EnrichCmd{
		Config:     cfgPath,
		Subdomains: true,
		Pin:        true,
	}

	var buf bytes.Buffer
	deps := &appDeps{Runner: nil, Stdout: &buf}
	if err := cmd.Run(deps); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "no approved") {
		t.Errorf("expected 'no approved' message, got: %s", buf.String())
	}
}

// ---------------------------------------------------------------------------
// TestEnrichSubdomainPin_NoDraftFile — pin with no draft file prints guidance
// ---------------------------------------------------------------------------

func TestEnrichSubdomainPin_NoDraftFile(t *testing.T) {
	t.Parallel()
	cfgPath, _ := writeEnrichSubdomainFixture(t)

	cmd := &EnrichCmd{
		Config:     cfgPath,
		Subdomains: true,
		Pin:        true,
	}

	var buf bytes.Buffer
	deps := &appDeps{Runner: nil, Stdout: &buf}
	if err := cmd.Run(deps); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "no approved") {
		t.Errorf("expected 'no approved' message, got: %s", buf.String())
	}
}

// ---------------------------------------------------------------------------
// TestEnrichSubdomains_LLMUnconfigured — --subdomains without tools.llm → exit 3
// ---------------------------------------------------------------------------

func TestEnrichSubdomains_LLMUnconfigured(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".archfit.yaml")
	content := `version: 1
modules:
  auth:
    paths:
      - "internal/auth/**"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	code := Run([]string{"enrich", "--subdomains", "-c", cfgPath}, &buf)
	if code != 3 {
		t.Errorf("exit = %d, want 3 (llm not configured)\noutput: %s", code, buf.String())
	}
}
