package policy3_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/assessment/copies"
	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition/checkfacts"
	"github.com/alexei-led/archfit/v3/internal/initcfg/policy3"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	origin     = "git@github.com:acme/shop.git"
	copyText   = "the order could not be saved"
	gitattrs   = ".gitattributes"
	mainGo     = "main.go"
	appMain    = "src/app/main.ts"
	unbalanced = "unbalanced"
)

func requireTool(t *testing.T, tool string) {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Skip(tool + " not installed")
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...) //nolint:gosec // fixed binary, test args
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// repo commits files in a new repository with remote as origin ("" for none)
// and returns its root.
func repo(t *testing.T, files map[string]string, remote string) string {
	t.Helper()
	requireTool(t, "git")
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	if remote != "" {
		gitIn(t, root, "remote", "add", "origin", remote)
	}
	writeFiles(t, root, files)
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-qm", "code")
	return root
}

// withSources adds a file for each source to files: a package clause for Go,
// empty otherwise.
func withSources(sources []string, files map[string]string) map[string]string {
	out := map[string]string{}
	for _, s := range sources {
		out[s] = ""
		if strings.HasSuffix(s, ".go") {
			out[s] = "package x\n"
		}
	}
	for name, content := range files {
		out[name] = content
	}
	return out
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // a test file
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// recorder keeps the facts Init read.
type recorder struct {
	inner policy3.HeadFacts
	facts evidence.CheckFacts
}

func (r *recorder) Head(ctx context.Context, root string) (evidence.CheckFacts, error) {
	f, err := r.inner.Head(ctx, root)
	r.facts = f
	return f, err
}

// fixed gives the same facts for any tree.
type fixed evidence.CheckFacts

func (f fixed) Head(context.Context, string) (evidence.CheckFacts, error) {
	return evidence.CheckFacts(f), nil
}

func initWith(t *testing.T, facts policy3.HeadFacts, root string) (policy3.Result, error) {
	t.Helper()
	return policy3.Init(context.Background(), policy3.Deps{Runner: toolrun.New(), Facts: facts}, root)
}

var goRepo = map[string]string{
	gitattrs: "api/** linguist-generated=true\n",
	"go.mod": "module example.com/shop\n\ngo 1.22\n",
	mainGo:   "package main\n\nimport \"example.com/shop/internal/app\"\n\nfunc main() { app.Run() }\n",
	"cmd/shopd/main.go": "package main\n\nimport (\n\t\"example.com/shop/internal/app\"\n\t\"example.com/shop/internal/pay/card\"\n)\n\n" +
		"func main() { app.Run(); card.Charge() }\n",
	"internal/app/app.go": "package app\n\nimport (\n\t\"net/http\"\n\n\tv1 \"example.com/shop/api/v1\"\n" +
		"\t\"example.com/shop/internal/gen\"\n\t\"example.com/shop/internal/pay/bank\"\n\t\"example.com/shop/internal/store\"\n)\n\n" +
		"const failed = \"" + copyText + "\"\n\nconst Version = 1\n\n" +
		"func Run() { _ = http.StatusOK; _ = v1.Order{}; _ = gen.Kind(0); bank.Pay(); store.Save(); _ = failed }\n",
	"internal/app/app_test.go": "package app\n\nimport (\n\t\"testing\"\n\n\t\"example.com/shop/internal/store/sql\"\n)\n\n" +
		"func TestRun(t *testing.T) { sql.Open() }\n",
	"internal/store/store.go": "package store\n\nimport (\n\t\"example.com/shop/internal/app\"\n\t\"example.com/shop/internal/store/sql\"\n)\n\n" +
		"func Save() { _ = app.Version; sql.Open() }\n",
	"internal/store/sql/sql.go":   "package sql\n\nimport \"database/sql\"\n\nfunc Open() { _ = sql.ErrNoRows }\n",
	"internal/pay/card/card.go":   "package card\n\nfunc Charge() {}\n",
	"internal/pay/bank/bank.go":   "package bank\n\nfunc Pay() {}\n",
	"internal/util/text.go":       "package util\n\nconst message = \"" + copyText + "\"\n\nvar _ = message\n",
	"internal/gen/kind.go":        "// Code generated by stringer. DO NOT EDIT.\n\npackage gen\n\ntype Kind int\n",
	"api/v1/types.go":             "package v1\n\ntype Order struct{}\n",
	"internal/pay/bank/README.md": "Not a source file.\n",
}

var tsRepo = map[string]string{
	gitattrs:       "src/generated/** linguist-generated\n",
	"package.json": "{\"name\": \"web\", \"private\": true}\n",
	"index.ts":     "import { title } from \"./src/app/main\";\n\nconsole.log(title);\n",
	appMain: "import axios from \"axios\";\nimport { save } from \"../store/db\";\nimport { format } from \"../util/format\";\n\n" +
		"export const title = \"" + copyText + "\";\n\nsave();\nformat();\naxios.get(\"/\");\n",
	"src/app/main.test.ts": "import { title } from \"./main\";\n\nconsole.log(title);\n",
	"src/store/db.ts": "import { Client } from \"pg\";\nimport { schema } from \"../generated/schema\";\nimport { log } from \"../util/log\";\n\n" +
		"export function save(): void {\n  log(schema, Client);\n}\n",
	"src/util/format.ts":      "export function format(): void {}\n",
	"src/util/log.ts":         "export const message = \"" + copyText + "\";\n\nexport function log(...args: unknown[]): void {}\n",
	"src/generated/schema.ts": "export const schema = {};\n",
}

var pyRepo = map[string]string{
	"pyproject.toml":                   "[project]\nname = \"shop\"\n",
	"src/shop/__init__.py":             "",
	"src/shop/cli.py":                  "from shop.orders import service\n\nservice.place()\n",
	"src/shop/orders/__init__.py":      "",
	"src/shop/orders/service.py":       "import requests\n\nfrom shop.billing import invoice\n\nMESSAGE = \"" + copyText + "\"\n\n\ndef place():\n    invoice.bill(requests)\n",
	"src/shop/billing/__init__.py":     "",
	"src/shop/billing/invoice.py":      "import sqlite3\n\nfrom shop.orders import service\n\n\ndef bill(x):\n    return sqlite3, service\n",
	"src/shop/billing/tax/__init__.py": "",
	"src/shop/billing/tax/rates.py":    "RATE = 1\n",
	"src/shop/util/__init__.py":        "",
	"src/shop/util/text.py":            "TEXT = \"" + copyText + "\"\n",
	"tests/conftest.py":                "",
	"tests/test_orders.py":             "from shop.orders import service\n\n\ndef test_place():\n    service.place()\n",
}

// TestInitGolden writes the policy of a Go, a TypeScript and a Python
// repository. Each policy loads, fits its tree, and has an exception for
// every error it reports on the head, so the first check passes.
// ARCHFIT_UPDATE_INIT=1 rewrites the golden files.
func TestInitGolden(t *testing.T) {
	for _, tt := range []struct {
		name  string
		files map[string]string
		tools []string
	}{
		{"go", goRepo, nil},
		{"typescript", tsRepo, []string{"ast-grep"}},
		{"python", pyRepo, []string{"ast-grep"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, tool := range tt.tools {
				requireTool(t, tool)
			}
			root := repo(t, tt.files, origin)
			facts := &recorder{inner: &checkfacts.Acquirer{Runner: toolrun.New(), Config: policy3.FactsConfig()}}
			res, err := initWith(t, facts, root)
			if err != nil {
				t.Fatalf("Init: %v", err)
			}
			if len(res.Unknown) > 0 {
				t.Errorf("unknown items: %+v", res.Unknown)
			}
			data := readFile(t, filepath.Join(root, policy3.PolicyFile))
			pinGolden(t, filepath.Join("testdata", tt.name+".golden.yaml"), data)

			p, err := policyfile.Resolve(data)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if err := p.CheckTree(facts.facts.Sources); err != nil {
				t.Errorf("CheckTree: %v", err)
			}
			for _, e := range headErrors(p, facts.facts) {
				if !excepted(p.Exceptions, e) {
					t.Errorf("no exception for %+v at %v", e.Key, e.Sites)
				}
			}
			if got := readFile(t, filepath.Join(root, policy3.AgentsFile)); string(got) != policy3.Block {
				t.Errorf("AGENTS.md = %q, want the block", got)
			}
		})
	}
}

func headErrors(p resolved.Policy, facts evidence.CheckFacts) []change.Error {
	return append(change.Evaluate(p, facts).Errors, copies.Find(facts.Literals, p, nil, time.Time{})...)
}

// excepted reports whether an exception names the rule and the place of e.
func excepted(exceptions []resolved.Exception, e change.Error) bool {
	for _, x := range exceptions {
		from := e.Key.From
		if e.Key.Rule == resolved.RuleUnowned {
			from = e.Key.Symbol
		}
		if x.Rule == e.Key.Rule && x.From == from && x.To == e.Key.To && x.Expires.IsZero() {
			return true
		}
	}
	return false
}

func pinGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if os.Getenv("ARCHFIT_UPDATE_INIT") == "1" {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // a golden file of this package
	if err != nil {
		t.Fatalf("read %s: %v (ARCHFIT_UPDATE_INIT=1 writes it)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s drifted; review and rerun with ARCHFIT_UPDATE_INIT=1\n got:\n%s\nwant:\n%s", path, got, want)
	}
}

var goFacts = fixed{
	Sources: []string{"go.go", "pkg/a/a.go"},
	Imports: []evidence.ImportFact{{File: "go.go", Line: 3, Target: "pkg/a"}},
}

// TestInitNeverOverwritesAPolicy: an existing policy, of any version, stays
// byte for byte, and nothing else is written.
func TestInitNeverOverwritesAPolicy(t *testing.T) {
	for name, content := range map[string]string{
		"version 3": "version: 3\nowners: [acme]\nmodules:\n  app:\n    paths: [\"**\"]\n",
		"version 2": "version: 2\nmodules: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := repo(t, withSources(goFacts.Sources, map[string]string{policy3.PolicyFile: content}), origin)
			_, err := initWith(t, goFacts, root)
			if !errors.Is(err, policy3.ErrPolicyExists) {
				t.Fatalf("Init error = %v, want ErrPolicyExists", err)
			}
			if got := readFile(t, filepath.Join(root, policy3.PolicyFile)); string(got) != content {
				t.Errorf("policy changed to %q", got)
			}
			if _, err := os.Stat(filepath.Join(root, policy3.AgentsFile)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("AGENTS.md written: %v", err)
			}
		})
	}
}

// TestInitWritesTheBlock: the block goes between the markers, or after the
// text; every byte outside the markers stays.
func TestInitWritesTheBlock(t *testing.T) {
	const oldBlock = "<!-- archfit:start -->\nold archfit lines\n<!-- archfit:end -->\n"
	for _, tt := range []struct {
		name, agents, want string
	}{
		{"no file", "", policy3.Block},
		{"no markers, no final newline", "# Notes\nkeep me", "# Notes\nkeep me\n\n" + policy3.Block},
		{"no markers", "# Notes\n", "# Notes\n\n" + policy3.Block},
		{"markers in the middle", "# Notes\n\n" + oldBlock + "\n## More\ntail", "# Notes\n\n" + policy3.Block + "\n## More\ntail"},
		{"end marker at the end of the file", "# Notes\n\n" + strings.TrimSuffix(oldBlock, "\n"),
			"# Notes\n\n" + strings.TrimSuffix(policy3.Block, "\n")},
		{"CRLF after the end marker", strings.TrimSuffix(oldBlock, "\n") + "\r\ntail\r\n",
			strings.TrimSuffix(policy3.Block, "\n") + "\r\ntail\r\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{}
			if tt.agents != "" {
				files[policy3.AgentsFile] = tt.agents
			}
			root := repo(t, withSources(goFacts.Sources, files), origin)
			if _, err := initWith(t, goFacts, root); err != nil {
				t.Fatalf("Init: %v", err)
			}
			if got := readFile(t, filepath.Join(root, policy3.AgentsFile)); string(got) != tt.want {
				t.Errorf("AGENTS.md =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestWriteBlock: the block alone, for a repository whose policy exists. It
// writes only when the file does not hold the block, and keeps every byte
// outside the markers.
func TestWriteBlock(t *testing.T) {
	const oldBlock = "<!-- archfit:start -->\nold archfit lines\n<!-- archfit:end -->\n"
	for _, tt := range []struct {
		name, agents, want string
		absent, wrote      bool
	}{
		{name: "no file", absent: true, want: policy3.Block, wrote: true},
		{name: "an empty file", want: policy3.Block, wrote: true},
		{name: "no markers", agents: "# Notes\n", want: "# Notes\n\n" + policy3.Block, wrote: true},
		{name: "an old block", agents: "# Notes\n\n" + oldBlock + "tail\n", want: "# Notes\n\n" + policy3.Block + "tail\n", wrote: true},
		{name: "the block already", agents: "# Notes\n\n" + policy3.Block + "tail\n", want: "# Notes\n\n" + policy3.Block + "tail\n"},
		{name: "the block at the end without a newline", agents: strings.TrimSuffix(policy3.Block, "\n"), want: strings.TrimSuffix(policy3.Block, "\n")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, policy3.AgentsFile)
			if !tt.absent {
				if err := os.WriteFile(path, []byte(tt.agents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			wrote, err := policy3.WriteBlock(root)
			if err != nil {
				t.Fatalf("WriteBlock: %v", err)
			}
			if wrote != tt.wrote {
				t.Errorf("wrote = %v, want %v", wrote, tt.wrote)
			}
			if got := readFile(t, path); string(got) != tt.want {
				t.Errorf("AGENTS.md =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}

	root := t.TempDir()
	unbalanced := "<!-- archfit:start -->\nno end\n"
	if err := os.WriteFile(filepath.Join(root, policy3.AgentsFile), []byte(unbalanced), 0o600); err != nil {
		t.Fatal(err)
	}
	if wrote, err := policy3.WriteBlock(root); err == nil || wrote {
		t.Errorf("unbalanced markers: wrote %v, err %v; want an error and nothing written", wrote, err)
	}
	if got := readFile(t, filepath.Join(root, policy3.AgentsFile)); string(got) != unbalanced {
		t.Errorf("unbalanced markers changed the file: %q", got)
	}
}

// TestInitWritesNothingOnAnError: unbalanced markers, a missing origin, an
// origin outside GitHub and a missing tool stop Init before it writes.
func TestInitWritesNothingOnAnError(t *testing.T) {
	missingTool := fixed{Sources: goFacts.Sources, Unknown: []evidence.UnknownItem{{
		Reason: "ast-grep is not installed; it reads the literals of typescript", Cause: evidence.CauseTool,
		Fix: "install ast-grep: https://example.com",
	}}}
	for _, tt := range []struct {
		name   string
		agents string
		remote string
		facts  fixed
		want   string
	}{
		{"unbalanced markers", "<!-- archfit:start -->\nno end\n", origin, goFacts, unbalanced},
		{"end marker first", "<!-- archfit:end -->\n<!-- archfit:start -->\n", origin, goFacts, unbalanced},
		{"a second start marker", "<!-- archfit:start -->\na\n<!-- archfit:end -->\n<!-- archfit:start -->\n", origin, goFacts, unbalanced},
		{"no origin", "", "", goFacts, "no origin remote"},
		{"origin outside GitHub", "", "https://gitlab.com/acme/shop.git", goFacts, "on gitlab.com"},
		{"missing tool", "", origin, missingTool, "next: install ast-grep"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{}
			if tt.agents != "" {
				files[policy3.AgentsFile] = tt.agents
			}
			root := repo(t, withSources(tt.facts.Sources, files), tt.remote)
			_, err := initWith(t, tt.facts, root)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Init error = %v, want one that says %q", err, tt.want)
			}
			if _, err := os.Stat(filepath.Join(root, policy3.PolicyFile)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("policy written: %v", err)
			}
			//nolint:gosec // a file of the test repository
			if got, err := os.ReadFile(filepath.Join(root, policy3.AgentsFile)); err == nil && string(got) != tt.agents {
				t.Errorf("AGENTS.md changed to %q", got)
			}
		})
	}
}

// TestInitOwnsEverySourceFile: on a tree of mixed layouts every source file
// outside the tests is in exactly one module, and the policy fits the tree.
func TestInitOwnsEverySourceFile(t *testing.T) {
	sources := []string{
		mainGo, "setup.py",
		"cmd/tool.go", "cmd/archfit/main.go",
		"internal/x/x.go", "x/y.go",
		"src/shop/__init__.py", "src/shop/a/b.py", "src/shop/a/c/d.ts", "src/shop/e/deep/f.py",
		"web/ui/button.ts", "web/ui/form/input.ts",
		"app/layout.tsx", "app/[slug]/page.tsx", "app/{group}/view.tsx",
		"tests/test_a.py", "pkg/a/a_test.go",
	}
	facts := fixed{Sources: sources, Imports: []evidence.ImportFact{
		{File: "web/ui/button.ts", Line: 1, Target: "app/[slug]/page.tsx"},
	}}
	root := repo(t, withSources(sources, nil), origin)
	res, err := initWith(t, facts, root)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	p, err := policyfile.Resolve(readFile(t, filepath.Join(root, policy3.PolicyFile)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range sources {
		m, ok := p.ModuleOf(f)
		if test := p.IsTest(f); ok == test {
			t.Errorf("%s: module %q, test %v; want a module exactly when it is not a test", f, m, test)
		}
	}
	if m, _ := p.ModuleOf("app/[slug]/page.tsx"); !p.IsPublic(m, "app/[slug]/page.tsx", true) {
		t.Errorf("app/[slug]/page.tsx is used by web_ui but not public in %s: %v", m, p.Modules[m].Public)
	}
	if len(res.File.Exceptions) > 0 {
		t.Errorf("exceptions on a tree with one allowed import: %+v", res.File.Exceptions)
	}
}

// TestInitNamesTheRootModuleRoot: the root module keeps its name when other
// modules shorten to it.
func TestInitNamesTheRootModuleRoot(t *testing.T) {
	sources := []string{mainGo, "root/a.go", "internal/root/b.go"}
	root := repo(t, withSources(sources, nil), origin)
	res, err := initWith(t, fixed{Sources: sources}, root)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	want := map[string]string{"root": "*.go", "root_2": "root/**", "internal_root": "internal/root/**"}
	for name, glob := range want {
		if got := res.File.Modules[name].Paths; len(got) != 1 || got[0] != glob {
			t.Errorf("module %s paths = %v, want [%s]; modules: %v", name, got, glob, res.File.Modules)
		}
	}
}

// TestInitGeneratedFollowsGitattributes: the last .gitattributes line that
// matches a path decides, as in git, and only the sources it marks are out
// of the rules.
func TestInitGeneratedFollowsGitattributes(t *testing.T) {
	const genFile = "src/gen/a.ts"
	for _, tt := range []struct {
		name, attributes string
		sources, want    []string
	}{
		{"a later line unsets", "src/** linguist-generated\nsrc/manual/** -linguist-generated\n",
			[]string{genFile, "src/manual/b.ts"}, []string{genFile}},
		{"a later line sets again", "gen/** linguist-generated\ngen/keep.ts linguist-generated=false\ngen/** linguist-generated\n",
			[]string{"gen/a.ts", "gen/keep.ts", "lib/c.ts"}, []string{"gen/**"}},
		{"a pattern without a slash", "*.pb.ts linguist-generated=true\n",
			[]string{"api/x.pb.ts", "api/y.ts"}, []string{"**/*.pb.ts"}},
		{"a quoted pattern unsets", "src/** linguist-generated\n\"src/manual code/**\" -linguist-generated\n",
			[]string{genFile, "src/manual code/b.ts"}, []string{genFile}},
		{"a macro line names no path", "[attr]gen -linguist-generated\nsrc/** linguist-generated\n",
			[]string{"src/a.ts"}, []string{"src/**"}},
		{"a generated name with a bracket", "src/** linguist-generated\nsrc/b.ts -linguist-generated\n",
			[]string{"src/[id].ts", "src/b.ts"}, []string{`src/\[id\].ts`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := repo(t, withSources(tt.sources, map[string]string{gitattrs: tt.attributes}), origin)
			res, err := initWith(t, fixed{Sources: tt.sources}, root)
			if err != nil {
				t.Fatalf("Init: %v", err)
			}
			if got := res.File.Generated; !slices.Equal(got, tt.want) {
				t.Errorf("generated = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestInitRemovesThePolicyWhenTheBlockFails: when AGENTS.md cannot be written
// the policy goes too, so a second run is not stopped by it.
func TestInitRemovesThePolicyWhenTheBlockFails(t *testing.T) {
	root := repo(t, withSources(goFacts.Sources, nil), origin)
	if err := os.Symlink(filepath.Join(root, "missing", "AGENTS.md"), filepath.Join(root, policy3.AgentsFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := initWith(t, goFacts, root); err == nil {
		t.Fatal("Init wrote through a link to a missing directory")
	}
	if _, err := os.Stat(filepath.Join(root, policy3.PolicyFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("policy left behind: %v", err)
	}
}

// TestInitPublicOfAFileThatIsNotASource: an image in a directory with no
// source makes the nearest source directory public, so the policy fits the
// tree, and the use past it is an exception.
func TestInitPublicOfAFileThatIsNotASource(t *testing.T) {
	facts := fixed{
		Sources: []string{appMain, "src/ui/button.ts"},
		Imports: []evidence.ImportFact{{File: appMain, Line: 1, Target: "src/ui/icons/logo.svg"}},
	}
	root := repo(t, withSources(facts.Sources, map[string]string{"src/ui/icons/logo.svg": "<svg/>"}), origin)
	res, err := initWith(t, facts, root)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := res.File.Modules["ui"].Public; len(got) != 1 || got[0] != "src/ui" {
		t.Errorf("ui public = %v, want [src/ui]", got)
	}
	want := policyfile.Exception{Rule: resolved.RuleCoupling, From: "app", To: "ui", Why: "The error existed when archfit init wrote this policy."}
	if len(res.File.Exceptions) != 1 || res.File.Exceptions[0] != want {
		t.Errorf("exceptions = %+v, want [%+v]", res.File.Exceptions, want)
	}
}

// TestInitStopsOnAnUnreadableGitattributesLine: a quoted pattern init cannot
// read, on a line that names linguist-generated, stops Init: skipping it
// could mark hand-written code generated.
func TestInitStopsOnAnUnreadableGitattributesLine(t *testing.T) {
	for name, line := range map[string]string{
		"no closing quote": "\"src/manual/** -linguist-generated\n",
		"a bad escape":     "\"src/\\q/**\" -linguist-generated\n",
	} {
		t.Run(name, func(t *testing.T) {
			sources := []string{"src/manual/a.ts"}
			root := repo(t, withSources(sources, map[string]string{gitattrs: "src/** linguist-generated\n" + line}), origin)
			_, err := initWith(t, fixed{Sources: sources}, root)
			if err == nil || !strings.Contains(err.Error(), ".gitattributes line 2") {
				t.Fatalf("Init error = %v, want one that names .gitattributes line 2", err)
			}
			if _, err := os.Stat(filepath.Join(root, policy3.PolicyFile)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("policy written: %v", err)
			}
		})
	}
}
