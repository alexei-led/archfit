package graph

import (
	"reflect"
	"testing"
)

// langRuby is an unregistered language used to exercise the Lookup default path.
const langRuby = "ruby"

func TestNodeConventionModuleSegments(t *testing.T) {
	tests := []struct {
		name   string
		lang   string
		module string
		want   []string
	}{
		{"go slash path", LangGo, "internal/metrics/boundary", []string{"internal", "metrics", "boundary"}},
		{"ts slash path", LangTypeScript, "src/utils", []string{"src", "utils"}},
		{"python dotted", LangPython, "pkg.metrics.boundary", []string{"pkg", "metrics", "boundary"}},
		{"rust crate dir", LangRust, "crates/grep-cli", []string{"crates", "grep-cli"}},
		{"flat single segment", LangGo, "core", []string{"core"}},
		{"unknown lang slash default", langRuby, "a/b/c", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuiltinConventions.Lookup(tt.lang).ModuleSegments(tt.module)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ModuleSegments(%q) lang=%q = %v, want %v", tt.module, tt.lang, got, tt.want)
			}
		})
	}
}

func TestNodeConventionModuleFileCandidates(t *testing.T) {
	tests := []struct {
		name string
		lang string
		path string
		want []string
	}{
		{
			"python triple",
			LangPython,
			"pkg.metrics.boundary",
			[]string{"pkg/metrics/boundary.py", "pkg/metrics/boundary.pyi", "pkg/metrics/boundary/__init__.py"},
		},
		{"go nil", LangGo, "internal/metrics", nil},
		{"ts nil", LangTypeScript, "src/utils", nil},
		{"rust nil", LangRust, "crates/grep", nil},
		{"unknown nil", langRuby, "lib/foo", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuiltinConventions.Lookup(tt.lang).ModuleFileCandidates(tt.path)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ModuleFileCandidates(%q) lang=%q = %v, want %v", tt.path, tt.lang, got, tt.want)
			}
		})
	}
}

func TestNodeConventionFileToModuleKey(t *testing.T) {
	tests := []struct {
		name string
		lang string
		file string
		want string
	}{
		{"go dir collapse", LangGo, "internal/metrics/boundary/x.go", "internal/metrics/boundary"},
		{"go top-level file", LangGo, "main.go", ""},
		{"go non-go file", LangGo, "README.md", ""},
		{"python src-stripped", LangPython, "src/pkg/mod.py", "pkg.mod"},
		{"python init collapse", LangPython, "pkg/__init__.py", "pkg"},
		{"python non-py file", LangPython, "pkg/mod.pyi", ""},
		{"ts passthrough", LangTypeScript, "src/a.ts", "src/a.ts"},
		{"rust workspace crate", LangRust, "crates/grep/src/lib.rs", "grep"},
		{"rust root-level member", LangRust, "yazi-fs/src/cha/type.rs", "yazi-fs"},
		{"rust root-level member tests", LangRust, "yazi-core/tests/it.rs", "yazi-core"},
		{"rust root crate src", LangRust, "src/main.rs", ""},
		{"rust root crate nested src", LangRust, "src/cmd/run.rs", ""},
		{"rust non-rs file", LangRust, "crates/grep/Cargo.toml", ""},
		{"unknown passthrough", langRuby, "lib/foo.rb", "lib/foo.rb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuiltinConventions.Lookup(tt.lang).FileToModuleKey(tt.file)
			if got != tt.want {
				t.Errorf("FileToModuleKey(%q) lang=%q = %q, want %q", tt.file, tt.lang, got, tt.want)
			}
		})
	}
}

func TestConventionRegistryLookup(t *testing.T) {
	t.Run("known languages carry their priority", func(t *testing.T) {
		wantPriority := map[string]int{
			LangGo:         0,
			LangTypeScript: 1,
			LangPython:     2,
			LangRust:       3,
		}
		for lang, want := range wantPriority {
			c := BuiltinConventions.Lookup(lang)
			if c.Language != lang {
				t.Errorf("Lookup(%q).Language = %q, want %q", lang, c.Language, lang)
			}
			if c.Priority != want {
				t.Errorf("Lookup(%q).Priority = %d, want %d", lang, c.Priority, want)
			}
		}
	})

	t.Run("unknown language gets default", func(t *testing.T) {
		c := BuiltinConventions.Lookup(langRuby)
		if c.Language != langRuby {
			t.Errorf("Language = %q, want %q", c.Language, langRuby)
		}
		if c.ModuleSegmentSep != "/" {
			t.Errorf("ModuleSegmentSep = %q, want %q", c.ModuleSegmentSep, "/")
		}
		if c.Priority != defaultLanguagePriority {
			t.Errorf("Priority = %d, want %d", c.Priority, defaultLanguagePriority)
		}
	})
}
