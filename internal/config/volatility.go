package config

import (
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Volatility band constants (the ModuleDef.Volatility values classify understands).
const (
	volatilityHigh   = "high"
	volatilityMedium = "medium"
	volatilityLow    = "low"
)

// DeriveVolatility computes a relative volatility band per module from per-file git
// churn (commit counts). A file is attributed to the first module (alphabetical)
// whose paths glob matches it; Python files are also matched in dotted-module form,
// since Python module globs are dotted while churn paths are slash files. Modules
// are banded against the repo's busiest module: ≥2/3 of max churn → high, ≥1/3 →
// medium, else low. Returns nil when there is no churn signal, leaving volatility
// unknown (so unbalanced_edge reports n/a rather than guessing).
func DeriveVolatility(modules map[string]ModuleDef, churn map[string]int) map[string]string {
	if len(churn) == 0 || len(modules) == 0 {
		return nil
	}
	names := make([]string, 0, len(modules))
	for n := range modules {
		names = append(names, n)
	}
	sort.Strings(names)

	modChurn := make(map[string]int)
	for file, n := range churn {
		cands := []string{file}
		if strings.HasSuffix(file, ".py") {
			cands = append(cands, pyFileModule(file))
		}
		if owner := ownerModule(names, modules, cands); owner != "" {
			modChurn[owner] += n
		}
	}

	maxC := 0
	for _, c := range modChurn {
		if c > maxC {
			maxC = c
		}
	}
	if maxC == 0 {
		return nil
	}

	vol := make(map[string]string, len(modules))
	for _, name := range names {
		c := modChurn[name]
		switch {
		case c*3 >= maxC*2:
			vol[name] = volatilityHigh
		case c*3 >= maxC:
			vol[name] = volatilityMedium
		default:
			vol[name] = volatilityLow
		}
	}
	return vol
}

// ApplyVolatility records the churn-derived volatility band for modules that
// do not declare an explicit volatility or subdomain signal, so hand-authored
// config always wins. Derived values go into a separate store — c.Modules is
// never mutated — so the classify view sees them (effectiveModules) while
// consumers of hand-authored volatility (risk_hub, enrich context) cannot,
// regardless of when this is called.
func (c *Config) ApplyVolatility(vol map[string]string) {
	for name, v := range vol {
		def, ok := c.Modules[name]
		if !ok || def.Volatility != "" || def.Subdomain != "" {
			continue
		}
		if c.derivedVolatility == nil {
			c.derivedVolatility = make(map[string]string, len(vol))
		}
		c.derivedVolatility[name] = v
	}
}

// effectiveModules returns the module definitions with churn-derived
// volatility filled in where no explicit volatility/subdomain exists.
// It returns a copy when derived values are present; c.Modules itself
// always stays hand-authored-only.
func (c Config) effectiveModules() map[string]ModuleDef {
	if len(c.derivedVolatility) == 0 {
		return c.Modules
	}
	out := make(map[string]ModuleDef, len(c.Modules))
	for name, def := range c.Modules {
		if v, ok := c.derivedVolatility[name]; ok {
			def.Volatility = v
		}
		out[name] = def
	}
	return out
}

// ownerModule returns the first module (in the given sorted order) whose paths glob
// matches any of the candidate paths.
func ownerModule(names []string, modules map[string]ModuleDef, candidates []string) string {
	for _, name := range names {
		for _, glob := range modules[name].Paths {
			for _, cand := range candidates {
				if matched, _ := doublestar.Match(glob, cand); matched {
					return name
				}
			}
		}
	}
	return ""
}

// pyFileModule converts a Python file path to its dotted module path
// (src/ccgram/handlers/x.py → ccgram.handlers.x), matching dotted module globs.
func pyFileModule(file string) string {
	p := strings.TrimSuffix(file, ".py")
	p = strings.TrimPrefix(p, "src/")
	p = strings.TrimSuffix(p, "/__init__")
	return strings.ReplaceAll(p, "/", ".")
}
