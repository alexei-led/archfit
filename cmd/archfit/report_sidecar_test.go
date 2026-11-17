package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v4/internal/config"
	"github.com/alexei-led/archfit/v4/internal/evidence/acquisition"
	gitpkg "github.com/alexei-led/archfit/v4/internal/history/git"
	"github.com/alexei-led/archfit/v4/internal/model/report"
	"github.com/alexei-led/archfit/v4/internal/output/jsonout"
	"github.com/alexei-led/archfit/v4/internal/toolrun"
)

const reportSnapshotChanged = "changed"

func TestReportSnapshotTracksConsumedCoverageSidecar(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, sidecarField, sidecarPath, wantStability string
		enabled                                        bool
	}{
		{"default sidecar", "", "coverage.info.sidecar.json", reportSnapshotChanged, true},
		{"explicit sidecar", "      sidecar_path: custom-sidecar.json\n", "custom-sidecar.json", reportSnapshotChanged, true},
		{"disabled coverage", "", "coverage.info.sidecar.json", "stable_by_pre_post_check", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy := fixturePolicy + "coverage:\n  enabled: " + strconv.FormatBool(tc.enabled) + "\n  sources:\n    - path: coverage.info\n      format: lcov\n" + tc.sidecarField
			files := with(fixtureFiles(), map[string]string{defaultConfigPath: policy, "coverage.info": "TN:\nSF:pkg/tc/netem.go\nDA:4,1\nend_of_record\n"})
			sum := sha256.Sum256([]byte(files[tcNetem]))
			sidecar := func(ref string) string {
				data, err := json.Marshal(struct {
					SchemaVersion int               `json:"schema_version"`
					SourceRef     string            `json:"source_ref"`
					Modules       []string          `json:"modules"`
					Sources       map[string]string `json:"sources"`
				}{1, ref, []string{"pkg_tc"}, map[string]string{tcNetem: hex.EncodeToString(sum[:])}})
				if err != nil {
					t.Fatal(err)
				}
				return string(data)
			}
			files[tc.sidecarPath] = sidecar("producer-before")
			dir := fixtureRepo(t, files, nil)
			objects, err := gitpkg.NewObjectStore(toolrun.New())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = objects.Close() }()
			var stderr bytes.Buffer
			deps := &appDeps{Runner: objects, Stderr: &stderr, now: day(fixtureDate)}
			ctx := deps.runContext()
			co, err := openCheckout(ctx, deps, dir)
			if err != nil {
				t.Fatal(err)
			}
			raw, found, err := co.policyAt("")
			if err != nil || !found {
				t.Fatalf("policy read: found=%v err=%v", found, err)
			}
			in, err := config.NewReportInput(raw)
			if err != nil {
				t.Fatal(err)
			}
			snapshots := acquisition.ReportSnapshotSource{Root: co.root, PolicyPath: co.policyPath(""), Exclude: in.Config.RunOptions().Exclusions, Inputs: reportSnapshotInputs(in), Runner: deps.Runner}
			request := reportRequest(ctx, co, deps, in, snapshots)
			if err := os.WriteFile(filepath.Join(dir, tc.sidecarPath), []byte(sidecar("producer-after")), 0600); err != nil {
				t.Fatal(err)
			}
			doc, err := co.measure(ctx, deps, in, request)
			if err != nil {
				t.Fatalf("measure: %v\n%s", err, stderr.String())
			}
			var stdout bytes.Buffer
			if err := jsonout.New().Render(doc, &stdout); err != nil {
				t.Fatal(err)
			}
			var state report.ArchitectureState
			if err := json.Unmarshal(stdout.Bytes(), &state); err != nil {
				t.Fatal(err)
			}
			snap := state.Evidence.Snapshot
			if snap.Stability != tc.wantStability || snap.PolicyBinding != "match" || snap.BeforeDigest == nil || snap.AfterDigest == nil {
				t.Fatalf("sidecar-only mutation: %+v", snap)
			}
			changed := *snap.BeforeDigest != *snap.AfterDigest
			if changed != tc.enabled {
				t.Fatalf("digest changed=%v for enabled=%v", changed, tc.enabled)
			}
			if tc.enabled {
				var ingested bool
				for _, row := range state.Coverage.Tools {
					if row.Tool == "supplied-coverage" {
						ingested = row.Status != "disabled" && !strings.Contains(row.Reason, "sidecar_unavailable")
					}
				}
				if !ingested {
					t.Fatalf("coverage was not consumed: %+v", state.Coverage.Tools)
				}
			}
		})
	}
}
