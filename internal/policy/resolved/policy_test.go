package resolved_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

func TestDigest(t *testing.T) {
	smallest := "version: 3\nowners: [alexei-led]\nmodules:\n  app:\n    paths: [cmd/**, pkg/**]\n"
	const want = "sha256:c482e499dc88f1c036864461493fb14806607166ba068bed2e5513918efe0729"
	tests := []struct {
		name string
		file string
		same bool
	}{
		{"the same bytes", smallest, true},
		{"one byte more", smallest + "\n", false},
		{"the same policy written another way", strings.Replace(smallest, "[cmd/**, pkg/**]", "[cmd/**,pkg/**]", 1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolved.Digest([]byte(tt.file)); (got == want) != tt.same {
				t.Errorf("Digest = %q, same as %q: want %v", got, want, tt.same)
			}
		})
	}
}

func TestWithDefaultsKeepsDeclaredValues(t *testing.T) {
	p := resolved.Policy{
		Gate: resolved.GateRequired,
		Modules: map[string]resolved.Module{
			"app": {Paths: []string{"pkg/**"}, Public: []string{"pkg/api"}, Volatility: resolved.VolatilityLow, Subdomain: resolved.SubdomainCore},
		},
		Builtin: resolved.Builtin{Coupling: resolved.ModeOff, MinBalance: 9},
	}.WithDefaults()
	app := p.Modules["app"]
	if p.Gate != resolved.GateRequired || app.Volatility != resolved.VolatilityLow || app.Public[0] != "pkg/api" {
		t.Errorf("WithDefaults changed a declared value: %+v", p)
	}
	if p.Builtin.Coupling != resolved.ModeOff || p.Builtin.MinBalance != 9 || p.Builtin.Graph != resolved.ModeAsk {
		t.Errorf("Builtin = %+v", p.Builtin)
	}
}

func TestUses(t *testing.T) {
	tests := []struct {
		use        string
		lang       resolved.Language
		importPath string
		want       bool
	}{
		{resolved.SetProcess, resolved.LanguageGo, "os/exec", true},
		{resolved.SetProcess, resolved.LanguageGo, "os", false},
		{resolved.SetNetwork, resolved.LanguageGo, "net", true},
		{resolved.SetNetwork, resolved.LanguageGo, "net/url", false},
		{resolved.SetNetwork, resolved.LanguageGo, "net/http/httputil", true},
		{resolved.SetDatabase, resolved.LanguageGo, "github.com/jackc/pgx/v5", true},
		{resolved.SetDatabase, resolved.LanguageGo, "github.com/jackc/pgxpool", false},
		{resolved.SetNetwork, resolved.LanguagePython, "requests.adapters", true},
		{resolved.SetNetwork, resolved.LanguagePython, "http.cookies", false},
		{resolved.SetProcess, resolved.LanguagePython, "subprocess", true},
		{resolved.SetProcess, resolved.LanguageTypeScript, "node:child_process", true},
		{resolved.SetProcess, resolved.LanguageTypeScript, "child_process", true},
		{resolved.SetNetwork, resolved.LanguageTypeScript, "http2", true},
		{resolved.SetNetwork, resolved.LanguageTypeScript, "httpx", false},
		{resolved.SetDatabase, resolved.LanguageTypeScript, "@prisma/client", true},
		{resolved.SetNetwork, resolved.LanguageRust, "std::net::TcpStream", true},
		{resolved.SetProcess, resolved.LanguageRust, "std::processing", false},
		{resolved.SetNetwork, resolved.LanguageDotNet, "System.Net.Http", true},
		{resolved.SetDatabase, resolved.LanguageDotNet, "System.Data.SqlClient", true},
		{resolved.SetProcess, resolved.LanguageDotNet, "System.Diagnostics", false},
		// A library matches itself and what is below it.
		{"github.com/docker/docker", resolved.LanguageGo, "github.com/docker/docker/client", true},
		{"github.com/docker/docker", resolved.LanguageGo, "github.com/docker/dockerfile", false},
		{"requests", resolved.LanguagePython, "requests", true},
	}
	for _, tt := range tests {
		if got := resolved.Uses(tt.use, tt.lang, tt.importPath); got != tt.want {
			t.Errorf("Uses(%q, %s, %q) = %v, want %v", tt.use, tt.lang, tt.importPath, got, tt.want)
		}
	}
}

// TestImportsNoConfig keeps the model free of the file format: decoding is the
// job of internal/config/policyfile, which projects into this package.
func TestImportsNoConfig(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if strings.Contains(path, "/internal/") || strings.Contains(path, "yaml") {
				t.Errorf("%s imports %s: the policy model imports no config or other internal package", file, path)
			}
		}
	}
}
