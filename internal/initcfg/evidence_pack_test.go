package initcfg

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeEvidenceFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestBuildArchitectureEvidencePack_DiscoversSourcesWithStableIDs(t *testing.T) {
	root := t.TempDir()
	writeEvidenceFile(t, root, "README.md", "# Payments\n\nOwns settlement intent.\n")
	writeEvidenceFile(t, root, "docs/design/system.md", "# System Design\n\nPayment boundary.\n")
	writeEvidenceFile(t, root, "docs/architecture/layers.md", "# Layers\n\nAdapters stay out.\n")
	writeEvidenceFile(t, root, "docs/adr/0001-payments.md", "# ADR 0001\n\nUse a payments module.\n")
	writeEvidenceFile(t, root, "internal/payments/doc.go", "// Package payments owns settlement workflows.\npackage payments\n")
	writeEvidenceFile(t, root, "internal/payments/api.go", "package payments\n\n// Service settles invoices.\ntype Service struct{}\n\nfunc SettleInvoice() {}\nconst CurrencyUSD = \"USD\"\n")
	writeEvidenceFile(t, root, ".archfit.yaml", "version: 1\nlayers:\n  - domain\nmodules:\n  payments:\n    paths:\n      - \"internal/payments/**\"\n    public:\n      - \"internal/payments/api.go\"\nai:\n  api_key: should-not-leak\n")

	mods := []ModuleDef{{
		Name:   "payments",
		Paths:  []string{"internal/payments/**"},
		Public: []string{"internal/payments/api.go"},
	}}
	opts := EvidencePackOptions{
		Root:    root,
		Modules: mods,
		Diagnostics: []EvidenceDiagnostic{{
			Source:  "discovery",
			Summary: "go modules=1 layers=domain",
		}},
	}

	first := BuildArchitectureEvidencePack(opts)
	second := BuildArchitectureEvidencePack(opts)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("evidence order/content is unstable\nfirst:  %+v\nsecond: %+v", first, second)
	}

	ids := make(map[string]EvidenceItem, len(first))
	for _, item := range first {
		ids[item.ID] = item
	}
	for _, want := range []string{
		testEvidenceREADME,
		"doc:docs/adr/0001-payments.md",
		"doc:docs/architecture/layers.md",
		"doc:docs/design/system.md",
		"comment:internal/payments/doc.go",
		"api:payments",
		"config:.archfit.yaml",
		"diag:discovery#1",
	} {
		if _, ok := ids[want]; !ok {
			t.Fatalf("missing evidence id %q in %+v", want, first)
		}
	}
	if got := ids["api:payments"].Text; !strings.Contains(got, "Service") || !strings.Contains(got, "SettleInvoice") || !strings.Contains(got, "public globs: internal/payments/api.go") {
		t.Fatalf("api evidence missing exported names/public globs: %q", got)
	}
	if got := ids["config:.archfit.yaml"].Text; strings.Contains(got, "should-not-leak") || !strings.Contains(got, "[redacted]") {
		t.Fatalf("config evidence was not redacted: %q", got)
	}
}

func TestBuildArchitectureEvidencePack_BoundsDocsAndSkipsSecretFiles(t *testing.T) {
	root := t.TempDir()
	writeEvidenceFile(t, root, "docs/design/02-second.md", "# Second\n\n"+strings.Repeat("long ", 40))
	writeEvidenceFile(t, root, "docs/design/01-first.md", "# First\n\n"+strings.Repeat("long ", 40))
	writeEvidenceFile(t, root, "docs/design/03-third.md", "# Third\n\n"+strings.Repeat("long ", 40))
	writeEvidenceFile(t, root, "docs/design/secrets.md", "# Secret\n\npassword = hunter2\n")
	writeEvidenceFile(t, root, ".env", "TOKEN=hunter2\n")

	items := BuildArchitectureEvidencePack(EvidencePackOptions{
		Root: root,
		Budget: EvidenceBudget{
			Docs:         2,
			MaxTextBytes: 48,
		},
	})

	var docIDs []string
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.ID+item.Source+item.Text), "secret") || strings.Contains(item.Text, "hunter2") {
			t.Fatalf("secret-like evidence included: %+v", item)
		}
		if item.Kind == EvidenceKindDoc {
			docIDs = append(docIDs, item.ID)
			if len(item.Text) > 51 {
				t.Fatalf("doc text not bounded (%d bytes): %q", len(item.Text), item.Text)
			}
		}
	}
	want := []string{"doc:docs/design/01-first.md", "doc:docs/design/02-second.md"}
	if !reflect.DeepEqual(docIDs, want) {
		t.Fatalf("doc IDs = %v, want %v", docIDs, want)
	}
}

func TestBuildArchitectureEvidencePack_RedactsSecretLikeFreeText(t *testing.T) {
	root := t.TempDir()
	writeEvidenceFile(t, root, "README.md", "# Service\n\nAPI_TOKEN = hunter2\nAPI key: sk-live\nclient secret = s3cr3t\nArchitecture boundary.\n")

	items := BuildArchitectureEvidencePack(EvidencePackOptions{Root: root})
	var readme EvidenceItem
	for _, item := range items {
		if item.ID == testEvidenceREADME {
			readme = item
			break
		}
	}
	if readme.ID == "" {
		t.Fatalf("README evidence missing: %+v", items)
	}
	for _, leaked := range []string{"hunter2", "sk-live", "s3cr3t", "API_TOKEN"} {
		if strings.Contains(readme.Text, leaked) {
			t.Fatalf("secret-like free text leaked %q: %q", leaked, readme.Text)
		}
	}
	if strings.Count(readme.Text, "[redacted]") != 3 {
		t.Fatalf("redaction markers missing: %q", readme.Text)
	}
}
