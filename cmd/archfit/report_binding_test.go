package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v4/internal/application"
	"github.com/alexei-led/archfit/v4/internal/config"
	gitpkg "github.com/alexei-led/archfit/v4/internal/history/git"
	"github.com/alexei-led/archfit/v4/internal/model/report"
	"github.com/alexei-led/archfit/v4/internal/toolrun"
)

func TestReportConsumedPolicySurvivesStableDiskReplacement(t *testing.T) {
	t.Parallel()
	policyA := strings.Replace(fixturePolicy, "    volatility: high\n", "", 1) + "exclude: [.archfit.yaml]\n"
	policyB := strings.Replace(policyA, "    paths: [pkg/chaos/**]\n", "    paths: [pkg/chaos/**]\n    volatility: low\n", 1) + "builtin:\n  graph: off\n"
	dir := fixtureRepo(t, with(fixtureFiles(), map[string]string{defaultConfigPath: policyA}), map[string]string{tcNetem: tcImportsUtil})
	objects, err := gitpkg.NewObjectStore(toolrun.New())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = objects.Close() }()
	var stdout, stderr bytes.Buffer
	deps := &appDeps{Runner: objects, Stdout: &stdout, Stderr: &stderr, now: day(fixtureDate)}
	ctx := deps.runContext()
	co, err := openCheckout(ctx, deps, dir)
	if err != nil {
		t.Fatal(err)
	}
	data, found, err := co.policyAt("")
	if err != nil || !found {
		t.Fatalf("read policy A: %v %v", found, err)
	}
	in, err := config.NewReportInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, defaultConfigPath), []byte(policyB), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (&ReportCmd{Format: formatJSON}).runLoaded(ctx, deps, co, in); err != nil {
		t.Fatalf("report continuation: %v\n%s", err, stderr.String())
	}
	var doc report.ArchitectureState
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	assertConsumedReport(t, doc, in)
	acquired, err := reportEvidenceService(co.policyPath(""), co.root, in, deps).Acquire(ctx, application.AnalysisRequest{ConfigSource: co.policyPath(""), BundleDir: co.root})
	if err != nil {
		t.Fatalf("acquisition: %v", err)
	}
	if acquired.Context.ConfigHash != strings.TrimPrefix(in.Policy.Digest, "sha256:") {
		t.Fatalf("acquisition ConfigHash = %q, want A", acquired.Context.ConfigHash)
	}
	if doc.Repository == nil || doc.Repository.State != report.RepositoryDrift || len(doc.Repository.Drift) != 1 || doc.Repository.Drift[0].Rule != ruleGraph {
		t.Fatalf("repository state must be judged under A: %+v", doc.Repository)
	}
	fresh := runCLI(t, fixtureDate, cmdReport, "-C", dir, flagFormat, formatJSON)
	if fresh.code != 0 {
		t.Fatalf("fresh B report: %s", fresh.stderr)
	}
	var b report.ArchitectureState
	if err := json.Unmarshal([]byte(fresh.stdout), &b); err != nil {
		t.Fatal(err)
	}
	if b.Repository == nil || b.Repository.State != report.RepositoryClean {
		t.Fatalf("fresh B must differ from A rule settings: %+v", b.Repository)
	}
}

func assertConsumedReport(t *testing.T, doc report.ArchitectureState, in config.ReportInput) {
	t.Helper()
	snap := doc.Evidence.Snapshot
	if snap.PolicyBinding != "mismatch" || snap.Stability != reportSnapshotChanged || snap.PolicyBeforeDigest == nil || snap.PolicyAfterDigest == nil || *snap.PolicyBeforeDigest != *snap.PolicyAfterDigest {
		t.Fatalf("stable B must be mismatched against consumed A: %+v", snap)
	}
	if doc.Evidence.Scope.PolicyRef.SHA256 == nil || *doc.Evidence.Scope.PolicyRef.SHA256 != in.Policy.Digest || doc.Identity.PolicyDigest != strings.TrimPrefix(in.Policy.Digest, "sha256:") {
		t.Fatalf("identity does not retain A: %+v %+v", doc.Identity, doc.Evidence.Scope.PolicyRef)
	}
	var chaos *report.EvidenceComponent
	for i := range doc.Evidence.Components {
		if doc.Evidence.Components[i].Name == "pkg_chaos" {
			chaos = &doc.Evidence.Components[i]
		}
	}
	if chaos == nil || chaos.Volatility.Raw != nil || chaos.Volatility.Effective == nil || *chaos.Volatility.Effective != "high" || chaos.Volatility.Origin != "assumed" {
		t.Fatalf("origin/config must come from A: %+v", chaos)
	}
}
