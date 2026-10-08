package deployunit_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/deployunit"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	discoveryAPI    = "api"
	discoveryWorker = "worker"
)

func TestDiscoverNonGoDoesNotRequireGo(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "package.json"), `{"name":"web","main":"index.js"}`)
	runner := emptyRunner()
	got := deployunit.Discover(context.Background(), root, emptyModuleMap(), runner, evidenceports.ExtractConfig{})
	if got.Coverage.Status != evidence.StatusOK || len(got.Units) != 1 {
		t.Fatalf("discovery = %+v, want complete TS deployment", got)
	}
	if len(runner.DetectCalls()) != 0 || len(runner.RunCalls()) != 0 {
		t.Fatal("non-Go deployment discovery invoked the Go toolchain")
	}
}

func TestDiscoverGoMembersRetainsSuccessfulFacts(t *testing.T) {
	root := t.TempDir()
	for _, member := range []string{discoveryAPI, discoveryWorker} {
		mustMkdir(t, filepath.Join(root, member))
		mustWrite(t, filepath.Join(root, member, "go.mod"), "module example.com/"+member+"\n\ngo 1.26\n")
	}
	mustWrite(t, filepath.Join(root, "go.work"), "go 1.26\nuse (\n./api\n./worker\n)\n")
	runner := goRunner(nil)
	runner.RunFunc = func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		if filepath.Base(cmd.WorkDir) == discoveryWorker {
			return toolrun.Output{}, context.DeadlineExceeded
		}
		return toolrun.Output{Stdout: []byte(cmd.WorkDir + "\n")}, nil
	}
	got := deployunit.Discover(context.Background(), root, emptyModuleMap(), runner, evidenceports.ExtractConfig{})
	if got.Coverage.Status != evidence.StatusTimedOut || !strings.Contains(got.Coverage.Reason, discoveryWorker) || len(got.Units) != 1 {
		t.Fatalf("discovery = %+v, want timeout with successful api fact and worker failure", got)
	}
	if got.Units[discoveryAPI].Source != evidence.TopologySourceGoMain {
		t.Fatalf("successful member fact lost: %+v", got.Units)
	}
}

func TestDiscoverGoTimeoutCategorySurvivesOtherSources(t *testing.T) {
	for _, timedOut := range []string{"a", "c", ""} {
		t.Run("timeout member "+timedOut, func(t *testing.T) {
			root := t.TempDir()
			for _, member := range []string{"a", "b", "c", "successful"} {
				mustMkdir(t, filepath.Join(root, member))
				mustWrite(t, filepath.Join(root, member, "go.mod"), "module example.com/"+member+"\n\ngo 1.26\n")
			}
			mustWrite(t, filepath.Join(root, "Dockerfile"), "FROM scratch\n")
			runner := goRunner(nil)
			runner.RunFunc = func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
				switch filepath.Base(cmd.WorkDir) {
				case timedOut:
					return toolrun.Output{}, context.DeadlineExceeded
				case "successful":
					return toolrun.Output{Stdout: []byte(cmd.WorkDir + "\n")}, nil
				default:
					return toolrun.Output{ExitCode: 1}, nil
				}
			}
			wantStatus := evidence.StatusTimedOut
			if timedOut == "" {
				wantStatus = evidence.StatusPartial
			}
			got := deployunit.Discover(context.Background(), root, emptyModuleMap(), runner, evidenceports.ExtractConfig{})
			if got.Coverage.Status != wantStatus || got.Coverage.FilesSeen != 2 {
				t.Fatalf("coverage = %+v, want %s retaining two facts", got.Coverage, wantStatus)
			}
			if got.Units["successful"].Source != evidence.TopologySourceGoMain || got.Units["."].Source != evidence.TopologySourceDockerfile {
				t.Fatalf("successful facts lost: %+v", got.Units)
			}
			if !strings.Contains(got.Coverage.Reason, "exit 1") || timedOut != "" && !strings.Contains(got.Coverage.Reason, context.DeadlineExceeded.Error()) {
				t.Fatalf("failure reasons lost: %+v", got.Coverage)
			}
		})
	}
}

func TestDiscoverGoHonorsMemberSelectionAndBuildFlags(t *testing.T) {
	root := t.TempDir()
	for _, member := range []string{discoveryAPI, discoveryWorker} {
		mustMkdir(t, filepath.Join(root, member))
		mustWrite(t, filepath.Join(root, member, "go.mod"), "module example.com/"+member+"\n\ngo 1.26\n")
	}
	runner := goRunner(nil)
	got := deployunit.Discover(context.Background(), root, emptyModuleMap(), runner, evidenceports.ExtractConfig{
		GoModuleInclude: []string{discoveryAPI}, BuildFlags: []string{"-tags=deploytest"},
	})
	if got.Coverage.Status != evidence.StatusOK {
		t.Fatalf("discovery coverage = %+v", got.Coverage)
	}
	calls := runner.RunCalls()
	if len(calls) != 1 || calls[0].Cmd.WorkDir != filepath.Join(root, discoveryAPI) || !slices.Contains(calls[0].Cmd.Args, "-tags=deploytest") {
		t.Fatalf("go list calls = %+v", calls)
	}
}

func TestDiscoverRealGoFindParity(t *testing.T) {
	for _, scenario := range []string{"single module", "workspace", "unrelated parent workspace"} {
		t.Run(scenario, func(t *testing.T) {
			parent, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(parent, "repo")
			mustMkdir(t, root)
			members := []string{root}
			if scenario == "workspace" {
				members = []string{filepath.Join(root, discoveryAPI), filepath.Join(root, discoveryWorker)}
				mustWrite(t, filepath.Join(root, "go.work"), "go 1.26\nuse (\n./api\n./worker\n)\n")
			}
			var env []string
			if scenario == "unrelated parent workspace" {
				mustMkdir(t, filepath.Join(parent, "other"))
				mustWrite(t, filepath.Join(parent, "other", "go.mod"), "module example.com/other\n\ngo 1.26\n")
				mustWrite(t, filepath.Join(parent, "go.work"), "go 1.26\nuse ./other\n")
				env = []string{"GOWORK=off"}
			}
			runner := toolrun.New()
			want := make(map[string]evidence.CorroboratedDeployUnit)
			for _, member := range members {
				mustMkdir(t, filepath.Join(member, "cmd", "app"))
				mustWrite(t, filepath.Join(member, "go.mod"), "module example.com/"+filepath.Base(member)+"\n\ngo 1.26\n")
				mustWrite(t, filepath.Join(member, "cmd", "app", "main.go"), "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"hello\") }\n")
			}
			for _, member := range members {
				old, err := runner.Run(context.Background(), toolrun.ToolCmd{
					Name: "go", Args: []string{"list", "-f", "{{if eq .Name \"main\"}}{{.Dir}}{{end}}", "./..."},
					WorkDir: member, Env: env, Timeout: 10 * time.Second,
				})
				if err != nil || old.ExitCode != 0 {
					t.Fatalf("reference go list failed: %v %+v", err, old)
				}
				for line := range strings.SplitSeq(strings.TrimSpace(string(old.Stdout)), "\n") {
					rel, err := filepath.Rel(root, line)
					if err != nil {
						t.Fatal(err)
					}
					want[rel] = evidence.CorroboratedDeployUnit{Path: rel, Unit: "app", Source: evidence.TopologySourceGoMain}
				}
			}
			for range 2 {
				got := deployunit.Discover(context.Background(), root, emptyModuleMap(), runner, evidenceports.ExtractConfig{})
				if got.Coverage.Status != evidence.StatusOK || !reflect.DeepEqual(got.Units, want) {
					t.Fatalf("discovery = %+v, want complete matching %v", got, want)
				}
			}
		})
	}
}

// TestDiscoverGoMainFromAliasedInvocationPath runs discovery from a shell whose
// PWD spells the canonical root through a symlink or a case variant. The Go
// main package must still be found: model_hash covers detected deploy units, so
// a unit that depends on the invocation path makes an unchanged tree compare
// as a policy change.
func TestDiscoverGoMainFromAliasedInvocationPath(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "repo")
	mustMkdir(t, filepath.Join(root, "cmd", "server"))
	mustWrite(t, filepath.Join(root, "go.mod"), "module example.com/repo\n\ngo 1.26\n")
	mustWrite(t, filepath.Join(root, "cmd", "server", "main.go"), "package main\n\nfunc main() {}\n")
	want := map[string]evidence.CorroboratedDeployUnit{
		"cmd/server": {Path: "cmd/server", Unit: "server", Source: evidence.TopologySourceGoMain},
	}

	symlink := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, symlink); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	for name, alias := range map[string]string{
		"symlink":      symlink,
		"case variant": filepath.Join(parent, "REPO"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(alias); err != nil {
				t.Skipf("%s does not resolve here (case-sensitive filesystem): %v", alias, err)
			}
			t.Chdir(alias)
			got := deployunit.Discover(context.Background(), root, emptyModuleMap(), toolrun.New(), evidenceports.ExtractConfig{})
			if got.Coverage.Status != evidence.StatusOK || !reflect.DeepEqual(got.Units, want) {
				t.Fatalf("discovery from %s = %+v, want %v", alias, got, want)
			}
		})
	}
}

func TestDiscoverGoMainDoesNotLoadDependencies(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.26\n")
	mustWrite(t, filepath.Join(root, "main.go"), "package main\nimport _ \"example.invalid/unavailable/module\"\nfunc main() {}\n")
	got := deployunit.Discover(context.Background(), root, emptyModuleMap(), toolrun.New(), evidenceports.ExtractConfig{})
	if got.Coverage.Status != evidence.StatusOK || got.Units["."].Source != evidence.TopologySourceGoMain {
		t.Fatalf("discovery = %+v, want complete main-package discovery without resolving imports", got)
	}
}
