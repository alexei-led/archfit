package main

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v4/internal/config"
	"github.com/alexei-led/archfit/v4/internal/toolrun"
)

const (
	testAstGrep = "ast-grep"
	testNode    = "node"
	testBrew    = "brew"
)

func TestDoctorTypeScriptToolGroups(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			out, err := runDoctorResult(t, t.TempDir(), format)
			if err != nil {
				t.Fatal(err)
			}
			if format == "text" {
				check, _, _ := strings.Cut(out, "Optional, for the report:")
				if !regexp.MustCompile(`(?m)^ast-grep +missing +typescript\b`).MatchString(check) || doctorRow(testNode, "missing", "").MatchString(out) {
					t.Fatalf("unexpected TypeScript tool groups:\n%s", out)
				}
				return
			}
			var doc doctorReport
			if err := json.Unmarshal([]byte(out), &doc); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range doc.Tools {
				if row.Name == testNode {
					t.Fatal("doctor reported node as a tool")
				}
				if row.Name == testAstGrep {
					found = row.Group == toolGroupCheck && row.Status == "missing" && slices.Contains(row.Languages, config.LangTypeScript)
				}
			}
			if !found {
				t.Fatalf("ast-grep is not a missing TypeScript check tool: %s", out)
			}
		})
	}
}

func TestDoctorFixTypeScriptTools(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		astGrep, brew, dryRun bool
		want                  string
		install               bool
	}{
		{name: "present without node", astGrep: true, want: "ast-grep         ok"},
		{name: "missing without brew", want: "https://ast-grep.github.io/guide/quick-start.html"},
		{name: "install with brew", brew: true, want: "brew install ast-grep", install: true},
		{name: "dry run with brew", brew: true, dryRun: true, want: "[dry-run] brew install ast-grep"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chdirIsolated(t, t.TempDir())
			var commands []toolrun.ToolCmd
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
					if name == testNode {
						t.Error("doctor checked node for TypeScript")
					}
					return toolrun.ToolInfo{}, name == testAstGrep && tc.astGrep || name == testBrew && tc.brew
				},
				RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					if cmd.Name == testBrew {
						commands = append(commands, cmd)
						return toolrun.Output{}, nil
					}
					return toolrun.New().Run(ctx, cmd)
				},
			}
			var out bytes.Buffer
			cmd := DoctorCmd{Fix: true, Lang: []string{"ts"}, DryRun: tc.dryRun}
			if err := cmd.Run(&appDeps{Runner: runner, Stdout: &out}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(fixSection(out.String()), tc.want) {
				t.Errorf("output does not contain %q:\n%s", tc.want, out.String())
			}
			if tc.install {
				if len(commands) != 1 || !slices.Equal(commands[0].Args, []string{"install", testAstGrep}) {
					t.Errorf("install commands = %v", commands)
				}
			} else if len(commands) != 0 {
				t.Errorf("unexpected install commands: %v", commands)
			}
		})
	}
}
