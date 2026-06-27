package py

import "testing"

func TestHasPrivateSymbolImport(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		// Symbol-level private imports → intrusive.
		{"private symbol", "from pub import _priv", true},
		{"private symbol with alias", "from pub import _sym as s", true},
		{"multi-import one private", "from pub import a, _b, c", true},
		{"multi-import all private", "from pub import _a, _b", true},

		// NOT intrusive: alias is private but the imported name is public.
		{"public symbol aliased private", "from pub import thing as _local", false},

		// NOT intrusive: dunder names are runtime/package machinery, not internals.
		{"dunder import", "from pkg import __init__", false},
		{"dunder import2", "from pkg import __all__", false},

		// NOT intrusive: normal public symbol.
		{"public symbol", "from pub import thing", false},
		{"public multi-symbol", "from pub import a, b, c", false},

		// Plain "import x" form — no symbol, module-level rule applies.
		{"plain import", "import pub", false},
		{"plain import dotted", "import pub.sub", false},

		// Empty / whitespace — safe abstain.
		{"empty", "", false},
		{"whitespace", "   ", false},

		// Inline comment does not confuse parser.
		{"with comment", "from pub import thing  # _private", false},
		{"private with comment", "from pub import _sym  # alias", true},

		// Single-line parenthesized form.
		{"parens single-line", "from pub import (_priv)", true},
		{"parens single-line public", "from pub import (thing)", false},

		// Multi-line opening line only (line_contents captures the first physical line).
		// Safe to abstain — the ceiling is documented in hasPrivateSymbolImport.
		{"multiline open only", "from pub import (", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasPrivateSymbolImport(tt.line)
			if got != tt.want {
				t.Errorf("hasPrivateSymbolImport(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}
