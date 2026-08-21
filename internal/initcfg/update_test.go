package initcfg

import (
	"reflect"
	"testing"
)

func TestDiffModules(t *testing.T) {
	mod := func(name string, paths ...string) ModuleDef {
		return ModuleDef{Name: name, Paths: paths}
	}
	ex := func(name string, sub, vol, layer bool, paths ...string) ExistingModule {
		return ExistingModule{
			Name:          name,
			Paths:         paths,
			HasSubdomain:  sub,
			HasVolatility: vol,
			HasLayer:      layer,
			HasOwner:      true,
		}
	}

	tests := []struct {
		name         string
		existing     []ExistingModule
		fresh        []ModuleDef
		requireLayer bool
		want         UpdateReport
	}{
		{
			name:     "added only",
			existing: nil,
			fresh:    []ModuleDef{mod("gw", "gw/**"), mod("core", "core/**")},
			want: UpdateReport{
				Added:            []ModuleDef{mod("core", "core/**"), mod("gw", "gw/**")},
				StructuralInSync: false,
			},
		},
		{
			name:     "removed only",
			existing: []ExistingModule{ex("gw", true, true, true, "gw/**"), ex("core", true, true, true, "core/**")},
			fresh:    nil,
			want: UpdateReport{
				Removed:          []ExistingModule{ex("core", true, true, true, "core/**"), ex("gw", true, true, true, "gw/**")},
				StructuralInSync: false,
			},
		},
		{
			name:     "path drift only",
			existing: []ExistingModule{ex("svc", true, true, true, "old/**")},
			fresh:    []ModuleDef{mod("svc", "new/**")},
			want: UpdateReport{
				PathDrift: []PathDelta{
					{Name: testSvc, ConfigPaths: []string{"old/**"}, DiscoveredPaths: []string{"new/**"}},
				},
				StructuralInSync: false,
			},
		},
		{
			name:     "path reorder is NOT drift",
			existing: []ExistingModule{ex("svc", true, true, true, "b/**", "a/**")},
			fresh:    []ModuleDef{mod("svc", "a/**", "b/**")},
			want: UpdateReport{
				StructuralInSync: true,
			},
		},
		{
			name:     "fully in sync",
			existing: []ExistingModule{ex("svc", true, true, true, "svc/**")},
			fresh:    []ModuleDef{mod("svc", "svc/**")},
			want: UpdateReport{
				StructuralInSync: true,
			},
		},
		{
			name: "mixed: add + remove + drift + unclassified",
			existing: []ExistingModule{
				ex("gw", true, true, true, "gw/**"),
				ex("core", false, false, true, "core/**"), // no subdomain AND no volatility → unclassified
				ex("old", true, true, true, "old/**"),     // removed
			},
			fresh: []ModuleDef{
				mod("gw", "gw/v2/**"),  // drift
				mod("core", "core/**"), // unchanged paths, but unclassified
				mod("new", "new/**"),   // added
			},
			want: UpdateReport{
				Added:   []ModuleDef{mod("new", "new/**")},
				Removed: []ExistingModule{ex("old", true, true, true, "old/**")},
				PathDrift: []PathDelta{
					{Name: "gw", ConfigPaths: []string{"gw/**"}, DiscoveredPaths: []string{"gw/v2/**"}},
				},
				Unclassified:     []string{layerCore},
				StructuralInSync: false,
			},
		},
		{
			name: "removed-and-unclassified: removed module excluded from Unclassified",
			existing: []ExistingModule{
				ex(testGone, false, false, false, "gone/**"), // removed AND missing all fields
			},
			fresh: nil,
			want: UpdateReport{
				Removed:          []ExistingModule{ex(testGone, false, false, false, "gone/**")},
				StructuralInSync: false,
				// Unclassified must be nil/empty — testGone is removed
			},
		},
		{
			name: "missing layer with active layer policy → Unclassified",
			existing: []ExistingModule{
				ex("svc", true, true, false, "svc/**"), // HasLayer=false
			},
			fresh:        []ModuleDef{mod("svc", "svc/**")},
			requireLayer: true,
			want: UpdateReport{
				Unclassified:     []string{"svc"},
				StructuralInSync: true,
			},
		},
		{
			name: "missing layer without active layer policy → NOT Unclassified",
			existing: []ExistingModule{
				ex("svc", true, true, false, "svc/**"),
			},
			fresh: []ModuleDef{mod("svc", "svc/**")},
			want: UpdateReport{
				StructuralInSync: true,
			},
		},
		{
			name: "volatility alone satisfies classification (subdomain absent)",
			existing: []ExistingModule{
				ex("svc", false, true, true, "svc/**"),
			},
			fresh: []ModuleDef{mod("svc", "svc/**")},
			want: UpdateReport{
				StructuralInSync: true,
			},
		},
		{
			name: "subdomain alone satisfies classification (volatility absent)",
			existing: []ExistingModule{
				ex("svc", true, false, true, "svc/**"),
			},
			fresh: []ModuleDef{mod("svc", "svc/**")},
			want: UpdateReport{
				StructuralInSync: true,
			},
		},
		{
			name: "path drift preserves original ordering in output",
			existing: []ExistingModule{
				ex("m", true, true, true, "z/**", "a/**"),
			},
			fresh: []ModuleDef{
				// different set — triggers drift; discovered order is c, b
				mod("m", "c/**", "b/**"),
			},
			want: UpdateReport{
				PathDrift: []PathDelta{
					{
						Name:            "m",
						ConfigPaths:     []string{"z/**", "a/**"}, // original config order preserved
						DiscoveredPaths: []string{"c/**", "b/**"}, // original discovered order preserved
					},
				},
				StructuralInSync: false,
			},
		},
		{
			name: "dedupe and empty-string trim in normalization",
			existing: []ExistingModule{
				ex("m", true, true, true, "a/**", "", "a/**"),
			},
			fresh: []ModuleDef{mod("m", "a/**")},
			want: UpdateReport{
				StructuralInSync: true,
			},
		},
		{
			name:     "added output sorted by name",
			existing: nil,
			fresh: []ModuleDef{
				mod("z", "z/**"),
				mod("b", "b/**"),
				mod("m", "m/**"),
			},
			want: UpdateReport{
				Added: []ModuleDef{
					mod("b", "b/**"),
					mod("m", "m/**"),
					mod("z", "z/**"),
				},
				StructuralInSync: false,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DiffModules(tc.existing, tc.fresh, tc.requireLayer)
			if !reflect.DeepEqual(got.Added, tc.want.Added) {
				t.Errorf("Added:\n  got  %#v\n  want %#v", got.Added, tc.want.Added)
			}
			if !reflect.DeepEqual(got.Removed, tc.want.Removed) {
				t.Errorf("Removed:\n  got  %#v\n  want %#v", got.Removed, tc.want.Removed)
			}
			if !reflect.DeepEqual(got.PathDrift, tc.want.PathDrift) {
				t.Errorf("PathDrift:\n  got  %#v\n  want %#v", got.PathDrift, tc.want.PathDrift)
			}
			if !reflect.DeepEqual(got.Unclassified, tc.want.Unclassified) {
				t.Errorf("Unclassified:\n  got  %#v\n  want %#v", got.Unclassified, tc.want.Unclassified)
			}
			if got.StructuralInSync != tc.want.StructuralInSync {
				t.Errorf("StructuralInSync: got %v, want %v", got.StructuralInSync, tc.want.StructuralInSync)
			}
		})
	}
}

// TestResolveNameDrift pins the naming-difference pass: DiffModules matches by
// NAME, so a config key and a discovery key over the same paths read as an
// add + a remove. Left alone, that made `config update` report action_required
// on this repo's own config and pointed --apply at commenting out 44 stanzas.
func TestResolveNameDrift(t *testing.T) {
	mod := func(name string, paths ...string) ModuleDef {
		return ModuleDef{Name: name, Paths: paths}
	}
	ex := func(name string, paths ...string) ExistingModule {
		return ExistingModule{Name: name, Paths: paths, HasOwner: true, HasSubdomain: true}
	}

	tests := []struct {
		name          string
		in            UpdateReport
		wantAdded     []string
		wantRemoved   []string
		wantDrift     []NameDrift
		wantInSync    bool
		wantUntouched bool // the pass must return the report unchanged
	}{
		{
			name: "same paths under a different name is drift, not add+remove",
			in: UpdateReport{
				Added:   []ModuleDef{mod("agenttask", "internal/agenttask/**")},
				Removed: []ExistingModule{ex("internal/agenttask", "internal/agenttask/**")},
			},
			wantAdded:   []string{},
			wantRemoved: []string{},
			wantDrift: []NameDrift{
				{ConfigName: "internal/agenttask", DiscoveredName: "agenttask", Paths: []string{"internal/agenttask/**"}},
			},
			// Naming differences are not "in sync": the report shows a NAME DRIFT
			// section, and an in-sync line beside it would read as "nothing to see".
			wantInSync: false,
		},
		{
			name: "path sets that differ stay a real add and remove",
			in: UpdateReport{
				Added:   []ModuleDef{mod("scripts_eval", "scripts/eval/**")},
				Removed: []ExistingModule{ex("scripts/eval/coverage", "scripts/eval/coverage/**")},
			},
			wantAdded:     []string{"scripts_eval"},
			wantRemoved:   []string{"scripts/eval/coverage"},
			wantDrift:     nil,
			wantUntouched: true,
		},
		{
			name: "path order and duplicates do not block pairing",
			in: UpdateReport{
				Added:   []ModuleDef{mod(testSvc, "b/**", testPathA, testPathA)},
				Removed: []ExistingModule{ex(testSvcCfg, testPathA, "b/**")},
			},
			wantAdded:   []string{},
			wantRemoved: []string{},
			wantDrift: []NameDrift{
				{ConfigName: testSvcCfg, DiscoveredName: testSvc, Paths: []string{testPathA, "b/**"}},
			},
			// Naming differences are not "in sync": the report shows a NAME DRIFT
			// section, and an in-sync line beside it would read as "nothing to see".
			wantInSync: false,
		},
		{
			// Two modules claiming one path set cannot be paired 1:1. Guessing
			// which stanza owns the key is the failure this pass prevents.
			name: "ambiguous path set stays in its original bucket",
			in: UpdateReport{
				Added:   []ModuleDef{mod(testSvc, testPathA)},
				Removed: []ExistingModule{ex("one", testPathA), ex("two", testPathA)},
			},
			wantAdded:     []string{"svc"},
			wantRemoved:   []string{"one", "two"},
			wantDrift:     nil,
			wantUntouched: true,
		},
		{
			name: "a pending path drift keeps the report out of sync",
			in: UpdateReport{
				Added:     []ModuleDef{mod(testSvc, testPathA)},
				Removed:   []ExistingModule{ex(testSvcCfg, testPathA)},
				PathDrift: []PathDelta{{Name: "other"}},
			},
			wantAdded:   []string{},
			wantRemoved: []string{},
			wantDrift: []NameDrift{
				{ConfigName: testSvcCfg, DiscoveredName: testSvc, Paths: []string{testPathA}},
			},
			wantInSync: false,
		},
	}

	// driftNames strips the carried ExistingModule so a case can state only the
	// naming pair it is about.
	driftNames := func(drift []NameDrift) []NameDrift {
		out := make([]NameDrift, 0, len(drift))
		for _, d := range drift {
			out = append(out, NameDrift{ConfigName: d.ConfigName, DiscoveredName: d.DiscoveredName, Paths: d.Paths})
		}
		return out
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveNameDrift(tc.in, false)
			if tc.wantUntouched {
				if !reflect.DeepEqual(got, tc.in) {
					t.Fatalf("report must be returned unchanged:\n  got  %#v\n  want %#v", got, tc.in)
				}
				return
			}
			if names := moduleDefNames(got.Added); !reflect.DeepEqual(names, tc.wantAdded) {
				t.Errorf("Added: got %v, want %v", names, tc.wantAdded)
			}
			if names := existingModuleNames(got.Removed); !reflect.DeepEqual(names, tc.wantRemoved) {
				t.Errorf("Removed: got %v, want %v", names, tc.wantRemoved)
			}
			// Compared on the naming fields only: NameDrift also carries the
			// source ExistingModule so the per-module field checks can run over it.
			if !reflect.DeepEqual(driftNames(got.NameDrift), tc.wantDrift) {
				t.Errorf("NameDrift:\n  got  %#v\n  want %#v", got.NameDrift, tc.wantDrift)
			}
			for _, d := range got.NameDrift {
				if d.Existing.Name != d.ConfigName {
					t.Errorf("NameDrift must carry the configured stanza it was built from: %#v", d)
				}
			}
			if got.StructuralInSync != tc.wantInSync {
				t.Errorf("StructuralInSync: got %v, want %v", got.StructuralInSync, tc.wantInSync)
			}
		})
	}

	// DiffModules matches by NAME and skips everything it put in Removed, so a
	// stanza discovery merely names differently was never field-checked: on this
	// repo's own reference config that left 30 of 45 modules unevaluated while
	// the document still reported `issues: []`. "Not evaluated" must never render
	// as "clean".
	t.Run("a name-drifted stanza is field-checked like any other", func(t *testing.T) {
		existing := []ExistingModule{
			{Name: "internal/api", Paths: []string{"internal/api/**"}},                 // no owner, no volatility input
			{Name: "internal/db", Paths: []string{"internal/db/**"}, HasOwner: true},   // no volatility input
			{Name: "kept", Paths: []string{"kept/**"}, HasOwner: true, HasLayer: true}, // no volatility input
			{Name: testGone, Paths: []string{"gone/**"}},                               // discovery does not emit it
		}
		fresh := []ModuleDef{
			mod("api", "internal/api/**"),
			mod("db", "internal/db/**"),
			mod("kept", "kept/**"),
		}
		requireLayer := true
		got := ResolveNameDrift(DiffModules(existing, fresh, requireLayer), requireLayer)

		want := []string{
			"internal/api|" + IssueMissingLayer,
			"internal/api|" + IssueMissingOwner,
			"internal/api|" + IssueMissingVolatilityInput,
			"internal/db|" + IssueMissingLayer,
			"internal/db|" + IssueMissingVolatilityInput,
			"kept|" + IssueMissingVolatilityInput,
		}
		gotKeys := make([]string, 0, len(got.Issues))
		for _, i := range got.Issues {
			gotKeys = append(gotKeys, i.Module+"|"+i.Code)
		}
		if !reflect.DeepEqual(gotKeys, want) {
			t.Errorf("issues:\n  got  %v\n  want %v", gotKeys, want)
		}
		// The issue names the CONFIG key, which is what a human edits.
		if !reflect.DeepEqual(got.Unclassified, []string{"internal/api", "internal/db", "kept"}) {
			t.Errorf("unclassified = %v, want the config keys of every checked module", got.Unclassified)
		}
		// An unpaired removal is still not checked — discovery found no code for
		// it — so it must be disclosed rather than silently counted as clean.
		for _, i := range got.Issues {
			if i.Module == testGone {
				t.Errorf("an unmatched module must not be field-checked: %+v", i)
			}
		}
		unchecked := BuildConfigReview(got).UncheckedModules
		if len(unchecked) != 1 || unchecked[0].Module != testGone || unchecked[0].Reason == "" {
			t.Errorf("unchecked_modules = %+v, want exactly the unmatched module with a reason", unchecked)
		}
	})
}
