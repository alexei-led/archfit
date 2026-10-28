package rules_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

// wantLocationCap is the location list a finding may carry; the full count
// rides in matched_by.locations_total.
const wantLocationCap = 50

// TestModuleCycle_OneFindingPerComponent pins the shape of a module-cycle
// finding: one per strongly-connected component, members sorted once in
// matched_by.cycle_modules, whatever the number of ordered module pairs
// inside it. A component that reports a finding per pair grows the report
// quadratically — one 11-module component produced 72 findings and 72 agent
// tasks for a single architecture defect.
func TestModuleCycle_OneFindingPerComponent(t *testing.T) {
	tests := []struct {
		name      string
		declared  []string
		edges     []moduleTestEdge
		want      []string // members of each component, in finding order
		wantPairs []string // cycle_pairs_total of each finding
	}{
		{
			name:     "three-module cycle is one finding",
			declared: []string{modBilling, modShipping, modCatalog},
			edges: []moduleTestEdge{
				{nodeBillingAGo, nodePkgShipping, modBilling, modShipping, nil},
				{nodeShippingBGo, nodePkgCatalog, modShipping, modCatalog, nil},
				{nodeCatalogC, nodePkgBilling, modCatalog, modBilling, nil},
			},
			want:      []string{"billing, catalog, shipping"},
			wantPairs: []string{"3"},
		},
		{
			name:     "two-module cycle is one finding",
			declared: []string{modBilling, modShipping},
			edges: []moduleTestEdge{
				{nodeBillingAGo, nodePkgShipping, modBilling, modShipping, nil},
				{nodeShippingBGo, nodePkgBilling, modShipping, modBilling, nil},
			},
			want:      []string{membersBillingSH},
			wantPairs: []string{"2"},
		},
		{
			name:     "two disjoint components are two findings",
			declared: []string{modBilling, modShipping, modCatalog, modAudit},
			edges: []moduleTestEdge{
				{nodeBillingAGo, nodePkgShipping, modBilling, modShipping, nil},
				{nodeShippingBGo, nodePkgBilling, modShipping, modBilling, nil},
				{nodeCatalogC, "package:audit", modCatalog, modAudit, nil},
				{"file:audit/d.go", nodePkgCatalog, modAudit, modCatalog, nil},
			},
			want:      []string{"audit, catalog", membersBillingSH},
			wantPairs: []string{"2", "2"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			findings := checkProduction(newModuleCycleRule(t, "", tc.declared...), moduleSet(tc.edges...))
			if len(findings) != len(tc.want) {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), len(tc.want), findings)
			}
			for i, f := range findings {
				if got := f.MatchedBy["cycle_modules"]; got != tc.want[i] {
					t.Errorf("finding %d cycle_modules = %q, want %q", i, got, tc.want[i])
				}
				if got := f.MatchedBy["cycle_pairs_total"]; got != tc.wantPairs[i] {
					t.Errorf("finding %d cycle_pairs_total = %q, want %q", i, got, tc.wantPairs[i])
				}
				members := strings.Split(tc.want[i], ", ")
				if got := f.MatchedBy["cycle_size"]; got != strconv.Itoa(len(members)) {
					t.Errorf("finding %d cycle_size = %q, want %d", i, got, len(members))
				}
				if f.Kind != kindGate || f.Severity != finding.SeverityHigh {
					t.Errorf("finding %d kind/severity = %s/%s, want gate/high", i, f.Kind, f.Severity)
				}
				if want := finding.NewKeyed(ruleIDModuleCycle, kindModuleDep, members...).ID; f.ID != want {
					t.Errorf("finding %d ID = %q, want the component key %q", i, f.ID, want)
				}
				// The representative pair names a dependency that exists inside
				// the component, so the repair goal points at a real edge.
				if f.Edge.From.Module == "" || f.Edge.To.Module == "" || f.Edge.Kind != kindModuleDep {
					t.Errorf("finding %d edge = %+v, want a module pair inside the component", i, f.Edge)
				}
				wantLocations := 0
				for _, e := range tc.edges {
					if e.fromModule == f.Edge.From.Module && e.toModule == f.Edge.To.Module {
						wantLocations++
					}
				}
				if wantLocations == 0 {
					t.Errorf("finding %d edge %s -> %s is not a dependency of the component", i, f.Edge.From.Module, f.Edge.To.Module)
				}
			}
		})
	}
}

// TestModuleCycle_FullMeshYieldsOneFinding bounds a dense component: a fully
// meshed 16-module component has 240 ordered pairs, all of them one finding.
// The pair count stays visible in matched_by, and the locations stay capped.
func TestModuleCycle_FullMeshYieldsOneFinding(t *testing.T) {
	mesh := make([]string, 0, 16)
	for i := range 16 {
		mesh = append(mesh, fmt.Sprintf("m%02d", i))
	}
	edges := meshEdges(mesh)
	var sites int
	for i := range edges {
		edges[i].locs = []relationship.Location{{File: edges[i].from + "/site.go", Line: i + 1}}
		sites++
	}
	findings := checkProduction(newModuleCycleRule(t, "", mesh...), moduleSet(edges...))
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want one for the whole component", len(findings))
	}
	f := findings[0]
	if f.MatchedBy["cycle_size"] != "16" || f.MatchedBy["cycle_pairs_total"] != "240" {
		t.Errorf("matched_by = %v, want a 16-member component with 240 pairs", f.MatchedBy)
	}
	if len(f.Locations) != wantLocationCap || f.MatchedBy["locations_total"] != strconv.Itoa(sites) {
		t.Errorf("locations = %d, locations_total = %s, want %d kept of %d distinct sites",
			len(f.Locations), f.MatchedBy["locations_total"], wantLocationCap, sites)
	}
	if !strings.Contains(f.Why, fmt.Sprintf("%d import sites", sites)) {
		t.Errorf("why = %q, want the omitted-site count", f.Why)
	}
	assertBoundedWhy(t, f.Why)
}

// TestModuleCycle_LocationsAreEveryMemberPairSite pins that the single
// finding carries the import sites of every ordered pair in the component —
// sorted, deduplicated, and capped with the full count in matched_by — rather
// than the sites of one pair.
func TestModuleCycle_LocationsAreEveryMemberPairSite(t *testing.T) {
	edges := []moduleTestEdge{
		{nodeBillingAGo, nodePkgShipping, modBilling, modShipping, []relationship.Location{
			loc("billing/app/notify.go", 3), loc("billing/app/notify.go", 3), // duplicate site
		}},
		{nodeShippingBGo, nodePkgBilling, modShipping, modBilling, []relationship.Location{
			loc("shipping/app/bill.go", 7),
		}},
	}
	findings := checkProduction(newModuleCycleRule(t, "", modBilling, modShipping), moduleSet(edges...))
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	f := findings[0]
	if got, want := fmt.Sprint(f.Locations), fmt.Sprint([]relationship.Location{
		loc("billing/app/notify.go", 3), loc("shipping/app/bill.go", 7),
	}); got != want {
		t.Errorf("locations = %s, want every member pair's sites in file order %s", got, want)
	}
	if f.MatchedBy["locations_total"] != "2" {
		t.Errorf("locations_total = %q, want the 2 distinct sites", f.MatchedBy["locations_total"])
	}
}

// TestModuleCycle_DeterministicAcrossEdgeOrder pins that the findings — their
// order, IDs, representative pairs and locations — do not depend on the order
// the relationship set arrives in.
func TestModuleCycle_DeterministicAcrossEdgeOrder(t *testing.T) {
	edges := []moduleTestEdge{
		{nodeBillingAGo, nodePkgShipping, modBilling, modShipping, []relationship.Location{loc("billing/a.go", 1)}},
		{nodeShippingBGo, nodePkgCatalog, modShipping, modCatalog, []relationship.Location{loc("shipping/b.go", 2)}},
		{nodeCatalogC, nodePkgBilling, modCatalog, modBilling, []relationship.Location{loc("catalog/c.go", 3)}},
		{"file:catalog/d.go", nodePkgShipping, modCatalog, modShipping, []relationship.Location{loc("catalog/d.go", 4)}},
	}
	reversed := make([]moduleTestEdge, len(edges))
	for i, e := range edges {
		reversed[len(edges)-1-i] = e
	}
	r := newModuleCycleRule(t, "", modBilling, modShipping, modCatalog)
	first := checkProduction(r, moduleSet(edges...))
	second := checkProduction(r, moduleSet(reversed...))
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("got %d and %d findings, want 1 and 1", len(first), len(second))
	}
	a, b := first[0], second[0]
	if a.ID != b.ID || a.Edge != b.Edge || a.MatchedBy["cycle_modules"] != b.MatchedBy["cycle_modules"] ||
		a.MatchedBy["cycle_pairs_total"] != b.MatchedBy["cycle_pairs_total"] || fmt.Sprint(a.Locations) != fmt.Sprint(b.Locations) {
		t.Errorf("findings differ across edge order:\n%+v\n%+v", a, b)
	}
}
