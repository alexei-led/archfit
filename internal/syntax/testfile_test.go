package syntax_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/syntax"
)

const (
	langGo         = graph.LangGo
	langPython     = graph.LangPython
	langTypeScript = graph.LangTypeScript
	langRust       = graph.LangRust
	langDotNet     = graph.LangDotNet
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
		{langRust, "src/types/tests.rs", true},
		{langRust, "src/tests.rs", true},
		{langRust, "src/parser_test.rs", true},
		{langRust, "src/parser_tests.rs", true},
		{langRust, "src/types/property_tests/setup.rs", true},
		{langRust, "src/types/property-tests/setup.rs", true},
		{langRust, "benches/throughput.rs", true},
		{langRust, "crates/ruff/benches/parser.rs", true},
		{langRust, "src/contests/score.rs", false},     // "tests" suffix without a separator
		{langRust, "src/latests.rs", false},            // not tests.rs, *_test.rs, or *_tests.rs
		{langRust, "src/testing.rs", false},            // helper module, not a test file
		{langRust, "src/test.rs", false},               // not in the conservative list
		{langRust, "src/attests/check.rs", false},      // "tests" suffix without a separator
		{langRust, "src/bench/mod.rs", false},          // only the benches/ target directory
		{langPython, "pkg/property_tests/x.py", false}, // the Rust directory forms stay Rust-only
		{langGo, "pkg/tests.go", false},                // the Rust file forms stay Rust-only
		// .NET
		{langDotNet, "src/Core/Services/UserService.cs", false},
		{langDotNet, "src/Core.Test/UserServiceTests.cs", true},
		{langDotNet, "src/Core/UserServiceTest.cs", true},
		{langDotNet, "test/Core.Test/Helpers.cs", true},
		{langDotNet, "src/Core/tests/Fixture.cs", true},
		{langDotNet, "src/Core/Contest.cs", false},       // "test" in a word, not a suffix
		{langDotNet, "src/Core/Testing/Clock.cs", false}, // not a test/ or tests/ segment
		{langDotNet, "src/Core.Tests/Program.cs", false}, // project dir name alone is not a rule
		{langDotNet, "src/Core/Tests/Fixture.cs", true},  // .NET folders are PascalCase
		{langDotNet, "Test/Core/Helpers.cs", true},
		{langPython, "pkg/Tests/helpers.py", false}, // other languages keep their lowercase convention
		{langRust, "crate/Tests/helpers.rs", false},
		{langGo, "test/tool.go", false},              // the .NET segment rule stays .NET-only
		{langRust, "src/UserServiceTests.rs", false}, // the .NET file forms stay .NET-only
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
