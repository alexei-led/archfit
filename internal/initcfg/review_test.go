package initcfg

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const (
	testReviewModZ   = "zeta"
	testReviewModA   = "alpha"
	testReviewPathZ  = "zeta/**"
	testReviewPathA  = "alpha/**"
	testReviewDeploy = "web-service"
	testReviewWebMod = "web"
)

// ---------------------------------------------------------------------------
// Issue derivation
// ---------------------------------------------------------------------------

func TestDiffModules_Issues(t *testing.T) {
	tests := []struct {
		name         string
		existing     []ExistingModule
		fresh        []ModuleDef
		requireLayer bool
		wantCodes    []string
	}{
		{
			name: "fully specified module has no issues",
			existing: []ExistingModule{{
				Name: testSvc, Paths: []string{testSvcPath},
				HasSubdomain: true, HasVolatility: true, HasLayer: true, HasOwner: true,
			}},
			fresh:        []ModuleDef{{Name: testSvc, Paths: []string{testSvcPath}}},
			requireLayer: true,
			wantCodes:    nil,
		},
		{
			name: "missing owner only",
			existing: []ExistingModule{{
				Name: testSvc, Paths: []string{testSvcPath},
				HasSubdomain: true, HasVolatility: true, HasLayer: true,
			}},
			fresh:        []ModuleDef{{Name: testSvc, Paths: []string{testSvcPath}}},
			requireLayer: true,
			wantCodes:    []string{IssueMissingOwner},
		},
		{
			name: "subdomain alone closes the volatility gap",
			existing: []ExistingModule{{
				Name: testSvc, Paths: []string{testSvcPath},
				HasSubdomain: true, HasLayer: true, HasOwner: true,
			}},
			fresh:        []ModuleDef{{Name: testSvc, Paths: []string{testSvcPath}}},
			requireLayer: true,
			wantCodes:    nil,
		},
		{
			name: "volatility alone closes the volatility gap",
			existing: []ExistingModule{{
				Name: testSvc, Paths: []string{testSvcPath},
				HasVolatility: true, HasLayer: true, HasOwner: true,
			}},
			fresh:        []ModuleDef{{Name: testSvc, Paths: []string{testSvcPath}}},
			requireLayer: true,
			wantCodes:    nil,
		},
		{
			name: "neither subdomain nor volatility",
			existing: []ExistingModule{{
				Name: testSvc, Paths: []string{testSvcPath},
				HasLayer: true, HasOwner: true,
			}},
			fresh:        []ModuleDef{{Name: testSvc, Paths: []string{testSvcPath}}},
			requireLayer: true,
			wantCodes:    []string{IssueMissingVolatilityInput},
		},
		{
			name: "missing layer only counts when the layer policy is active",
			existing: []ExistingModule{{
				Name: testSvc, Paths: []string{testSvcPath},
				HasSubdomain: true, HasVolatility: true, HasOwner: true,
			}},
			fresh:     []ModuleDef{{Name: testSvc, Paths: []string{testSvcPath}}},
			wantCodes: nil,
		},
		{
			name: "missing layer with active layer policy",
			existing: []ExistingModule{{
				Name: testSvc, Paths: []string{testSvcPath},
				HasSubdomain: true, HasVolatility: true, HasOwner: true,
			}},
			fresh:        []ModuleDef{{Name: testSvc, Paths: []string{testSvcPath}}},
			requireLayer: true,
			wantCodes:    []string{IssueMissingLayer},
		},
		{
			name:         "all three gaps on one module, code-sorted",
			existing:     []ExistingModule{{Name: testSvc, Paths: []string{testSvcPath}}},
			fresh:        []ModuleDef{{Name: testSvc, Paths: []string{testSvcPath}}},
			requireLayer: true,
			wantCodes:    []string{IssueMissingLayer, IssueMissingOwner, IssueMissingVolatilityInput},
		},
		{
			name:         "pathless module classifies nothing and is skipped",
			existing:     []ExistingModule{{Name: testSvc}},
			fresh:        []ModuleDef{{Name: testSvc}},
			requireLayer: true,
			wantCodes:    nil,
		},
		{
			name:         "removed module produces no issue",
			existing:     []ExistingModule{{Name: testGone, Paths: []string{testGone + "/**"}}},
			fresh:        nil,
			requireLayer: true,
			wantCodes:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DiffModules(tc.existing, tc.fresh, tc.requireLayer)
			var codes []string
			for _, issue := range got.Issues {
				codes = append(codes, issue.Code)
				if issue.Reason == "" || issue.NextAction == "" {
					t.Errorf("issue %+v must carry a reason and a next action", issue)
				}
				if !strings.Contains(issue.NextAction, issue.Module) {
					t.Errorf("next action %q must name module %q", issue.NextAction, issue.Module)
				}
			}
			if !reflect.DeepEqual(codes, tc.wantCodes) {
				t.Errorf("codes: got %v, want %v", codes, tc.wantCodes)
			}
		})
	}
}

func TestDiffModules_IssuesSortedByModuleThenCode(t *testing.T) {
	existing := []ExistingModule{
		{Name: testReviewModZ, Paths: []string{testReviewPathZ}},
		{Name: testReviewModA, Paths: []string{testReviewPathA}, HasOwner: true, HasLayer: true},
	}
	fresh := []ModuleDef{
		{Name: testReviewModZ, Paths: []string{testReviewPathZ}},
		{Name: testReviewModA, Paths: []string{testReviewPathA}},
	}

	got := DiffModules(existing, fresh, true)
	want := []ModuleIssue{
		{Code: IssueMissingVolatilityInput, Module: testReviewModA},
		{Code: IssueMissingLayer, Module: testReviewModZ},
		{Code: IssueMissingOwner, Module: testReviewModZ},
		{Code: IssueMissingVolatilityInput, Module: testReviewModZ},
	}
	if len(got.Issues) != len(want) {
		t.Fatalf("issue count: got %d, want %d (%+v)", len(got.Issues), len(want), got.Issues)
	}
	for i := range want {
		if got.Issues[i].Module != want[i].Module || got.Issues[i].Code != want[i].Code {
			t.Errorf("issue %d: got %s/%s, want %s/%s",
				i, got.Issues[i].Module, got.Issues[i].Code, want[i].Module, want[i].Code)
		}
	}
}

// TestBuildConfigReview_PathlessStanzaIsDisclosed pins the hole
// unchecked_modules was added to close, one shape narrower than the original:
// a stanza that declares NO paths is skipped by checkModuleFields before any
// issue is raised, so its missing owner shows up in no issue and in no
// unclassified list. Reported nowhere, `issues: []` reads as "every module is
// clean" over a stanza nothing evaluated.
func TestBuildConfigReview_PathlessStanzaIsDisclosed(t *testing.T) {
	// Pathless, and carrying subdomain so it is not even Unclassified — the
	// combination that used to vanish entirely.
	existing := []ExistingModule{
		{Name: testReviewModA, HasSubdomain: true},
		{Name: testReviewModZ, Paths: []string{testReviewPathZ}, HasOwner: true, HasSubdomain: true},
	}
	fresh := []ModuleDef{
		{Name: testReviewModA},
		{Name: testReviewModZ, Paths: []string{testReviewPathZ}},
	}

	rev := BuildConfigReview(DiffModules(existing, fresh, false))
	if len(rev.Issues) != 0 {
		t.Fatalf("fixture regression: this shape must raise no issue, got %+v", rev.Issues)
	}
	var found *ReviewUncheckedModule
	for i, u := range rev.UncheckedModules {
		if u.Module == testReviewModA {
			found = &rev.UncheckedModules[i]
		}
	}
	if found == nil {
		t.Fatalf("the pathless stanza %q is absent from unchecked_modules (%+v) while issues is empty",
			testReviewModA, rev.UncheckedModules)
	}
	if found.Reason != reasonUncheckedPathless {
		t.Errorf("reason = %q, want %q", found.Reason, reasonUncheckedPathless)
	}
	// The module discovery DID account for must not be reported unchecked.
	for _, u := range rev.UncheckedModules {
		if u.Module == testReviewModZ {
			t.Errorf("a fully checked module must not appear in unchecked_modules: %+v", u)
		}
	}
}

// ---------------------------------------------------------------------------
// Status priority
// ---------------------------------------------------------------------------

func TestBuildConfigReview_StatusPriority(t *testing.T) {
	issue := ModuleIssue{Code: IssueMissingOwner, Module: testSvc, Reason: "r", NextAction: "n"}
	deploy := DeployUnitSuggestion{Module: testReviewWebMod, Unit: testReviewDeploy}

	tests := []struct {
		name   string
		report UpdateReport
		want   string
	}{
		{"empty report", UpdateReport{StructuralInSync: true}, ReviewStatusNoKnownIssues},
		{"issue only", UpdateReport{Issues: []ModuleIssue{issue}}, ReviewStatusActionRequired},
		{"added module", UpdateReport{Added: []ModuleDef{{Name: testSvc}}}, ReviewStatusActionRequired},
		// Removed and NameDrift are review-only: --apply writes neither, so a
		// status of action_required would name an edit apply refuses to make.
		{"removed module", UpdateReport{Removed: []ExistingModule{{Name: testGone}}}, ReviewStatusReviewAvailable},
		{"name drift", UpdateReport{NameDrift: []NameDrift{{ConfigName: testGone, DiscoveredName: testSvc}}}, ReviewStatusReviewAvailable},
		{"path drift", UpdateReport{PathDrift: []PathDelta{{Name: testSvc}}}, ReviewStatusActionRequired},
		{"settings only", UpdateReport{Settings: []SettingChange{RustDeepAnalysisSetting()}}, ReviewStatusActionRequired},
		{"suggestion only", UpdateReport{DeployUnitSuggestions: []DeployUnitSuggestion{deploy}}, ReviewStatusReviewAvailable},
		{
			name: "issue outranks suggestion",
			report: UpdateReport{
				Issues:                []ModuleIssue{issue},
				DeployUnitSuggestions: []DeployUnitSuggestion{deploy},
			},
			want: ReviewStatusActionRequired,
		},
		{
			name:   "distance candidate alone is review_available",
			report: UpdateReport{DistanceConfigCandidates: []DistanceConfigCandidate{{Module: testSvc}}},
			want:   ReviewStatusReviewAvailable,
		},
		{
			name:   "llm-only suggestion still reads review_available",
			report: UpdateReport{RuleSuggestions: []RuleSuggestion{{Type: "forbidden_dependency"}}},
			want:   ReviewStatusReviewAvailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildConfigReview(tc.report).Status; got != tc.want {
				t.Errorf("status: got %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// JSON shape
// ---------------------------------------------------------------------------

func TestBuildConfigReview_EmptyListsMarshalAsArrays(t *testing.T) {
	rev := BuildConfigReview(UpdateReport{StructuralInSync: true})
	data, err := json.Marshal(rev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "null") {
		t.Errorf("review document must not contain null lists:\n%s", data)
	}
	if rev.SchemaVersion != ConfigReviewSchemaVersion {
		t.Errorf("schema_version: got %q, want %q", rev.SchemaVersion, ConfigReviewSchemaVersion)
	}
	for _, key := range []string{
		`"added_modules":[]`, `"removed_modules":[]`, `"path_drift":[]`, `"settings":[]`,
		`"issues":[]`, `"unchecked_modules":[]`, `"deploy_units":[]`, `"distance_config":[]`,
	} {
		if !strings.Contains(string(data), key) {
			t.Errorf("missing empty array %s:\n%s", key, data)
		}
	}
}

// TestBuildConfigReview_PureProjection pins that BuildConfigReview copies the
// orders DiffModules and the suggestion builders establish instead of re-sorting.
func TestBuildConfigReview_PureProjection(t *testing.T) {
	report := UpdateReport{
		Added:   []ModuleDef{{Name: testReviewModA}, {Name: testReviewModZ}},
		Removed: []ExistingModule{{Name: testGone}},
		PathDrift: []PathDelta{{
			Name:            testSvc,
			ConfigPaths:     []string{testOldPath},
			DiscoveredPaths: []string{testNewPath},
		}},
		Settings: []SettingChange{RustDeepAnalysisSetting()},
		// Deliberately unsorted: a pure projection preserves the caller's order.
		DeployUnitSuggestions: []DeployUnitSuggestion{
			{Module: testReviewModZ, Unit: testReviewDeploy},
			{Module: testReviewModA, Unit: testReviewDeploy},
		},
		DistanceConfigCandidates: []DistanceConfigCandidate{
			{Module: testReviewModZ, Target: "t2"},
			{Module: testReviewModA, Target: "t1"},
		},
	}

	rev := BuildConfigReview(report)
	if want := []string{testReviewModA, testReviewModZ}; !reflect.DeepEqual(rev.Structure.AddedModules, want) {
		t.Errorf("added_modules: got %v, want %v", rev.Structure.AddedModules, want)
	}
	if want := []string{testGone}; !reflect.DeepEqual(rev.Structure.RemovedModules, want) {
		t.Errorf("removed_modules: got %v, want %v", rev.Structure.RemovedModules, want)
	}
	wantDrift := []ReviewPathDrift{{
		Module:          testSvc,
		ConfigPaths:     []string{testOldPath},
		DiscoveredPaths: []string{testNewPath},
	}}
	if !reflect.DeepEqual(rev.Structure.PathDrift, wantDrift) {
		t.Errorf("path_drift: got %+v, want %+v", rev.Structure.PathDrift, wantDrift)
	}
	if rev.ReviewSuggestions.DeployUnits[0].Module != testReviewModZ {
		t.Errorf("deploy_units order must be preserved: %+v", rev.ReviewSuggestions.DeployUnits)
	}
	if rev.ReviewSuggestions.DistanceConfig[0].Module != testReviewModZ {
		t.Errorf("distance_config order must be preserved: %+v", rev.ReviewSuggestions.DistanceConfig)
	}
	if rev.Structure.Settings[0].Code != SettingCodeRustDeepAnalysis {
		t.Errorf("settings: got %+v", rev.Structure.Settings)
	}
}

// ---------------------------------------------------------------------------
// Status line
// ---------------------------------------------------------------------------

func TestRenderReviewStatus(t *testing.T) {
	tests := []struct {
		name   string
		report UpdateReport
		want   string
	}{
		{
			name:   "no known issues",
			report: UpdateReport{StructuralInSync: true},
			want:   "status: no_known_issues — nothing detected by these checks\n",
		},
		{
			name:   "review available",
			report: UpdateReport{DeployUnitSuggestions: []DeployUnitSuggestion{{Module: testReviewWebMod}}},
			want:   "status: review_available — nothing to apply; review items only\n",
		},
		{
			name: "action required counts issues and pending edits",
			report: UpdateReport{
				Issues:   []ModuleIssue{{Code: IssueMissingOwner, Module: testSvc}},
				Added:    []ModuleDef{{Name: testReviewModA}},
				Settings: []SettingChange{RustDeepAnalysisSetting()},
			},
			want: "status: action_required — 1 module issue(s), 2 pending edit(s)\n",
		},
		{
			// The count names only what --apply writes: review-only buckets are
			// reported in their own sections, never as pending edits.
			name: "pending-edit count excludes review-only buckets",
			report: UpdateReport{
				Issues:    []ModuleIssue{{Code: IssueMissingOwner, Module: testSvc}},
				Added:     []ModuleDef{{Name: testReviewModA}},
				Removed:   []ExistingModule{{Name: testGone}},
				NameDrift: []NameDrift{{ConfigName: testGone, DiscoveredName: testSvc}},
			},
			// The unchecked-module clause is not decoration: Removed stanzas are
			// never field-checked, so the issue count is a partial audit and the
			// line has to say so.
			// CHANGED: unchecked covers two reasons now (unmatched by discovery, and
			// pathless stanzas the field checks skip), so the one-line status points
			// at the list instead of asserting one of them for all.
			want: "status: action_required — 1 module issue(s), 1 pending edit(s) (1 module(s) unchecked — see unchecked_modules)\n",
		},
		{
			// Name drift alone leaves nothing unchecked: the name-drift pass
			// rescues those stanzas and their fields are evaluated like any other.
			name: "name drift alone leaves nothing unchecked",
			report: UpdateReport{
				NameDrift: []NameDrift{{ConfigName: testGone, DiscoveredName: testSvc}},
			},
			want: "status: review_available — nothing to apply; review items only\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderReviewStatus(BuildConfigReview(tc.report))
			if got != tc.want {
				t.Errorf("status line:\n  got  %q\n  want %q", got, tc.want)
			}
			for _, banned := range []string{"healthy", "complete", "looks good"} {
				if strings.Contains(strings.ToLower(got), banned) {
					t.Errorf("status line must not claim %q: %q", banned, got)
				}
			}
		})
	}

	// Defensive: no_known_issues has to name unchecked modules too — "nothing
	// detected by these checks" over skipped modules is the misreading the clause
	// exists to prevent. Built by hand because BuildConfigReview cannot produce
	// this pair: both sources of UncheckedModules (Removed, Pathless) also make
	// HasReviewItems true, which lifts the status to review_available.
	t.Run("no_known_issues still names unchecked modules", func(t *testing.T) {
		got := RenderReviewStatus(ConfigReview{
			Status:           ReviewStatusNoKnownIssues,
			UncheckedModules: []ReviewUncheckedModule{{Module: testGone, Reason: reasonUncheckedPathless}},
		})
		want := "status: no_known_issues — nothing detected by these checks (1 module(s) unchecked — see unchecked_modules)\n"
		if got != want {
			t.Errorf("status line:\n  got  %q\n  want %q", got, want)
		}
	})
}

// ---------------------------------------------------------------------------
// Text report sections
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_SettingsAndIssues(t *testing.T) {
	r := UpdateReport{
		StructuralInSync: true,
		Settings:         []SettingChange{RustDeepAnalysisSetting()},
		Issues: []ModuleIssue{
			newModuleIssue(IssueMissingOwner, testSvc),
		},
	}
	got := RenderUpdateReport(r, nil, nil)
	for _, want := range []string{
		"SETTINGS (1 config setting(s)",
		SettingCodeRustDeepAnalysis,
		"ISSUES (1 module gap(s) needing a decision)",
		IssueMissingOwner,
		testSvc,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderUpdateReport_SettingsAndIssuesOmittedWhenEmpty(t *testing.T) {
	got := RenderUpdateReport(UpdateReport{StructuralInSync: true}, nil, nil)
	for _, sec := range []string{"SETTINGS", "ISSUES"} {
		if strings.Contains(got, sec) {
			t.Errorf("empty section %q should be omitted:\n%s", sec, got)
		}
	}
}

func TestRustDeepAnalysisSetting_NamesTheAppliedEdit(t *testing.T) {
	s := RustDeepAnalysisSetting()
	if s.Code != SettingCodeRustDeepAnalysis {
		t.Errorf("code: got %q", s.Code)
	}
	for _, want := range []string{"languages.rust.enabled: auto", "cargo_modules", "scip"} {
		if !strings.Contains(s.Reason, want) {
			t.Errorf("reason must name %q: %q", want, s.Reason)
		}
	}
}

func TestHasReviewSuggestions(t *testing.T) {
	tests := []struct {
		name   string
		report UpdateReport
		want   bool
	}{
		{"empty", UpdateReport{}, false},
		{"suggested module", UpdateReport{Suggested: []ModuleDef{{Name: testSvc}}}, true},
		{"deploy unit", UpdateReport{DeployUnitSuggestions: []DeployUnitSuggestion{{Module: testSvc}}}, true},
		{"distance", UpdateReport{DistanceConfigCandidates: []DistanceConfigCandidate{{Module: testSvc}}}, true},
		{"rule", UpdateReport{RuleSuggestions: []RuleSuggestion{{Type: "x"}}}, true},
		{"external system", UpdateReport{ExternalSystemSuggestions: []ExternalSystemSuggestion{{Name: "s3"}}}, true},
		{"structure change is not a suggestion", UpdateReport{Added: []ModuleDef{{Name: testSvc}}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasReviewSuggestions(tc.report); got != tc.want {
				t.Errorf("HasReviewSuggestions: got %v, want %v", got, tc.want)
			}
		})
	}
}
