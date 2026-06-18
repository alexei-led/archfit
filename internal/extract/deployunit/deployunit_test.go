package deployunit_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/deployunit"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	unitAPIService = "api-service"
	unitWebApp     = "web-app"
	unitCLI        = "cli"
)

// emptyRunner returns a RunnerMock that reports no tools available and never runs.
func emptyRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{}, nil
		},
	}
}

// goRunner returns a RunnerMock that simulates `go list` returning absolute paths
// for main packages. mainDirs must be absolute paths under root.
func goRunner(mainDirs []string) *toolrun.RunnerMock {
	var stdout []byte
	for _, d := range mainDirs {
		stdout = append(stdout, []byte(d+"\n")...)
	}
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "go" {
				return toolrun.ToolInfo{Name: "go", Path: "/usr/local/go/bin/go"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: stdout}, nil
		},
	}
}

// emptyModuleMap builds a ModuleMap with no modules (no glob matching).
func emptyModuleMap() config.ModuleMap {
	return (config.Config{}).ModuleMapView()
}

// singleModuleMap builds a ModuleMap with one module covering the given path glob.
func singleModuleMap(name, pathGlob string) config.ModuleMap {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			name: {Paths: []string{pathGlob}},
		},
	}
	return cfg.ModuleMapView()
}

// mustMkdir creates a directory in the test, fataling on error.
func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatal(err)
	}
}

// mustWrite writes content to path, fataling on error.
func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDetect_GoMain verifies that Go main packages detected via go list
// are recorded as deploy units.
func TestDetect_GoMain(t *testing.T) {
	root := t.TempDir()
	mainDir := filepath.Join(root, "cmd", "myapp")
	mustMkdir(t, mainDir)

	runner := goRunner([]string{mainDir})
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	want := "cmd/myapp"
	unit, ok := result[want]
	if !ok {
		t.Errorf("expected deploy unit at %q, got none; result=%v", want, result)
	}
	if unit == "" {
		t.Errorf("unit name for %q is empty", want)
	}
}

// TestDetect_GoMain_ModuleNamePreferred verifies that when a ModuleMap matches
// the detected main package path, the module name is used as the unit name.
func TestDetect_GoMain_ModuleNamePreferred(t *testing.T) {
	root := t.TempDir()
	mainDir := filepath.Join(root, "cmd", "server")
	mustMkdir(t, mainDir)

	runner := goRunner([]string{mainDir})
	mm := singleModuleMap(unitCLI, "cmd/server/**")

	result := deployunit.Detect(context.Background(), root, mm, runner)

	unit, ok := result["cmd/server"]
	if !ok {
		t.Errorf("expected unit at cmd/server, got none; result=%v", result)
	}
	if unit != unitCLI {
		t.Errorf("unit name = %q, want %q (module name from ModuleMap)", unit, unitCLI)
	}
}

// TestDetect_GoMain_ToolAbsent verifies that when `go` is not on PATH,
// no Go main units are detected.
func TestDetect_GoMain_ToolAbsent(t *testing.T) {
	root := t.TempDir()
	runner := emptyRunner()
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	if len(result) != 0 {
		t.Errorf("expected empty result when go absent, got %v", result)
	}
}

// TestDetect_TSWorkspaces verifies detection from a package.json workspaces field.
func TestDetect_TSWorkspaces(t *testing.T) {
	root := t.TempDir()

	// Write root package.json with workspaces.
	rootPkg := `{"name":"monorepo","workspaces":["packages/api","packages/web"]}`
	mustWrite(t, filepath.Join(root, "package.json"), rootPkg)

	// Write workspace package.json files.
	for _, ws := range []struct{ path, name string }{
		{"packages/api", unitAPIService},
		{"packages/web", unitWebApp},
	} {
		dir := filepath.Join(root, ws.path)
		mustMkdir(t, dir)
		mustWrite(t, filepath.Join(dir, "package.json"), `{"name":"`+ws.name+`"}`)
	}

	runner := emptyRunner()
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	for _, tc := range []struct {
		path string
		name string
	}{
		{"packages/api", unitAPIService},
		{"packages/web", unitWebApp},
	} {
		unit, ok := result[tc.path]
		if !ok {
			t.Errorf("expected unit at %q, got none; result=%v", tc.path, result)
			continue
		}
		if unit != tc.name {
			t.Errorf("unit at %q = %q, want %q", tc.path, unit, tc.name)
		}
	}
}

// TestDetect_TSBinMain verifies detection from root package.json with bin or main.
func TestDetect_TSBinMain(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "package.json"), `{"name":"my-cli","bin":"dist/cli.js"}`)

	runner := emptyRunner()
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	unit, ok := result["."]
	if !ok {
		t.Errorf("expected unit at '.', got none; result=%v", result)
	}
	if unit != "my-cli" {
		t.Errorf("unit = %q, want %q", unit, "my-cli")
	}
}

// TestDetect_PythonPyproject verifies detection from pyproject.toml [project].name.
func TestDetect_PythonPyproject(t *testing.T) {
	root := t.TempDir()
	svcDir := filepath.Join(root, "services", "billing")
	mustMkdir(t, svcDir)
	mustWrite(t, filepath.Join(svcDir, "pyproject.toml"),
		"[project]\nname = \"billing-service\"\nversion = \"1.0.0\"\n")

	runner := emptyRunner()
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	unit, ok := result["services/billing"]
	if !ok {
		t.Errorf("expected unit at 'services/billing', got none; result=%v", result)
	}
	if unit != "billing-service" {
		t.Errorf("unit = %q, want %q", unit, "billing-service")
	}
}

// TestDetect_PythonPyproject_NoProjectSection verifies that a pyproject.toml
// without [project].name falls back to the directory name.
func TestDetect_PythonPyproject_NoProjectSection(t *testing.T) {
	root := t.TempDir()
	svcDir := filepath.Join(root, "myservice")
	mustMkdir(t, svcDir)
	mustWrite(t, filepath.Join(svcDir, "pyproject.toml"),
		"[build-system]\nrequires = [\"setuptools\"]\n")

	runner := emptyRunner()
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	unit, ok := result["myservice"]
	if !ok {
		t.Errorf("expected unit at 'myservice', got none; result=%v", result)
	}
	if unit != "myservice" {
		t.Errorf("unit = %q, want %q", unit, "myservice")
	}
}

// TestDetect_Dockerfile verifies detection from Dockerfile presence.
func TestDetect_Dockerfile(t *testing.T) {
	root := t.TempDir()
	svcDir := filepath.Join(root, "services", "auth")
	mustMkdir(t, svcDir)
	mustWrite(t, filepath.Join(svcDir, "Dockerfile"), "FROM alpine\n")

	runner := emptyRunner()
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	unit, ok := result["services/auth"]
	if !ok {
		t.Errorf("expected unit at 'services/auth', got none; result=%v", result)
	}
	if unit == "" {
		t.Errorf("unit name is empty for services/auth")
	}
}

// TestDetect_Dockerfile_Variant verifies detection from Dockerfile.prod etc.
func TestDetect_Dockerfile_Variant(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "worker")
	mustMkdir(t, dir)
	mustWrite(t, filepath.Join(dir, "Dockerfile.prod"), "FROM alpine\n")

	runner := emptyRunner()
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	if _, ok := result["worker"]; !ok {
		t.Errorf("expected unit at 'worker', got none; result=%v", result)
	}
}

// TestDetect_K8sDeployment verifies detection from a k8s Deployment manifest.
func TestDetect_K8sDeployment(t *testing.T) {
	root := t.TempDir()
	k8sDir := filepath.Join(root, "deploy", "k8s")
	mustMkdir(t, k8sDir)
	mustWrite(t, filepath.Join(k8sDir, "payment.yaml"),
		"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: payment-service\nspec:\n  replicas: 1\n")

	runner := emptyRunner()
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	unit, ok := result["deploy/k8s"]
	if !ok {
		t.Errorf("expected unit at 'deploy/k8s', got none; result=%v", result)
	}
	if unit != "payment-service" {
		t.Errorf("unit = %q, want %q", unit, "payment-service")
	}
}

// TestDetect_K8sStatefulSet verifies detection from a k8s StatefulSet manifest.
func TestDetect_K8sStatefulSet(t *testing.T) {
	root := t.TempDir()
	k8sDir := filepath.Join(root, "infra")
	mustMkdir(t, k8sDir)
	mustWrite(t, filepath.Join(k8sDir, "db.yaml"),
		"kind: StatefulSet\nmetadata:\n  name: postgres-db\n")

	runner := emptyRunner()
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	unit, ok := result["infra"]
	if !ok {
		t.Errorf("expected unit at 'infra', got none; result=%v", result)
	}
	if unit != "postgres-db" {
		t.Errorf("unit = %q, want %q", unit, "postgres-db")
	}
}

// TestDetect_GoWins verifies first-write-wins priority: Go main detection
// wins over Dockerfile in the same directory.
func TestDetect_GoWins(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "cmd", "app")
	mustMkdir(t, appDir)
	// Dockerfile is also present in the same dir — Go detection runs first and wins.
	mustWrite(t, filepath.Join(appDir, "Dockerfile"), "FROM scratch\n")

	runner := goRunner([]string{appDir})
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	if _, ok := result["cmd/app"]; !ok {
		t.Errorf("expected unit at 'cmd/app', got none; result=%v", result)
	}
}

// TestDetect_SkipsNodeModules verifies that node_modules directories are skipped.
func TestDetect_SkipsNodeModules(t *testing.T) {
	root := t.TempDir()
	nmDir := filepath.Join(root, "node_modules", "some-pkg")
	mustMkdir(t, nmDir)
	mustWrite(t, filepath.Join(nmDir, "Dockerfile"), "FROM scratch\n")

	runner := emptyRunner()
	mm := emptyModuleMap()

	result := deployunit.Detect(context.Background(), root, mm, runner)

	for path := range result {
		if strings.HasPrefix(path, "node_modules") {
			t.Errorf("node_modules path leaked into result: %q", path)
		}
	}
}

// TestFillMissingDeployUnits verifies that config-authored deploy_unit wins
// and empty modules are filled from the resolved map.
func TestFillMissingDeployUnits(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"api":   {Paths: []string{"api/**"}, DeployUnit: unitAPIService}, // config wins
			"web":   {Paths: []string{"web/**"}},                             // empty — should be filled
			unitCLI: {Paths: []string{"cmd/**"}, DeployUnit: ""},             // empty — should be filled
		},
	}

	resolved := map[string]string{
		"api":   "detected-api", // must NOT overwrite config value
		"web":   "detected-web", // must fill
		unitCLI: "detected-cli", // must fill
		"xyz":   "ghost-unit",   // no such module — ignored
	}

	cfg.FillMissingDeployUnits(resolved)

	if got := cfg.Modules["api"].DeployUnit; got != unitAPIService {
		t.Errorf("api.DeployUnit = %q, want %q (config must win)", got, unitAPIService)
	}
	if got := cfg.Modules["web"].DeployUnit; got != "detected-web" {
		t.Errorf("web.DeployUnit = %q, want %q", got, "detected-web")
	}
	if got := cfg.Modules[unitCLI].DeployUnit; got != "detected-cli" {
		t.Errorf("cli.DeployUnit = %q, want %q", got, "detected-cli")
	}
}
