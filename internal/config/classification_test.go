package config

import (
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
	"exclude":                        classProfile,
	"languages":                      classProfile,
	"languages.*.gate":               classProfile, // reaches the profile only through the producer status it changes
	"analyzers":                      classProfile,
	"analyzers.*.gate":               classProfile,    // same: a demanded gate changes whether an absent row is not applicable
	"analyzers.*.timeout":            classGovernance, // a timeout shows as a producer status
	"coverage":                       classProfile,
	"coverage.gate":                  classProfile,    // same, through the supplied-coverage producer status
	"coupling.min_severity":          classGovernance, // filters advisories, not facts
	"coupling.volatility_cascade":    classClassification,
	"layers":                         classGovernance, // findings and seam policy; module layer is model
	"modules":                        classModel,
	"modules.*.depends_on":           classGovernance,
	"modules.*.visible_to":           classGovernance,
	"modules.*.reviewed_at":          classGovernance,
	"modules.*.reviewed_by":          classGovernance,
	"externalsystems":                classClassification,
	"metrics.function_loc_threshold": classClassification,
	"metrics.*.gate":                 classGovernance,
	"metrics.*.min_delta":            classGovernance,
	"metrics.*.max_new":              classGovernance,
	"metrics.*.enabled":              classClassification,
	"modulereview":                   classGovernance,
	"modulereview.change_gate":       classGovernance,
	"fileclass":                      classProfile,
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

const classificationBaseYAML = `version: 3
owners: [owner]
languages:
  go: {enabled: true}
modules:
  core: {paths: ["core/**"], volatility: low, layer: domain}
  app: {paths: ["app/**"], volatility: high, layer: edge}
layers: [edge, domain]
rules: []
`

func classificationHash(t *testing.T, yamlBody string) string {
	t.Helper()
	cfg, err := ReportConfig([]byte(yamlBody))
	if err != nil {
		t.Fatalf("ReportConfig: %v\n%s", err, yamlBody)
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
		{"exception", classificationBaseYAML + "exceptions:\n  - {rule: graph, from: app, to: core, why: accepted}\n", false},
		{"rule", strings.Replace(classificationBaseYAML, "rules: []\n", "rules:\n  - {id: guide, note: Keep it simple, why: clarity}\n", 1), false},
		{"allowlist", strings.Replace(classificationBaseYAML, "layer: edge}", "layer: edge, depends_on: [core]}", 1), false},
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
