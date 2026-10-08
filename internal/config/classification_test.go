package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition"
)

// leafClass says which comparison input a config leaf belongs to. Comparability
// reads model_hash, classification_hash and the measurement profile, never the
// raw file, so a leaf in none of them (governance) can change findings and
// what blocks but never makes a stored reference non-comparable.
type leafClass string

const (
	classModel          leafClass = "model"          // model_hash (resolved module map)
	classClassification leafClass = "classification" // classification_hash
	classProfile        leafClass = "profile"        // measurement profile settings
	classGovernance     leafClass = "governance"     // decides findings and gates, not facts
)

// leafClasses maps a config path to its class. A segment `*` matches one
// segment (a map key). The most specific entry wins, so a subtree default stays
// safe for modules because policy.TestModelHashCoversEveryModuleField fails on a
// ModuleDef field that is neither hashed nor deliberately governance. A new
// top-level key matches nothing and fails TestEveryConfigLeafHasOneClass, so it
// needs a decision.
var leafClasses = map[string]leafClass{
	"version":                        classGovernance,
	"exclude":                        classProfile,
	"languages":                      classProfile,
	"languages.*.gate":               classProfile, // reaches the profile only through the producer status it changes
	"analyzers":                      classProfile,
	"analyzers.*.gate":               classProfile,    // same: a demanded gate changes whether an absent row is not applicable
	"analyzers.*.timeout":            classGovernance, // a timeout shows as a producer status
	"coverage":                       classProfile,
	"coverage.gate":                  classProfile, // same, through the supplied-coverage producer status
	"ai":                             classGovernance,
	"coupling.min_severity":          classGovernance, // filters advisories, not facts
	"coupling.duplicated_knowledge":  classClassification,
	"coupling.volatility_cascade":    classClassification,
	"coupling.gate":                  classGovernance,
	"layers":                         classGovernance, // findings and seam policy; module layer is model
	"modules":                        classModel,
	"modules.*.depends_on":           classGovernance,
	"modules.*.visible_to":           classGovernance,
	"modules.*.reviewed_at":          classGovernance,
	"modules.*.reviewed_by":          classGovernance,
	"external_systems":               classClassification,
	"rules.patterns":                 classProfile, // the ast-grep pass runs these, so they enter the settings hash
	"rules":                          classGovernance,
	"waivers":                        classGovernance,
	"metrics.function_loc_threshold": classClassification,
	"metrics.*.gate":                 classGovernance,
	"metrics.*.min_delta":            classGovernance,
	"metrics.*.max_new":              classGovernance,
	"metrics.*.enabled":              classClassification,
	"module_review":                  classGovernance,
	"file_class":                     classProfile,
	"outputs":                        classGovernance,
}

// configLeafPaths walks Config by its yaml tags. A map contributes a `*`
// segment; an exported `yaml:"-"` map (metric entries) does too.
func configLeafPaths() []string {
	var out []string
	var walk func(t reflect.Type, prefix string)
	walk = func(t reflect.Type, prefix string) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Map:
			walk(t.Elem(), prefix+".*")
		case reflect.Struct:
			if t.PkgPath() == "time" {
				out = append(out, prefix)
				return
			}
			fields := 0
			for i := range t.NumField() {
				f := t.Field(i)
				if !f.IsExported() {
					continue
				}
				name := strings.Split(f.Tag.Get("yaml"), ",")[0]
				if name == "-" {
					if f.Type.Kind() == reflect.Map {
						fields++
						walk(f.Type, prefix)
					}
					continue
				}
				if name == "" {
					name = strings.ToLower(f.Name)
				}
				fields++
				walk(f.Type, strings.TrimPrefix(prefix+"."+name, "."))
			}
			if fields == 0 {
				out = append(out, prefix)
			}
		default:
			out = append(out, prefix)
		}
	}
	walk(reflect.TypeOf(Config{}), "")
	sort.Strings(out)
	return out
}

func matchSegments(pattern, path []string) bool {
	if len(pattern) > len(path) {
		return false
	}
	for i, seg := range pattern {
		if seg != "*" && seg != path[i] {
			return false
		}
	}
	return true
}

// classOf returns the class of the most specific entry for leaf, and the number
// of entries tied at that specificity.
func classOf(leaf string) (leafClass, int) {
	best, ties := -1, 0
	var class leafClass
	for pattern, c := range leafClasses {
		segs := strings.Split(pattern, ".")
		if !matchSegments(segs, strings.Split(leaf, ".")) {
			continue
		}
		switch {
		case len(segs) > best:
			best, ties, class = len(segs), 1, c
		case len(segs) == best:
			ties++
		}
	}
	return class, ties
}

func TestEveryConfigLeafHasOneClass(t *testing.T) {
	t.Parallel()
	leaves := configLeafPaths()
	if len(leaves) < 40 {
		t.Fatalf("walked %d config leaves, want the whole Config tree: %v", len(leaves), leaves)
	}
	used := map[string]bool{}
	for _, leaf := range leaves {
		class, ties := classOf(leaf)
		if ties != 1 {
			t.Errorf("config leaf %q matches %d classification entries, want exactly 1: add or fix it in leafClasses", leaf, ties)
			continue
		}
		_ = class
		for pattern := range leafClasses {
			if matchSegments(strings.Split(pattern, "."), strings.Split(leaf, ".")) {
				used[pattern] = true
			}
		}
	}
	for pattern := range leafClasses {
		if !used[pattern] {
			t.Errorf("leafClasses entry %q matches no config leaf: remove the stale entry", pattern)
		}
	}
}

const classificationBaseYAML = `version: 2
languages:
  go: {enabled: true}
modules:
  core: {paths: ["core/**"], owner: team-a, volatility: low, layer: domain}
  app: {paths: ["app/**"], owner: team-a, volatility: high, layer: edge}
layers: [edge, domain]
rules: []
`

func classificationHash(t *testing.T, yamlBody string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	if err := os.WriteFile(path, []byte(yamlBody), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(t.Context(), path)
	if err != nil {
		t.Fatalf("Load: %v\n%s", err, yamlBody)
	}
	return acquisition.ClassificationHash(cfg.PolicySnapshot())
}

func TestClassificationHashFollowsTheLeafClasses(t *testing.T) {
	t.Parallel()
	want := classificationHash(t, classificationBaseYAML)
	tests := []struct {
		name    string
		yaml    string
		changed bool
	}{
		{"comment", "# a comment\n" + classificationBaseYAML, false},
		{"reordered keys and spacing", strings.Replace(classificationBaseYAML, "layers: [edge, domain]\n", "layers:\n  - edge\n  - domain\n", 1), false},
		{"waiver", strings.Replace(classificationBaseYAML, "rules: []\n", "rules:\n  - id: r\n    type: forbidden_dependency\n    from: app/**\n    to: core/**\nwaivers:\n  - rule: r\n    from: app/**\n    reason: x\n    approved_by: '@owner'\n    expires: '2099-01-01'\n", 1), false},
		{"rule", strings.Replace(classificationBaseYAML, "rules: []\n", "rules:\n  - id: r\n    type: cycle\n    gate: warn\n", 1), false},
		{"reviewed_at", strings.Replace(classificationBaseYAML, "layer: domain}", "layer: domain, reviewed_at: 2026-01-01T00:00:00Z}", 1), false},
		{"allowlist", strings.Replace(classificationBaseYAML, "layer: edge}", "layer: edge, depends_on: [core]}", 1), false},
		{"coupling gate", classificationBaseYAML + "coupling:\n  gate:\n    distributed_monolith: {mode: fail}\n", false},
		{"metric gate", classificationBaseYAML + "metrics:\n  cycle: {gate: warn}\n", false},
		{"volatility cascade", classificationBaseYAML + "coupling:\n  volatility_cascade: true\n", true},
		{"duplicated knowledge", classificationBaseYAML + "coupling:\n  duplicated_knowledge: advisory\n", true},
		{"external system", classificationBaseYAML + "external_systems:\n  aws: {targets: [\"aws/**\"], volatility: high}\n", true},
		{"function loc threshold", classificationBaseYAML + "metrics:\n  function_loc_threshold: 80\n", true},
		{"metric disabled", classificationBaseYAML + "metrics:\n  cycle: {enabled: false}\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classificationHash(t, tt.yaml)
			if (got != want) != tt.changed {
				t.Errorf("classification hash changed = %v, want %v", got != want, tt.changed)
			}
		})
	}
}
