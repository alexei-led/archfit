package classify

import (
	"testing"

	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// TestModuleVocabularyMatchesPolicyValidation pins that the values config lint
// and check reject as unknown (policy.ModuleDef.UnknownVolatility and
// UnknownSubdomain) are exactly the values classification ignores. A value
// classification reads must never be reported unknown, and an unknown value
// must never silently classify.
func TestModuleVocabularyMatchesPolicyValidation(t *testing.T) {
	for _, value := range []string{
		"high", "High", "medium", "low", "frozen", "legacy", "LEGACY",
		subdomainCore, "Core", "supporting", "generic",
		"hgih", "critical", "cor", "domain", "undeclared",
	} {
		for _, def := range []policy.ModuleDef{
			{Paths: []string{"m/**"}, Volatility: value},
			{Paths: []string{"m/**"}, Subdomain: value},
		} {
			modules := map[string]policy.ModuleDef{"m": def}
			read := classifyVolatility("m/x", buildModuleIndex(modules), modules) != coupling.VolatilityUndeclared
			if unknown := def.UnknownVolatility() || def.UnknownSubdomain(); read == unknown {
				t.Errorf("%+v: classification reads it = %v, policy reports it unknown = %v", def, read, unknown)
			}
		}
	}
}
