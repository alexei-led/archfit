package policy3_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition/checkfacts"
	"github.com/alexei-led/archfit/v3/internal/initcfg/policy3"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const archfit3Policy = `version: 2
layers: [core, cmd]
modules:
  app:
    owner: alexei-led
    paths: [cmd/**]
    layer: cmd
    depends_on: [core]
  core:
    paths: [internal/core/**]
    public: [internal/core]
    layer: core
    depends_on: []
rules:
  - id: no-core-to-cmd
    type: forbidden_dependency
    from: internal/core/**
    to: cmd/**
    rationale: Core never imports the CLI.
    alternatives: [Pass what core needs as an argument.]
`

const draftFile = ".archfit-volatility.yaml"

var archfit3Repo = map[string]string{
	"go.mod":                 "module example.com/shop\n\ngo 1.22\n",
	"cmd/main.go":            "package main\n\nimport \"example.com/shop/internal/core\"\n\nfunc main() { core.Run() }\n",
	"internal/core/core.go":  "package core\n\nfunc Run() {}\n",
	policy3.PolicyFile:       archfit3Policy,
	policy3.AppFile:          "version: 1\nowners: [alexei-led]\ngate: required\n",
	policy3.BaselineFile:     `{"schema_version": "archfit.baseline.v3", "accepted": []}`,
	policy3.LabelsFile:       "version: 1\nlabels: []\n",
	draftFile:                "volatility: {}\n",
	policy3.AgentsFile:       "# Agents\n\n<!-- archfit:start -->\nRun archfit analyze.\n<!-- archfit:end -->\n",
	"internal/core/notes.md": "Not a source file.\n",
}

// realFacts reads the head facts with the configuration Convert gives.
func realFacts(cfg checkfacts.Config) policy3.HeadFacts {
	return &checkfacts.Acquirer{Runner: toolrun.New(), Config: cfg}
}

func convertWith(root string, facts func(checkfacts.Config) policy3.HeadFacts) (policy3.Conversion, error) {
	return policy3.Convert(context.Background(), policy3.ConvertDeps{Runner: toolrun.New(), Facts: facts}, root)
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return err == nil
}

func TestConvertReplacesTheArchfit3Files(t *testing.T) {
	root := repo(t, archfit3Repo, origin)
	conv, err := convertWith(root, realFacts)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	data := readFile(t, filepath.Join(root, policy3.PolicyFile))
	p, err := policyfile.Resolve(data)
	if err != nil {
		t.Fatalf("the converted policy does not load: %v\n%s", err, data)
	}
	if p.Gate != "required" || !reflect.DeepEqual(p.Owners, []string{"alexei-led"}) {
		t.Errorf("gate %q, owners %v: want the App file's", p.Gate, p.Owners)
	}
	if len(p.Rules) != 1 || p.Rules[0].ID != "no-core-to-cmd" || p.Rules[0].Instead == "" {
		t.Errorf("rules = %+v, want the deny rule no-core-to-cmd", p.Rules)
	}
	if want := []string{policy3.AppFile, policy3.BaselineFile, policy3.LabelsFile}; !reflect.DeepEqual(conv.Removed, want) {
		t.Errorf("Removed = %v, want %v", conv.Removed, want)
	}
	for _, name := range []string{policy3.AppFile, policy3.BaselineFile, policy3.LabelsFile} {
		if exists(t, filepath.Join(root, name)) {
			t.Errorf("%s is still in the tree", name)
		}
	}
	if !exists(t, filepath.Join(root, draftFile)) {
		t.Errorf("%s is deleted: a draft may hold work nobody committed", draftFile)
	}
	agents := readFile(t, filepath.Join(root, policy3.AgentsFile))
	if want := "# Agents\n\n" + policy3.Block; string(agents) != want {
		t.Errorf("AGENTS.md =\n%s\nwant\n%s", agents, want)
	}
	if !strings.Contains(conv.Text(), "modules.*.owner: dropped (1 modules)") {
		t.Errorf("the text does not name the dropped owner key:\n%s", conv.Text())
	}
}

func TestConvertWritesNothing(t *testing.T) {
	policyV3 := "version: 3\nowners: [o]\nmodules:\n  app:\n    paths: [cmd/**]\n"
	tests := []struct {
		name    string
		files   map[string]string
		facts   func(checkfacts.Config) policy3.HeadFacts
		wantErr error
		wantMsg string
	}{
		{name: "no policy", files: map[string]string{"cmd/main.go": "package main\n"}, wantErr: policy3.ErrNotArchfit3},
		{name: "a schema 3 policy", files: map[string]string{policy3.PolicyFile: policyV3}, wantErr: policy3.ErrNotArchfit3},
		{name: "a policy that is not YAML", files: map[string]string{policy3.PolicyFile: "version: [2\n"}, wantMsg: policy3.PolicyFile},
		{
			name:    "a policy that does not load",
			files:   map[string]string{policy3.PolicyFile: strings.Replace(archfit3Policy, "layer: cmd", "layer: edge", 1)},
			wantMsg: "does not load",
		},
		{
			name:  "a missing tool",
			files: map[string]string{policy3.PolicyFile: archfit3Policy},
			facts: func(checkfacts.Config) policy3.HeadFacts {
				return fixed{Unknown: []evidence.UnknownItem{{Cause: evidence.CauseTool, Reason: "ast-grep is not installed", Fix: "install ast-grep"}}}
			},
			wantMsg: "install ast-grep",
		},
		{
			name:    "unbalanced markers in AGENTS.md",
			files:   map[string]string{policy3.PolicyFile: archfit3Policy, policy3.AgentsFile: "<!-- archfit:end -->\n"},
			wantMsg: "unbalanced",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{}
			for name, content := range archfit3Repo {
				if name != policy3.PolicyFile && name != policy3.AgentsFile {
					files[name] = content
				}
			}
			for name, content := range tt.files {
				files[name] = content
			}
			root := repo(t, files, origin)
			facts := tt.facts
			if facts == nil {
				facts = realFacts
			}
			_, err := convertWith(root, facts)
			switch {
			case err == nil:
				t.Fatal("Convert succeeded")
			case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			case tt.wantErr == nil && errors.Is(err, policy3.ErrNotArchfit3):
				t.Fatalf("error = %v, want an error that is not ErrNotArchfit3", err)
			case tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg):
				t.Fatalf("error = %v, want %q", err, tt.wantMsg)
			}
			for name, content := range files {
				if got := readFile(t, filepath.Join(root, name)); !bytes.Equal(got, []byte(content)) {
					t.Errorf("%s changed", name)
				}
			}
			if _, ok := files[policy3.AgentsFile]; !ok && exists(t, filepath.Join(root, policy3.AgentsFile)) {
				t.Errorf("%s written", policy3.AgentsFile)
			}
		})
	}
}

func TestConvertRestoresThePolicyWhenAgentsFails(t *testing.T) {
	root := repo(t, archfit3Repo, origin)
	agents := filepath.Join(root, policy3.AgentsFile)
	if err := os.Chmod(agents, 0o400); err != nil {
		t.Fatal(err)
	}
	if f, err := os.OpenFile(agents, os.O_WRONLY, 0); err == nil { //nolint:gosec // the test's own file
		_ = f.Close()
		t.Skip("the file stays writable (running as root)")
	}
	if _, err := convertWith(root, realFacts); err == nil {
		t.Fatal("Convert succeeded with an AGENTS.md it cannot write")
	}
	if got := readFile(t, filepath.Join(root, policy3.PolicyFile)); string(got) != archfit3Policy {
		t.Errorf("the Archfit 3 policy is not back:\n%s", got)
	}
	if !exists(t, filepath.Join(root, policy3.AppFile)) {
		t.Errorf("%s removed", policy3.AppFile)
	}
}
