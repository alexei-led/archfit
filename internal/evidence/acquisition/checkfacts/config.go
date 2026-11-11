package checkfacts

import (
	"fmt"
	"strconv"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// Config is what the check reads from the policy to read the facts.
type Config struct {
	// Exclude is the policy's exclude. The built-in exclusions (.git,
	// node_modules, vendor) are added by the registry.
	Exclude []string
	// Languages are the settings of each language by registry id. A language
	// that is absent or off is not read.
	Languages registry.Configs
	// Literals is false when the duplication rule is off: no rule reads them.
	Literals bool
}

// ConfigFrom returns the settings of the policy the check facts depend on:
// exclude, the languages block, and whether the duplication rule is on. It
// reads no "analyzers" key: the check runs no optional analyzer, so the facts
// are the same with every one of them on or off.
func ConfigFrom(p resolved.Policy) Config {
	s := settings(p.Settings)
	langs := registry.Configs{}
	for _, d := range registry.All() {
		prefix := "languages." + d.ID + "."
		langs[d.ID] = evidenceports.ExtractConfig{Src: ".", Mode: mode(s.get(prefix + "enabled"))}
	}
	set := func(id string, edit func(*evidenceports.ExtractConfig)) {
		cfg := langs[id]
		edit(&cfg)
		langs[id] = cfg
	}
	set("go", func(c *evidenceports.ExtractConfig) {
		c.GoModuleInclude = s.list("languages.go.modules.include")
		c.GoModuleExclude = s.list("languages.go.modules.exclude")
	})
	set("python", func(c *evidenceports.ExtractConfig) { c.PyPackage = s.get("languages.python.package") })
	set("rust", func(c *evidenceports.ExtractConfig) {
		c.CargoManifest = s.get("languages.rust.manifest")
		c.CargoFeatures = s.list("languages.rust.features")
		c.IncludeDevDeps, _ = strconv.ParseBool(s.get("languages.rust.include_dev_deps"))
	})
	set("dotnet", func(c *evidenceports.ExtractConfig) {
		c.DotNetProjects = s.list("languages.dotnet.projects")
		c.TypeGraphDisabled = true
	})
	return Config{Exclude: p.Exclude, Languages: langs, Literals: p.Builtin.Duplication != resolved.ModeOff}
}

// mode reads a languages.<id>.enabled value: false is off, true is on, and
// absent or auto is auto. In the check on reads as auto does: a language is
// read where its files exist, and a missing tool is unknown either way.
func mode(v string) evidenceports.ToolMode {
	switch v {
	case "false":
		return evidenceports.ModeOff
	case "true":
		return evidenceports.ModeOn
	default:
		return evidenceports.ModeAuto
	}
}

// settings are the flat policy settings ("languages.go.enabled" -> "true",
// "languages.rust.features[0]" -> "x").
type settings map[string]string

func (s settings) get(key string) string { return s[key] }

// list reads key[0], key[1], ... up to the first absent index.
func (s settings) list(key string) []string {
	var out []string
	for i := 0; ; i++ {
		v, ok := s[fmt.Sprintf("%s[%d]", key, i)]
		if !ok {
			return out
		}
		out = append(out, v)
	}
}
