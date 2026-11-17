package reporttext

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v4/internal/model/report"
)

func TestEvidenceTextRejectsUntrustedControlAndOversizedValues(t *testing.T) {
	t.Parallel()
	doc := report.NewDocument()
	doc.State.Evidence = report.NewReportEvidence()
	hostile := "\x1b[31m\nINJECTED" + strings.Repeat("x", 1000)
	doc.State.Evidence.Snapshot.Stability = hostile
	doc.State.Evidence.Snapshot.PolicyBinding = hostile
	doc.State.Evidence.Snapshot.Unknowns = []report.UnknownFact{{Reason: hostile}}
	doc.State.Evidence.Components = []report.EvidenceComponent{{Name: hostile, Volatility: report.EvidenceStringFact{Origin: hostile}}}
	var out bytes.Buffer
	if err := New().Render(doc, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "INJECTED") || strings.ContainsRune(out.String(), '\x1b') || len(out.String()) > 1000 {
		t.Fatalf("untrusted evidence reached terminal: %q", out.String())
	}
	if !strings.Contains(out.String(), "snapshot: unknown") || !strings.Contains(out.String(), "policy binding: unknown") {
		t.Fatalf("unknown status missing: %s", out.String())
	}
}
