package main

import (
	"encoding/json"
	"testing"
)

func TestReportEvidenceFromCLI(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, with(fixtureFiles(), map[string]string{defaultConfigPath: reportPolicy}), map[string]string{tcNetem: tcImportsUtil})
	got := runCLI(t, fixtureDate, cmdReport, "-C", dir, flagFormat, formatJSON)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	var doc struct {
		SchemaVersion string `json:"schema_version"`
		Evidence      *struct {
			PairPopulation struct {
				Population string `json:"population"`
				TotalEdges int    `json:"total_edges"`
			} `json:"pair_population"`
			Pairs []struct {
				PairID       string            `json:"pair_id"`
				Observations []json.RawMessage `json:"observations"`
			} `json:"pairs"`
			Connascence struct {
				Population string `json:"population"`
				TotalEdges int    `json:"total_edges"`
				Summary    struct {
					ByKind map[string]int `json:"by_kind"`
				} `json:"summary"`
			} `json:"connascence"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != "archfit.architecture-state.v2" || doc.Evidence == nil {
		t.Fatalf("want v2 evidence through actual renderer, got %s evidence=%v", doc.SchemaVersion, doc.Evidence)
	}
	e := doc.Evidence
	if e.PairPopulation.Population != "seam_eligible_set_edges" || e.Connascence.Population != "all_set_edges" || e.Connascence.TotalEdges <= e.PairPopulation.TotalEdges {
		t.Fatalf("mixed-import populations: %+v", e)
	}
	if len(e.Pairs) == 0 || e.PairPopulation.TotalEdges == 0 || e.Connascence.Summary.ByKind["name"] == 0 {
		t.Fatalf("missing actual producer observations/connascence: %+v", e)
	}
}
