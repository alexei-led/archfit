package status_test

import (
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/status"
)

const (
	testRuleID = "public_api_only"
	testFrom   = "pkg/a/a.go"
	testTo     = "pkg/b/internal/impl.go"
	testFP     = "deadbeefdeadbeefdeadbeefdeadbeef"
)

// makeEdge returns a graph.Edge with the standard test from/to node IDs and uses_internal kind.
func makeEdge() graph.Edge {
	return graph.Edge{
		From: "file:" + testFrom,
		To:   "file:" + testTo,
		Kind: graph.EdgeKindUsesInternal,
	}
}

// makeFindings creates a Finding via finding.New for the standard test edge.
func makeFindings(e graph.Edge) finding.Finding {
	return finding.New(testRuleID, e, nil)
}

func TestAssign_NewFinding(t *testing.T) {
	f := makeFindings(makeEdge())

	result := status.Assign(
		[]finding.Finding{f},
		baseline.Baseline{},
		config.ExceptionSet{},
		time.Now(),
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

	base := baseline.Baseline{
		SchemaVersion: baseline.SchemaVersion,
		Accepted: []baseline.AcceptedFinding{
			{Fingerprint: f.ID, RuleID: testRuleID},
		},
	}

	result := status.Assign(
		[]finding.Finding{f},
		base,
		config.ExceptionSet{},
		time.Now(),
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
	exceptions := config.ExceptionSet{
		Exceptions: []config.ExceptionDef{
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
		baseline.Baseline{},
		exceptions,
		time.Now(),
	)

	if len(result) != 1 {
		t.Fatalf("want 1 finding, got %d", len(result))
	}
	if result[0].Status != finding.StatusExcepted {
		t.Errorf("want status %q, got %q", finding.StatusExcepted, result[0].Status)
	}
}

func TestAssign_ExpiredException(t *testing.T) {
	f := makeFindings(makeEdge())

	// Expired 1 year ago.
	past := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	exceptions := config.ExceptionSet{
		Exceptions: []config.ExceptionDef{
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
		baseline.Baseline{},
		exceptions,
		time.Now(),
	)

	if len(result) != 1 {
		t.Fatalf("want 1 finding, got %d", len(result))
	}
	if result[0].Status != finding.StatusExpiredExcept {
		t.Errorf("want status %q, got %q", finding.StatusExpiredExcept, result[0].Status)
	}
}

func TestAssign_FixedFinding(t *testing.T) {
	// Baseline references a fingerprint that is NOT in the current findings.
	base := baseline.Baseline{
		SchemaVersion: baseline.SchemaVersion,
		Accepted: []baseline.AcceptedFinding{
			{Fingerprint: testFP, RuleID: testRuleID},
		},
	}

	// No current findings.
	result := status.Assign(
		[]finding.Finding{},
		base,
		config.ExceptionSet{},
		time.Now(),
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

func TestAssign_ExpiryBoundary(t *testing.T) {
	f := makeFindings(makeEdge())

	// Use a fixed reference date for the boundary test.
	// Expiry date is 2025-06-01; exception is valid on 2025-06-01 itself,
	// expires at end-of-day (2025-06-01 + 24h). So:
	//   - now = 2025-06-02 00:00:00 exactly → still valid (not after 2025-06-02 00:00:00)
	//   - now = 2025-06-02 00:00:01 → expired
	expiryDate := "2025-06-01"
	endOfExpiryDay := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC) // expiry + 24h

	exceptions := config.ExceptionSet{
		Exceptions: []config.ExceptionDef{
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
			now:        endOfExpiryDay.Add(-time.Second), // 2025-06-01 23:59:59 UTC
			wantStatus: finding.StatusExcepted,
		},
		{
			name:       "just after expiry boundary",
			now:        endOfExpiryDay.Add(time.Second), // 2025-06-02 00:00:01 UTC
			wantStatus: finding.StatusExpiredExcept,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := status.Assign(
				[]finding.Finding{f},
				baseline.Baseline{},
				exceptions,
				tc.now,
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
