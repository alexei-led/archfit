package acquisition

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/syntax"
)

// TestKeepsSyntaxFact pins which syntax facts reach the syntax surface: a route
// a test, generated, or vendored file registers is not a production entry
// point, while a declaration fact from such a file still reports.
func TestKeepsSyntaxFact(t *testing.T) {
	tests := []struct {
		name string
		fact evidence.SyntaxFact
		cfg  syntax.FileClassConfig
		want bool
	}{
		{name: "route in production source", fact: routeFact(graph.LangDotNet, "src/Api/Controllers/CiphersController.cs"), want: true},
		{name: "route in a test file", fact: routeFact(graph.LangDotNet, "test/Api.Test/Utilities/RouteTests.cs"), want: false},
		{name: "route in a generated file", fact: routeFact(graph.LangGo, "gen/api.pb.go"), want: false},
		{name: "route in a .NET build directory", fact: routeFact(graph.LangDotNet, "src/Api/obj/Debug/Program.cs"), want: false},
		{name: "route in a vendored file", fact: routeFact(graph.LangGo, "vendor/example.com/lib/route.go"), want: false},
		{name: "route matching a configured test glob", fact: routeFact(graph.LangGo, "e2e/server/routes.go"),
			cfg: syntax.FileClassConfig{TestGlobs: []string{"e2e/**"}}, want: false},
		{name: "declaration in a test file still reports", fact: evidence.SyntaxFact{Language: graph.LangGo, File: "pkg/api/handler_test.go", Kind: "function", Name: "TestHandle"}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			index := map[string]fileclass.FileClass{}
			if got := keepsSyntaxFact(tc.fact, index, tc.cfg); got != tc.want {
				t.Errorf("keepsSyntaxFact(%s) = %v, want %v", tc.fact.File, got, tc.want)
			}
		})
	}
}

func routeFact(lang, file string) evidence.SyntaxFact {
	return evidence.SyntaxFact{Language: lang, File: file, Kind: syntaxKindRoute, Name: "/probe"}
}
