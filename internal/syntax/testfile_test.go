package syntax_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/syntax"
)

const (
	langGo         = graph.LangGo
	langPython     = graph.LangPython
	langTypeScript = graph.LangTypeScript
	langRust       = graph.LangRust
)

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		lang string
		path string
		want bool
	}{
		// Go
		{langGo, "pkg/container/mock_client.go", false},
		{langGo, "pkg/container/client_test.go", true},
		{langGo, "internal/engine/engine_test.go", true},
		{langGo, "pkg/container/helper.go", false},
		{langGo, "test_helper.go", false}, // no _test.go suffix
		// Python
		{langPython, "billing/service.py", false},
		{langPython, "billing/test_service.py", true},
		{langPython, "billing/service_test.py", true},
		{langPython, "tests/billing.py", true},
		{langPython, "billing/tests/unit.py", true},
		{langPython, "billing/testutils.py", false},
		// TypeScript
		{langTypeScript, "src/service.ts", false},
		{langTypeScript, "src/service.test.ts", true},
		{langTypeScript, "src/service.spec.ts", true},
		{langTypeScript, "src/service.test.tsx", true},
		{langTypeScript, "src/service.spec.mts", true},
		{langTypeScript, "src/service.test.mjs", true},
		{langTypeScript, "src/__tests__/service.ts", true},
		{langTypeScript, "src/testService.ts", false},
		{langTypeScript, "src/service.mts", false},
		// Rust
		{langRust, "src/lib.rs", false},
		{langRust, "tests/integration.rs", true},
		{langRust, "src/tests/unit.rs", true},
		{langRust, "src/test_helper.rs", false}, // no /tests/ segment
		// Unknown language
		{"java", "Test.java", false},
	}

	for _, tc := range tests {
		t.Run(tc.lang+":"+tc.path, func(t *testing.T) {
			got := syntax.IsTestFile(tc.lang, tc.path)
			if got != tc.want {
				t.Errorf("IsTestFile(%q, %q) = %v, want %v", tc.lang, tc.path, got, tc.want)
			}
		})
	}
}
