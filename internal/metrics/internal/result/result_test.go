package result_test

import (
	"math"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}

// ---------------------------------------------------------------------------
// BandScore
// ---------------------------------------------------------------------------

func TestBandScore(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{10.0, result.BandStrong},
		{9.0, result.BandStrong},
		{8.9, result.BandServiceable},
		{7.0, result.BandServiceable},
		{6.9, result.BandMixed},
		{5.0, result.BandMixed},
		{4.9, result.BandPoor},
		{3.0, result.BandPoor},
		{2.9, result.BandCritical},
		{0.0, result.BandCritical},
	}
	for _, c := range cases {
		got := result.BandScore(c.score)
		if got != c.want {
			t.Errorf("BandScore(%.1f) = %q, want %q", c.score, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ApplyConfidenceCap
// ---------------------------------------------------------------------------

func TestApplyConfidenceCap(t *testing.T) {
	cases := []struct {
		band       string
		confidence string
		want       string
	}{
		// high: strong stays strong
		{result.BandStrong, result.ConfidenceHigh, result.BandStrong},
		// medium: strong capped to serviceable
		{result.BandStrong, result.ConfidenceMedium, result.BandServiceable},
		// low: strong capped to mixed
		{result.BandStrong, result.ConfidenceLow, result.BandMixed},
		// low: serviceable capped to mixed
		{result.BandServiceable, result.ConfidenceLow, result.BandMixed},
		// never raises a low band: critical stays critical under high confidence
		{result.BandCritical, result.ConfidenceHigh, result.BandCritical},
		// never raises a low band: poor stays poor under medium confidence
		{result.BandPoor, result.ConfidenceMedium, result.BandPoor},
		// mixed stays mixed under low confidence (already at cap)
		{result.BandMixed, result.ConfidenceLow, result.BandMixed},
	}
	for _, c := range cases {
		got := result.ApplyConfidenceCap(c.band, c.confidence)
		if got != c.want {
			t.Errorf("ApplyConfidenceCap(%q, %q) = %q, want %q", c.band, c.confidence, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ComputeDelta
// ---------------------------------------------------------------------------

func TestComputeDelta(t *testing.T) {
	t.Run("nil baseline returns nil", func(t *testing.T) {
		got := result.ComputeDelta(0.5, nil, "encapsulation", "encapsulation.v1")
		if got != nil {
			t.Errorf("expected nil, got %v", *got)
		}
	})

	t.Run("missing name returns nil", func(t *testing.T) {
		snap := diagnostic.MetricSnapshot{
			"other": {Value: 0.8, Version: "other.v1"},
		}
		got := result.ComputeDelta(0.5, snap, "encapsulation", "encapsulation.v1")
		if got != nil {
			t.Errorf("expected nil for missing name, got %v", *got)
		}
	})

	t.Run("version mismatch returns nil", func(t *testing.T) {
		snap := diagnostic.MetricSnapshot{
			"encapsulation": {Value: 0.8, Version: "encapsulation.v2"},
		}
		got := result.ComputeDelta(0.5, snap, "encapsulation", "encapsulation.v1")
		if got != nil {
			t.Errorf("expected nil for version mismatch, got %v", *got)
		}
	})

	t.Run("match returns current minus baseline", func(t *testing.T) {
		snap := diagnostic.MetricSnapshot{
			"encapsulation": {Value: 0.8, Version: "encapsulation.v1"},
		}
		got := result.ComputeDelta(0.6, snap, "encapsulation", "encapsulation.v1")
		if got == nil {
			t.Fatal("expected non-nil delta")
		}
		if !approxEqual(*got, -0.2) {
			t.Errorf("expected delta -0.2, got %v", *got)
		}
	})
}

// ---------------------------------------------------------------------------
// NACount
// ---------------------------------------------------------------------------

func TestNACount(t *testing.T) {
	def := "some definition"
	got := result.NACount("mymetric", "mymetric.v1", def)

	if got.Band != result.BandNA {
		t.Errorf("Band = %q, want %q", got.Band, result.BandNA)
	}
	if got.Confidence != result.ConfidenceLow {
		t.Errorf("Confidence = %q, want %q", got.Confidence, result.ConfidenceLow)
	}
	if got.Mode != result.ModeCount {
		t.Errorf("Mode = %q, want %q", got.Mode, result.ModeCount)
	}
	if got.Value != 0 {
		t.Errorf("Value = %v, want 0", got.Value)
	}
	if got.Display != result.BandNA {
		t.Errorf("Display = %q, want %q", got.Display, result.BandNA)
	}
	if got.Name != "mymetric" {
		t.Errorf("Name = %q, want mymetric", got.Name)
	}
	if got.Version != "mymetric.v1" {
		t.Errorf("Version = %q, want mymetric.v1", got.Version)
	}
	if got.Definition != def {
		t.Errorf("Definition = %q, want %q", got.Definition, def)
	}
}

// ---------------------------------------------------------------------------
// ShortModule
// ---------------------------------------------------------------------------

func TestShortModule(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// ≤2 segments: returned as-is
		{"foo", "foo"},
		{"foo/bar", "foo/bar"},
		// >2 slash segments: .../last/two
		{"a/b/c", ".../b/c"},
		{"github.com/org/repo/internal/pkg", ".../internal/pkg"},
		// dotted path treated like slashes: a.b.c → split on dots → a/b/c → .../b/c
		{"a.b.c", ".../b/c"},
		// 2-segment dotted path: ≤2 segments, returned as-is
		{"a.b", "a.b"},
	}
	for _, c := range cases {
		got := result.ShortModule(c.input)
		if got != c.want {
			t.Errorf("ShortModule(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
