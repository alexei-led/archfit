package application

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/model/report"
)

func TestBaselineSeamSnapshotPresence(t *testing.T) {
	for _, tc := range []struct {
		name, snapshot string
		comparable     bool
	}{
		{"missing", `{}`, false},
		{"null", `{"QualifyingSeamIDs":null}`, false},
		{"empty", `{"QualifyingSeamIDs":[]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := matchingSnapshot()
			snapshot.QualifyingSeamIDs = nil
			if err := json.Unmarshal([]byte(tc.snapshot), snapshot); err != nil {
				t.Fatal(err)
			}
			anchor := seamAnchor(Baseline{State: snapshot}, headContext())
			if anchor.SeamsComparable != tc.comparable {
				t.Fatalf("anchor = %+v", anchor)
			}
			if !tc.comparable && !strings.Contains(strings.Join(anchor.SnapshotMismatches, ";"), "qualifying_seam_ids") {
				t.Fatalf("missing reason: %+v", anchor)
			}
		})
	}
}

func TestGateReferenceIsVisibleWithoutBaseComparison(t *testing.T) {
	r := result.New()
	r.MeasurementProfile = referenceProfile()
	snapshot := matchingSnapshot()
	snapshot.MeasurementProfile = nil
	r.GateReference = baselineComparison(Baseline{State: snapshot}, headContext())
	doc := ProjectReport(r, score.Scorecard{})
	if doc.State.Comparison.Status != report.ComparisonNotRequested {
		t.Fatalf("display comparison unexpectedly requested: %+v", doc.State.Comparison)
	}
	if doc.State.GateReference == nil || doc.State.GateReference.Status != report.ComparisonNonComparable {
		t.Fatalf("missing gate reference: %+v", doc.State.GateReference)
	}
	if !strings.Contains(strings.Join(doc.State.GateReference.Reasons, ";"), "measurement_profile") {
		t.Fatal("profile reason lost")
	}
	if doc.State.Comparison.MeasurementProfile == nil {
		t.Fatal("measurement identity lost in projection")
	}
}
