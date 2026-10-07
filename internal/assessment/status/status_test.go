package status_test

import (
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	testRuleID       = "public_api_only"
	testFrom         = "pkg/a/a.go"
	testTo           = "pkg/b/internal/impl.go"
	testFP           = "deadbeefdeadbeefdeadbeefdeadbeef"
	testFutureExpiry = "2099-01-01"
	kindGate         = "gate"
	kindAdvisory     = "advisory"
)

// fakeAccepted is an in-memory status.AcceptedSet — status tests exercise the
// interface seam, not the baseline persistence type.
type fakeAccepted []status.AcceptedEntry

func (f fakeAccepted) HasFingerprint(fp string) bool {
	for _, e := range f {
		if e.Fingerprint == fp {
			return true
		}
	}
	return false
}

func (f fakeAccepted) Entries() []status.AcceptedEntry { return f }

// makeEdge returns a relationship.Edge with the standard test from/to node IDs and uses_internal kind.
func makeEdge() relationship.Edge {
	return relationship.Edge{
		FromID: "file:" + testFrom,
		ToID:   "file:" + testTo,
		Kind:   "uses_internal",
	}
}

// makeFindings creates a Finding via finding.New for the standard test edge.
func makeFindings(e relationship.Edge) finding.Finding {
	return finding.New(testRuleID, e, nil)
}

func TestAssign_NewFinding(t *testing.T) {
	f := makeFindings(makeEdge())

	result := status.Assign(
		[]finding.Finding{f},
		fakeAccepted{},
		policy.WaiverSet{},
		time.Now(),
		kindGate,
	)

	if len(result) != 1 {
		t.Fatalf("want 1 finding, got %d", len(result))
	}
	if result[0].Status != finding.StatusNew {
		t.Errorf("want status %q, got %q", finding.StatusNew, result[0].Status)
	}
}

func TestAssign_BaselineFinding(t *testing.T) {
	f := makeFindings(makeEdge())

	base := fakeAccepted{
		{Fingerprint: f.ID, RuleID: testRuleID, Kind: kindGate},
	}

	result := status.Assign(
		[]finding.Finding{f},
		base,
		policy.WaiverSet{},
		time.Now(),
		kindGate,
	)

	if len(result) != 1 {
		t.Fatalf("want 1 finding, got %d", len(result))
	}
	if result[0].Status != finding.StatusBaseline {
		t.Errorf("want status %q, got %q", finding.StatusBaseline, result[0].Status)
	}
}

func TestAssign_ActiveException(t *testing.T) {
	f := makeFindings(makeEdge())

	// Expires 1 year from now — active (not expired).
	future := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	exceptions := policy.WaiverSet{
		Waivers: []policy.WaiverDef{
			{
				Rule:    testRuleID,
				From:    testFrom,
				To:      testTo,
				Expires: future,
			},
		},
	}

	result := status.Assign(
		[]finding.Finding{f},
		fakeAccepted{},
		exceptions,
		time.Now(),
		kindGate,
	)

	if len(result) != 1 {
		t.Fatalf("want 1 finding, got %d", len(result))
	}
	if result[0].Status != finding.StatusWaived {
		t.Errorf("want status %q, got %q", finding.StatusWaived, result[0].Status)
	}
}

func TestAssign_ActiveWaiverWinsRegardlessOfOrder(t *testing.T) {
	f := makeFindings(makeEdge())
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	expired := policy.WaiverDef{Rule: testRuleID, From: testFrom, To: testTo, Expires: "2020-01-01"}
	active := policy.WaiverDef{Rule: testRuleID, From: testFrom, To: testTo, Expires: testFutureExpiry}
	for _, waivers := range []policy.WaiverSet{{Waivers: []policy.WaiverDef{expired, active}}, {Waivers: []policy.WaiverDef{active, expired}}} {
		result := status.Assign([]finding.Finding{f}, fakeAccepted{}, waivers, now, kindGate)
		if len(result) != 1 {
			t.Fatalf("want 1 finding, got %d", len(result))
		}
		if result[0].Status != finding.StatusWaived {
			t.Errorf("want status %q, got %q", finding.StatusWaived, result[0].Status)
		}
	}
}

func TestAssign_SyntheticWaiverMatchesModuleEndpoints(t *testing.T) {
	f := finding.Finding{
		ID: "synthetic", RuleID: "labels/stale", Kind: kindAdvisory,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Module: "checkout"},
			To:   finding.Endpoint{Module: "pricing"},
		},
	}
	waivers := policy.WaiverSet{Waivers: []policy.WaiverDef{{
		Rule: "labels/stale", From: "checkout", To: "pricing", Expires: testFutureExpiry,
	}}}
	result := status.Assign([]finding.Finding{f}, fakeAccepted{}, waivers, time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC), kindGate)
	if len(result) != 1 || result[0].Status != finding.StatusWaived {
		t.Fatalf("got %+v, want one waived finding", result)
	}
}

func TestAssign_EdgelessSyntheticRuleOnlyWaiver(t *testing.T) {
	f := finding.Finding{
		ID: "uncovered", RuleID: "map/uncovered_path", Kind: kindAdvisory,
		MatchedBy: map[string]string{"subject": "pkg/orphan"},
	}
	waivers := policy.WaiverSet{Waivers: []policy.WaiverDef{{
		Rule: "map/uncovered_path", Expires: testFutureExpiry,
	}}}
	result := status.Assign([]finding.Finding{f}, fakeAccepted{}, waivers, time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC), kindAdvisory)
	if len(result) != 1 || result[0].Status != finding.StatusWaived {
		t.Fatalf("got %+v, want one waived finding", result)
	}
}

func TestAssign_ExpiredException(t *testing.T) {
	f := makeFindings(makeEdge())

	// Expired 1 year ago.
	past := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	exceptions := policy.WaiverSet{
		Waivers: []policy.WaiverDef{
			{
				Rule:    testRuleID,
				From:    testFrom,
				To:      testTo,
				Expires: past,
			},
		},
	}

	result := status.Assign(
		[]finding.Finding{f},
		fakeAccepted{},
		exceptions,
		time.Now(),
		kindGate,
	)

	if len(result) != 1 {
		t.Fatalf("want 1 finding, got %d", len(result))
	}
	if result[0].Status != finding.StatusExpiredWaiver {
		t.Errorf("want status %q, got %q", finding.StatusExpiredWaiver, result[0].Status)
	}
}

func TestAssign_FixedFinding(t *testing.T) {
	// Baseline references a gate fingerprint that is NOT in the current findings.
	base := fakeAccepted{
		{Fingerprint: testFP, RuleID: testRuleID, Kind: kindGate},
	}

	// No current findings.
	result := status.Assign(
		[]finding.Finding{},
		base,
		policy.WaiverSet{},
		time.Now(),
		kindGate,
	)

	if len(result) != 1 {
		t.Fatalf("want 1 fixed finding, got %d", len(result))
	}
	if result[0].Status != finding.StatusFixed {
		t.Errorf("want status %q, got %q", finding.StatusFixed, result[0].Status)
	}
	if result[0].ID != testFP {
		t.Errorf("want ID %q, got %q", testFP, result[0].ID)
	}
	if result[0].RuleID != testRuleID {
		t.Errorf("want RuleID %q, got %q", testRuleID, result[0].RuleID)
	}
}

func TestAssign_FixedFindingKindFilter(t *testing.T) {
	// Baseline has both gate and advisory entries; only gate should be emitted as fixed
	// when forKind==kindGate, and vice versa.
	base := fakeAccepted{
		{Fingerprint: testFP, RuleID: testRuleID, Kind: kindGate},
		{Fingerprint: "aabbccddaabbccddaabbccddaabbccdd", RuleID: "bc/imbalanced_coupling", Kind: kindAdvisory},
	}

	gateResult := status.Assign([]finding.Finding{}, base, policy.WaiverSet{}, time.Now(), kindGate)
	if len(gateResult) != 1 || gateResult[0].Kind != kindGate {
		t.Errorf("gate pass: want 1 gate fixed finding, got %v", gateResult)
	}

	advResult := status.Assign([]finding.Finding{}, base, policy.WaiverSet{}, time.Now(), kindAdvisory)
	if len(advResult) != 1 || advResult[0].Kind != kindAdvisory {
		t.Errorf("advisory pass: want 1 advisory fixed finding, got %v", advResult)
	}
}

func TestAssign_ExpiryBoundary(t *testing.T) {
	f := makeFindings(makeEdge())

	expiryDate := "2025-06-01"
	endOfExpiryDay := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)

	exceptions := policy.WaiverSet{
		Waivers: []policy.WaiverDef{
			{
				Rule:    testRuleID,
				From:    testFrom,
				To:      testTo,
				Expires: expiryDate,
			},
		},
	}

	tests := []struct {
		name       string
		now        time.Time
		wantStatus finding.Status
	}{
		{
			name:       "just before expiry boundary",
			now:        endOfExpiryDay.Add(-time.Second),
			wantStatus: finding.StatusWaived,
		},
		{
			name:       "at expiry boundary",
			now:        endOfExpiryDay,
			wantStatus: finding.StatusExpiredWaiver,
		},
		{
			name:       "just after expiry boundary",
			now:        endOfExpiryDay.Add(time.Second),
			wantStatus: finding.StatusExpiredWaiver,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := status.Assign(
				[]finding.Finding{f},
				fakeAccepted{},
				exceptions,
				tc.now,
				kindGate,
			)
			if len(result) != 1 {
				t.Fatalf("want 1 finding, got %d", len(result))
			}
			if result[0].Status != tc.wantStatus {
				t.Errorf("want status %q, got %q", tc.wantStatus, result[0].Status)
			}
		})
	}
}
