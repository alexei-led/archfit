package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/view"
)

const (
	testRuleID   = "public_api_only"
	testFrom     = "pkg/a/a.go"
	testTo       = "pkg/b/internal/impl.go"
	testFP       = "deadbeefdeadbeefdeadbeefdeadbeef"
	kindGate     = "gate"
	kindAdvisory = "advisory"
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
		fakeAccepted{},
		view.WaiverSet{},
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
		view.WaiverSet{},
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
	exceptions := view.WaiverSet{
		Waivers: []view.WaiverDef{
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

func TestAssign_ExpiredException(t *testing.T) {
	f := makeFindings(makeEdge())

	// Expired 1 year ago.
	past := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	exceptions := view.WaiverSet{
		Waivers: []view.WaiverDef{
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
		view.WaiverSet{},
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

	gateResult := status.Assign([]finding.Finding{}, base, view.WaiverSet{}, time.Now(), kindGate)
	if len(gateResult) != 1 || gateResult[0].Kind != kindGate {
		t.Errorf("gate pass: want 1 gate fixed finding, got %v", gateResult)
	}

	advResult := status.Assign([]finding.Finding{}, base, view.WaiverSet{}, time.Now(), kindAdvisory)
	if len(advResult) != 1 || advResult[0].Kind != kindAdvisory {
		t.Errorf("advisory pass: want 1 advisory fixed finding, got %v", advResult)
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

	exceptions := view.WaiverSet{
		Waivers: []view.WaiverDef{
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
			wantStatus: finding.StatusWaived,
		},
		{
			name:       "just after expiry boundary",
			now:        endOfExpiryDay.Add(time.Second), // 2025-06-02 00:00:01 UTC
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

// mkDeltaFinding builds a finding with explicit lifecycle/severity/edge fields
// for delta-bucket tests (bypassing finding.New, which derives the ID).
func mkDeltaFinding(id string, st finding.Status, sev finding.Severity, from, to string) finding.Finding {
	return finding.Finding{
		ID:       id,
		RuleID:   testRuleID,
		Kind:     kindGate,
		Status:   st,
		Severity: sev,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Path: from},
			To:   finding.Endpoint{Path: to},
		},
	}
}

func TestDeltaBuckets(t *testing.T) {
	changed := []string{"internal/foo/bar.go"}

	// Each finding exercises one bucket; severities recorded in the accepted set
	// drive the severity_changed detection.
	newF := mkDeltaFinding("11111111111111111111111111111111", finding.StatusNew, finding.SeverityHigh, "internal/foo", "internal/bar")
	existingF := mkDeltaFinding("22222222222222222222222222222222", finding.StatusBaseline, finding.SeverityMedium, "internal/x", "internal/y")
	resolvedF := mkDeltaFinding("33333333333333333333333333333333", finding.StatusFixed, "", "", "")
	sevChangedF := mkDeltaFinding("44444444444444444444444444444444", finding.StatusBaseline, finding.SeverityHigh, "internal/z", "internal/w")
	touchedF := mkDeltaFinding("55555555555555555555555555555555", finding.StatusBaseline, finding.SeverityLow, "internal/foo", "internal/bar")

	accepted := fakeAccepted{
		{Fingerprint: existingF.ID, RuleID: testRuleID, Kind: kindGate, Severity: string(finding.SeverityMedium)},
		{Fingerprint: resolvedF.ID, RuleID: testRuleID, Kind: kindGate, Severity: string(finding.SeverityHigh)},
		{Fingerprint: sevChangedF.ID, RuleID: testRuleID, Kind: kindGate, Severity: string(finding.SeverityLow)}, // was low, now high
		{Fingerprint: touchedF.ID, RuleID: testRuleID, Kind: kindGate, Severity: string(finding.SeverityLow)},
	}

	r := status.DeltaBuckets(
		[]finding.Finding{existingF, newF, touchedF, sevChangedF, resolvedF}, // unordered input
		accepted,
		changed,
	)

	wantBucket := map[string][]string{
		"new":              {newF.ID},
		"existing":         {existingF.ID},
		"resolved":         {resolvedF.ID},
		"severity_changed": {sevChangedF.ID},
		"touched_by_delta": {touchedF.ID},
	}
	got := map[string][]string{
		"new":              r.New,
		"existing":         r.Existing,
		"resolved":         r.Resolved,
		"severity_changed": r.SeverityChanged,
		"touched_by_delta": r.TouchedByDelta,
	}
	for bucket, want := range wantBucket {
		if !slicesEqual(got[bucket], want) {
			t.Errorf("bucket %s: got %v, want %v", bucket, got[bucket], want)
		}
	}
	if r.Empty() {
		t.Error("Empty() = true, want false")
	}
}

func TestDeltaBuckets_Precedence(t *testing.T) {
	changed := []string{"internal/foo/bar.go"}

	// A new finding on a changed file goes to New (not touched_by_delta).
	newOnChanged := mkDeltaFinding("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", finding.StatusNew, finding.SeverityHigh, "internal/foo", "")
	// A fixed finding that is also accepted goes to Resolved (not severity_changed).
	fixedAccepted := mkDeltaFinding("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", finding.StatusFixed, "", "", "")

	accepted := fakeAccepted{
		{Fingerprint: fixedAccepted.ID, RuleID: testRuleID, Kind: kindGate, Severity: string(finding.SeverityLow)},
	}

	r := status.DeltaBuckets([]finding.Finding{newOnChanged, fixedAccepted}, accepted, changed)

	if !slicesEqual(r.New, []string{newOnChanged.ID}) {
		t.Errorf("New: got %v, want [%s]", r.New, newOnChanged.ID)
	}
	if !slicesEqual(r.Resolved, []string{fixedAccepted.ID}) {
		t.Errorf("Resolved: got %v, want [%s]", r.Resolved, fixedAccepted.ID)
	}
	if len(r.TouchedByDelta) != 0 || len(r.SeverityChanged) != 0 {
		t.Errorf("precedence leak: touched=%v severity_changed=%v", r.TouchedByDelta, r.SeverityChanged)
	}
}

func TestDeltaBuckets_UnknownSeverityIsNotChanged(t *testing.T) {
	// A baseline written before severity tracking records an empty severity; the
	// finding must not be flagged severity_changed.
	f := mkDeltaFinding("cccccccccccccccccccccccccccccccc", finding.StatusBaseline, finding.SeverityHigh, "internal/x", "internal/y")
	accepted := fakeAccepted{{Fingerprint: f.ID, RuleID: testRuleID, Kind: kindGate, Severity: ""}}

	r := status.DeltaBuckets([]finding.Finding{f}, accepted, nil)
	if len(r.SeverityChanged) != 0 {
		t.Errorf("SeverityChanged: got %v, want empty", r.SeverityChanged)
	}
	if !slicesEqual(r.Existing, []string{f.ID}) {
		t.Errorf("Existing: got %v, want [%s]", r.Existing, f.ID)
	}
}

func TestDeltaBuckets_ModuleKeyEndpointNotPathEvidence(t *testing.T) {
	// public_api_* findings stamp the bare config module key on both edge
	// endpoints (recorded in MatchedBy); real files live in Locations. A key
	// that collides with an unrelated real path must not bucket the finding as
	// touched, while a changed location file must.
	const locFile = "src/domain/api.go"
	tests := []struct {
		name    string
		locs    []graph.Location
		changed []string
		touched bool
	}{
		{"module key colliding with unrelated changed path", []graph.Location{{File: locFile}}, []string{"docs/readme.md"}, false},
		{"changed location file", []graph.Location{{File: locFile}}, []string{locFile}, true},
		{"no locations keeps endpoint fallback", nil, []string{"docs/readme.md"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := mkDeltaFinding("dddddddddddddddddddddddddddddddd", finding.StatusBaseline, finding.SeverityMedium, "docs", "docs")
			f.MatchedBy = map[string]string{"module": "docs"}
			f.Locations = tc.locs
			accepted := fakeAccepted{{Fingerprint: f.ID, RuleID: testRuleID, Kind: kindGate, Severity: string(finding.SeverityMedium)}}

			r := status.DeltaBuckets([]finding.Finding{f}, accepted, tc.changed)

			if got := len(r.TouchedByDelta) == 1; got != tc.touched {
				t.Errorf("touched=%v, want %v (result %+v)", got, tc.touched, r)
			}
		})
	}

	t.Run("general two-phase path: location misses, real edge endpoint matches", func(t *testing.T) {
		// Locations names a file that is NOT in changed (phase 1 misses, but
		// hasLoc becomes true); an edge endpoint names a real path DISTINCT from
		// the module key that IS in changed — phase 2 must still match it (the
		// modKey-equality skip only applies to the module-key value itself).
		f := mkDeltaFinding("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", finding.StatusBaseline, finding.SeverityMedium,
			"internal/realmodule/thing.go", "docs")
		f.MatchedBy = map[string]string{"module": "docs"}
		f.Locations = []graph.Location{{File: "src/unrelated/file.go"}}
		accepted := fakeAccepted{{Fingerprint: f.ID, RuleID: testRuleID, Kind: kindGate, Severity: string(finding.SeverityMedium)}}

		changed := []string{"internal/realmodule/thing.go"}
		r := status.DeltaBuckets([]finding.Finding{f}, accepted, changed)

		if len(r.TouchedByDelta) != 1 {
			t.Errorf("touched=%v, want true (result %+v)", len(r.TouchedByDelta) == 1, r)
		}
	})
}

func TestDeltaBuckets_Empty(t *testing.T) {
	r := status.DeltaBuckets(nil, fakeAccepted{}, nil)
	if !r.Empty() {
		t.Errorf("Empty() = false, want true for no findings: %+v", r)
	}
}

func TestDeltaBuckets_Sorted(t *testing.T) {
	// Two new findings in reverse-ID order must come back sorted by ID.
	hi := mkDeltaFinding("ffffffffffffffffffffffffffffffff", finding.StatusNew, finding.SeverityLow, "", "")
	lo := mkDeltaFinding("00000000000000000000000000000000", finding.StatusNew, finding.SeverityLow, "", "")
	r := status.DeltaBuckets([]finding.Finding{hi, lo}, fakeAccepted{}, nil)
	if !slicesEqual(r.New, []string{lo.ID, hi.ID}) {
		t.Errorf("New not sorted: got %v", r.New)
	}
}

func slicesEqual(a, b []string) bool {
	return slices.Equal(a, b)
}
