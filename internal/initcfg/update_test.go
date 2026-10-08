package initcfg

import (
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy"
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
			got := DiffModules(tc.existing, tc.fresh, tc.requireLayer, nil)
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

// TestResolveNameDrift pins the naming-difference pass in isolation:
// diffModulesByName matches by NAME, so a config key and a discovery key over
// the same paths read as an add + a remove. Left alone, that made
// `config update` report action_required on this repo's own config and pointed
// --apply at commenting out 44 stanzas.
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
			got := resolveNameDrift(tc.in, false)
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

	// The name pass matches by NAME and skips everything it put in Removed, so a
	// stanza discovery merely names differently was never field-checked: on this
	// repo's own reference config that left 30 of 45 modules unevaluated while
	// the document still reported `issues: []`. "Not evaluated" must never render
	// as "clean". DiffModules folds both passes, so this asserts the exported
	// entry point — the shape a caller cannot get wrong.
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
		got := DiffModules(existing, fresh, requireLayer, nil)

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

// TestDiffModules_OwnershipCoverage pins the ownership pass: a discovered
// module is new only when some source in it has no configured owner, whatever
// the names. A curated map finer or coarser than discovery's two-segment
// directories gets no catch-all stanza, and its owners are not reported as
// unmatched.
func TestDiffModules_OwnershipCoverage(t *testing.T) {
	cfgMod := func(name string, paths ...string) ExistingModule {
		return ExistingModule{Name: name, Paths: paths, HasOwner: true, HasSubdomain: true}
	}
	found := func(name, path string, sources ...string) ModuleDef {
		return ModuleDef{Name: name, Paths: []string{path}, Sources: sources}
	}
	tests := []struct {
		name        string
		existing    []ExistingModule
		fresh       []ModuleDef
		noResolver  bool
		wantAdded   []string
		wantRemoved []string
		wantCovered map[string][]string
		wantDrift   int
	}{
		{
			name: "finer curated map owns a discovered catch-all",
			existing: []ExistingModule{
				cfgMod(testModDomainOrder, "internal/domain/order/**"),
				cfgMod("domain-billing", "internal/domain/billing/**"),
			},
			fresh:       []ModuleDef{found(testModDomain, "internal/domain/**", "internal/domain/billing", "internal/domain/order")},
			wantCovered: map[string][]string{testModDomain: {"domain-billing", testModDomainOrder}},
		},
		{
			name:        "coarser capability module owns several discovered directories",
			existing:    []ExistingModule{cfgMod("decision-core", "internal/**")},
			fresh:       []ModuleDef{found("assessment", "internal/assessment/**", "internal/assessment/rules", "internal/assessment/score")},
			wantCovered: map[string][]string{"assessment": {"decision-core"}},
		},
		{
			name:        "x/** owns the bare package node x",
			existing:    []ExistingModule{cfgMod("application", "internal/app/**", "internal/appkit/**")},
			fresh:       []ModuleDef{found("app", "internal/app/**", "internal/app")},
			wantCovered: map[string][]string{"app": {"application"}},
		},
		{
			name:        "same path set stays a name drift, not coverage",
			existing:    []ExistingModule{cfgMod("internal/app", "internal/app/**")},
			fresh:       []ModuleDef{found("app", "internal/app/**", "internal/app")},
			wantCovered: map[string][]string{},
			wantDrift:   1,
		},
		{
			name:      "partially owned module is genuinely new code",
			existing:  []ExistingModule{cfgMod(testModDomainOrder, "internal/domain/order/**")},
			fresh:     []ModuleDef{found(testModDomain, "internal/domain/**", "internal/domain/order", "internal/domain/shipping")},
			wantAdded: []string{testModDomain},
		},
		{
			name:        "module without sources stays name-matched",
			existing:    []ExistingModule{cfgMod("ui", "src/**")},
			fresh:       []ModuleDef{{Name: layerCore, Paths: []string{"src/core/**"}}},
			wantAdded:   []string{layerCore},
			wantRemoved: []string{"ui"},
		},
		{
			name:        "nil resolver disables the pass",
			existing:    []ExistingModule{cfgMod(testModDomainOrder, "internal/domain/order/**")},
			fresh:       []ModuleDef{found(testModDomain, "internal/domain/**", "internal/domain/order")},
			noResolver:  true,
			wantAdded:   []string{testModDomain},
			wantRemoved: []string{testModDomainOrder},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modules := make(map[string]policy.ModuleDef, len(tt.existing))
			for _, e := range tt.existing {
				modules[e.Name] = policy.ModuleDef{Paths: e.Paths}
			}
			ownerOf := policy.BuildModuleMap(modules).ModuleFor
			if tt.noResolver {
				ownerOf = nil
			}
			got := DiffModules(tt.existing, tt.fresh, false, ownerOf)

			if names := moduleDefNames(got.Added); !equalStrings(names, tt.wantAdded) {
				t.Errorf("added = %v, want %v", names, tt.wantAdded)
			}
			if names := existingModuleNames(got.Removed); !equalStrings(names, tt.wantRemoved) {
				t.Errorf("removed = %v, want %v", names, tt.wantRemoved)
			}
			covered := map[string][]string{}
			for _, c := range got.Covered {
				covered[c.Discovered.Name] = c.Owners
			}
			if len(covered) != len(tt.wantCovered) || (len(covered) > 0 && !reflect.DeepEqual(covered, tt.wantCovered)) {
				t.Errorf("covered = %v, want %v", covered, tt.wantCovered)
			}
			if len(got.NameDrift) != tt.wantDrift {
				t.Errorf("name drift = %+v, want %d", got.NameDrift, tt.wantDrift)
			}
			if wantSync := len(tt.wantAdded) == 0 && len(tt.wantRemoved) == 0 && tt.wantDrift == 0; got.StructuralInSync != wantSync {
				t.Errorf("StructuralInSync = %v, want %v", got.StructuralInSync, wantSync)
			}
			if HasModuleEdits(got) != (len(tt.wantAdded) > 0) {
				t.Errorf("HasModuleEdits = %v with added %v", HasModuleEdits(got), tt.wantAdded)
			}
		})
	}
}

// TestDiffModules_OwnershipRescuesOwnersIntoFieldChecks: a stanza that owns
// discovered source leaves Removed and is checked like any matched module —
// also when the discovered module is only partly owned and stays Added — so
// its gaps are reported instead of hidden behind "unmatched".
func TestDiffModules_OwnershipRescuesOwnersIntoFieldChecks(t *testing.T) {
	for name, sources := range map[string][]string{
		"fully owned":  {"internal/domain/order"},
		"partly owned": {"internal/domain/order", "internal/domain/shipping"},
	} {
		t.Run(name, func(t *testing.T) {
			existing := []ExistingModule{{Name: testModDomainOrder, Paths: []string{"internal/domain/order/**"}}}
			ownerOf := policy.BuildModuleMap(map[string]policy.ModuleDef{
				testModDomainOrder: {Paths: []string{"internal/domain/order/**"}},
			}).ModuleFor
			got := DiffModules(existing, []ModuleDef{{
				Name: testModDomain, Paths: []string{"internal/domain/**"}, Sources: sources,
			}}, false, ownerOf)

			keys := make([]string, 0, len(got.Issues))
			for _, i := range got.Issues {
				keys = append(keys, i.Module+"|"+i.Code)
			}
			want := []string{testModDomainOrder + "|" + IssueMissingOwner, testModDomainOrder + "|" + IssueMissingVolatilityInput}
			if !reflect.DeepEqual(keys, want) {
				t.Errorf("issues = %v, want %v", keys, want)
			}
			if unchecked := BuildConfigReview(got).UncheckedModules; len(unchecked) != 0 {
				t.Errorf("unchecked_modules = %+v, want none", unchecked)
			}
		})
	}
}

const testModDomainOrder = "domain-order"

func equalStrings(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
