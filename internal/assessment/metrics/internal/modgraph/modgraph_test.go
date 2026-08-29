// Behavior tests for the shared module graph the modularity metrics compute
// over. blast_radius is a versioned metric (blast_radius.v2) that baselines
// compare against, so its edge set is part of the contract.
package modgraph

import (
	"testing"

	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	modA = "a"
	modB = "b"
	modC = "c"
)

func node(id string) relationship.Node {
	return relationship.Node{ID: id, Path: id, Module: relationship.ModuleKey(id), FirstParty: true, BoundaryClassified: true}
}

func edge(from, to, kind string) relationship.Edge {
	return relationship.Edge{
		FromID: from, ToID: to, FromPath: from, ToPath: to,
		FromModule: relationship.ModuleKey(from), ToModule: relationship.ModuleKey(to), Kind: kind,
	}
}

// TestBlastRadiusCountsEveryEdgeKind pins that blast radius reaches through
// non-dependency edges too. Narrowing it to imports/depends_on/uses_internal
// silently changed per-module values on Rust repos with
// analyzers.cargo_modules enabled — whose extractor emits belongs_to — while
// Version() still reported the same metric version, so existing baselines saw a
// phantom improving delta.
func TestBlastRadiusCountsEveryEdgeKind(t *testing.T) {
	tests := []struct {
		name string
		kind string
	}{
		{"import edge", "imports"},
		{"containment edge", "belongs_to"},
		{"module use edge", "uses_internal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set := relationship.Set{
				Nodes: []relationship.Node{node(modA), node(modB)},
				Edges: []relationship.Edge{edge(modA, modB, test.kind)},
			}
			blast, modules := BlastRadius(set)
			if modules != 2 {
				t.Fatalf("first-party modules = %d, want 2", modules)
			}
			if blast[modB] != 1 {
				t.Errorf("blast[%s] = %d, want 1: %s must reach its target", modB, blast[modB], test.kind)
			}
		})
	}
}

// A transitive chain is condensed, so the far end counts both dependents.
func TestBlastRadiusIsTransitive(t *testing.T) {
	set := relationship.Set{
		Nodes: []relationship.Node{node(modA), node(modB), node(modC)},
		Edges: []relationship.Edge{edge(modA, modB, "imports"), edge(modB, modC, "belongs_to")},
	}
	blast, _ := BlastRadius(set)
	if blast[modC] != 2 {
		t.Errorf("blast[%s] = %d, want 2 (direct + transitive dependent)", modC, blast[modC])
	}
}

// An edge into a node archfit never parsed is an external dependency and must
// not be scored as an owned module.
func TestBlastRadiusExcludesExternalTargets(t *testing.T) {
	set := relationship.Set{
		Nodes: []relationship.Node{node(modA), {ID: "ext", Module: "ext", FirstParty: false}},
		Edges: []relationship.Edge{edge(modA, "ext", "imports")},
	}
	blast, modules := BlastRadius(set)
	if modules != 1 {
		t.Errorf("first-party modules = %d, want only the parsed module", modules)
	}
	if _, ok := blast["ext"]; ok {
		t.Errorf("blast = %v, want no entry for the external target", blast)
	}
}

// TestBlastRadiusUsesDeclaredModuleIdentity pins MOD-2. Two extractor packages
// inside one declared capability remain one module; re-collapsing their IDs
// would fabricate a third architecture boundary.
func TestBlastRadiusUsesDeclaredModuleIdentity(t *testing.T) {
	set := relationship.Set{
		Nodes: []relationship.Node{
			{ID: "file:services/a/one.go", Module: modA, FirstParty: true, BoundaryClassified: true},
			{ID: "file:services/a/two/two.go", Module: modA, FirstParty: true, BoundaryClassified: true},
			{ID: "file:services/b/one.go", Module: modB, FirstParty: true, BoundaryClassified: true},
		},
		Edges: []relationship.Edge{{
			FromID: "file:services/a/two/two.go", ToID: "file:services/b/one.go",
			FromModule: modA, ToModule: modB, Kind: "imports",
		}},
	}
	blast, modules := BlastRadius(set)
	if modules != 2 {
		t.Fatalf("first-party modules = %d, want the two declared modules", modules)
	}
	if blast[modB] != 1 {
		t.Errorf("blast[%s] = %d, want one declared-module dependent", modB, blast[modB])
	}
}
