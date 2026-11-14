package main

import (
	"strings"
	"testing"
)

func TestReportIgnoresRetiredLabelsFile(t *testing.T) {
	t.Parallel()
	const invalidDraft = "drafts: [invalid"
	for _, policy := range []string{
		fixturePolicy,
		"version: 3\nowners: [owner]\nmodules:\n  all:\n    paths: ['**']\n    volatility: low\n    subdomain: generic\n",
	} {
		files := with(fixtureFiles(), map[string]string{
			defaultConfigPath:          policy,
			".archfit-labels.yaml":     "labels: [this is not valid YAML",
			".archfit-volatility.yaml": invalidDraft,
			".archfit-subdomains.yaml": invalidDraft,
			".archfit-owners.yaml":     "drafts: [invalid",
		})
		dir := fixtureRepo(t, files, nil)
		got := runCLI(t, fixtureDate, cmdReport, "-C", dir)
		if got.code != 0 {
			t.Fatalf("report read the retired labels file: exit %d; stderr: %s", got.code, got.stderr)
		}
		if strings.Contains(got.stdout+got.stderr, ".archfit-labels.yaml") {
			t.Fatalf("report still depends on labels: %s\n%s", got.stdout, got.stderr)
		}
	}
}
