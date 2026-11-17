package main

import (
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/v4/internal/model/report"
)

const capturedReference = "captured"

func TestReportPairsJoinSeamsAndCaptureReferences(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, with(fixtureFiles(), map[string]string{defaultConfigPath: reportPolicy}), map[string]string{tcNetem: tcImportsUtil})
	got := runCLI(t, fixtureDate, cmdReport, "-C", dir, flagFormat, formatJSON)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	var state report.ArchitectureState
	if err := json.Unmarshal([]byte(got.stdout), &state); err != nil {
		t.Fatal(err)
	}
	seams := map[string]int{}
	for _, s := range state.Seams {
		seams[s.ID] = s.Edges
	}
	edges, sites := 0, 0
	for _, pair := range state.Evidence.Pairs {
		if n, ok := seams[pair.PairID]; !ok || n != len(pair.Observations) {
			t.Fatalf("pair does not join seam: %+v", pair)
		}
		delete(seams, pair.PairID)
		if pair.InterfaceInventory.Status != "unknown" {
			t.Fatalf("invented interface inventory: %+v", pair)
		}
		edges += len(pair.Observations)
		for _, obs := range pair.Observations {
			for _, site := range obs.Sites {
				if site.Status != capturedReference || site.SHA256 == nil || site.Path == "" || site.Line != nil && *site.Line <= 0 {
					t.Fatalf("invalid site reference: %+v", site)
				}
				sites++
			}
		}
	}
	if edges != state.Evidence.PairPopulation.TotalEdges || len(seams) != 0 || sites == 0 {
		t.Fatalf("pair joins: edges=%d sites=%d missing=%v", edges, sites, seams)
	}
	if state.Evidence.Scope.PolicyRef.Status != capturedReference || state.Evidence.Scope.PolicyRef.SHA256 == nil {
		t.Fatalf("policy reference: %+v", state.Evidence.Scope.PolicyRef)
	}
	for _, component := range state.Evidence.Components {
		if !component.Declared {
			continue
		}
		for _, ref := range component.Volatility.Refs {
			if ref.Selector == "" || ref.Status != capturedReference || ref.SHA256 == nil || ref.Path != defaultConfigPath {
				t.Fatalf("policy selector: %+v", ref)
			}
		}
	}
}
