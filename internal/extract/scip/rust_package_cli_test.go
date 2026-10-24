package scip

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestRustWorkspacePackageEdgesPublicCLI reproduces the native image contract's
// two-package seam using real bundled Rust tools and an explicitly owned reader.
func TestRustWorkspacePackageEdgesPublicCLI(t *testing.T) {
	requireRustPackageTools(t)
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "archfit")
	runRustPackageCommand(t, repo, "go", "build", "-o", binary, "./cmd/archfit")
	for _, tc := range []struct {
		name, coreTarget, adapterTarget, call, status, strength string
		scored                                                  int
	}{
		{"native fixture", "smoke_core", "smoke_adapter", "smoke_adapter::value()", "ok", strengthFunctional, 1},
		{"custom targets", "caller_lib", "service_lib", "service_lib::value()", "ok", strengthFunctional, 1},
		{"unused dependency", "caller_lib", "service_lib", "1", "ok", "unknown", 0},
		{"colliding targets", "shared", "shared", "service_alias::value()", "partial", "unknown", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dependency := "smoke-adapter"
			packageField := ""
			if tc.name == "colliding targets" {
				dependency = "service_alias"
				packageField = "package='smoke-adapter',"
			}
			files := map[string]string{
				"Cargo.toml":         "[workspace]\nmembers=['core','adapter']\nresolver='2'\n",
				"core/Cargo.toml":    fmt.Sprintf("[package]\nname='smoke-core'\nversion='0.1.0'\nedition='2021'\n[lib]\nname='%s'\n[dependencies]\n%s={%spath='../adapter'}\n", tc.coreTarget, dependency, packageField),
				"adapter/Cargo.toml": fmt.Sprintf("[package]\nname='smoke-adapter'\nversion='0.1.0'\nedition='2021'\n[lib]\nname='%s'\n", tc.adapterTarget),
				"core/src/lib.rs":    "pub fn run() -> u32 { " + tc.call + " }\n",
				"adapter/src/lib.rs": "pub fn value() -> u32 { 1 }\n",
				"deep.yaml":          "version: 2\nlanguages:\n  go: {enabled: false}\n  typescript: {enabled: false}\n  python: {enabled: false}\n  rust: {enabled: true}\nanalyzers:\n  syntax: {enabled: false}\n  scip: {enabled: true}\n  clones: {enabled: false}\n  cargo_modules: {enabled: true}\nmodules:\n  core:\n    paths: ['smoke-core']\n    owner: team-core\n  adapter:\n    paths: ['smoke-adapter']\n    owner: team-adapter\n",
			}
			for path, contents := range files {
				path = filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runRustPackageCommand(t, root, "cargo", "check", "--workspace", "--offline")
			var cold []publicRustTool
			for run := range 2 {
				args := []string{"analyze", "--root", root, "--config", filepath.Join(root, "deep.yaml"), "--json", "--progress", "none"}
				if run == 0 {
					args = append(args, "--refresh")
				}
				raw := runRustPackageCommand(t, root, binary, args...)
				var report struct {
					Coverage struct {
						Tools []publicRustTool `json:"tools"`
					} `json:"coverage"`
					Seams []struct {
						From     string `json:"from_module"`
						To       string `json:"to_module"`
						Strength string `json:"strength"`
						Scored   int    `json:"scored_edges"`
					} `json:"seams"`
				}
				if err := json.Unmarshal(raw, &report); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, seam := range report.Seams {
					if seam.From == "core" && seam.To == "adapter" {
						found = true
						if seam.Strength != tc.strength || seam.Scored != tc.scored {
							t.Fatalf("primary Cargo seam = %+v, want %s/%d scored", seam, tc.strength, tc.scored)
						}
					}
				}
				if !found {
					t.Fatal("declared Cargo package edge disappeared")
				}
				var tools []publicRustTool
				for _, tool := range report.Coverage.Tools {
					if tool.Tool == toolName || tool.Tool == toolNameSymbols {
						tools = append(tools, tool)
						if tool.Status != tc.status {
							t.Fatalf("attribution status = %+v", tool)
						}
						if tc.status == "partial" && !strings.Contains(tool.Reason, "Rust package aliases") {
							t.Fatalf("collision reason hidden: %+v", tool)
						}
					}
				}
				if len(tools) != 2 {
					t.Fatalf("missing SCIP coverage rows: %+v", tools)
				}
				if run == 0 {
					cold = tools
				} else if !reflect.DeepEqual(cold, tools) {
					t.Fatalf("cold/warm status differs: %+v %+v", cold, tools)
				}
				t.Logf("%s run%d: %s/%d scored; coverage %+v", tc.name, run, tc.strength, tc.scored, tools)
			}
		})
	}
}

type publicRustTool struct {
	Tool   string `json:"tool"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func requireRustPackageTools(t *testing.T) {
	t.Helper()
	missing := func(reason string) {
		t.Helper()
		if os.Getenv("ARCHFIT_REQUIRE_TOOLS") == "1" {
			t.Fatal(reason)
		}
		t.Skip(reason)
	}
	if !filepath.IsAbs(os.Getenv("ARCHFIT_TOOL_PYTHON")) {
		missing("Rust public CLI regression needs an explicit provisioned ARCHFIT_TOOL_PYTHON")
	}
	for _, tool := range []string{toolCargo, indexerRust, "cargo-modules"} {
		if _, err := exec.LookPath(tool); err != nil {
			missing("Rust public CLI regression requires " + tool)
		}
	}
	version := string(runRustPackageCommand(t, t.TempDir(), indexerRust, "--version"))
	if !strings.HasPrefix(version, "rust-analyzer 1.99.1") {
		missing("Rust public CLI regression requires bundled rust-analyzer1.99.1; got " + version)
	}
}

func runRustPackageCommand(t *testing.T, dir, name string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- fixed test tools/binary; arguments are generated fixture data
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s\n%s", name, args, err, out, stderr.String())
	}
	return out
}
