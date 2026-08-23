// Package report defines the stable data-only contract shared by scoring,
// decision synthesis, persistence, and output adapters.
package report

// RubricVersion is the architect scorecard rubric this contract represents.
// Scorecards are comparable only when their rubric versions match.
const RubricVersion = 1

// ScoreBand is a qualitative label for a 0-100 dimension value.
type ScoreBand string

// ScoreBandCritical through ScoreBandStrong define measured score bands.
const (
	ScoreBandCritical    ScoreBand = "critical"
	ScoreBandPoor        ScoreBand = "poor"
	ScoreBandMixed       ScoreBand = "mixed"
	ScoreBandServiceable ScoreBand = "serviceable"
	ScoreBandStrong      ScoreBand = "strong"
	// ScoreBandNA marks a dimension that could not be measured.
	ScoreBandNA ScoreBand = "n/a"
)

// Unmeasured reports whether b marks an unmeasured dimension.
func (b ScoreBand) Unmeasured() bool { return b == ScoreBandNA }

// Confidence describes how trustworthy a dimension assessment is.
type Confidence string

// ConfidenceLow through ConfidenceHigh define evidence confidence levels.
const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

// DimCouplingBalance names the coupling score dimension.
const (
	DimCouplingBalance = "coupling_balance"
)

// Dimension is one scored axis of the architecture.
type Dimension struct {
	Name       string     `json:"name"`
	Value      int        `json:"value"`
	Band       ScoreBand  `json:"band"`
	Confidence Confidence `json:"confidence"`
	Evidence   []string   `json:"evidence"`
	Summary    string     `json:"summary"`
	// Meta marks a review-process dimension.
	Meta bool `json:"meta,omitempty"`
}

// Scorecard is the synthesised banded assessment across all dimensions.
type Scorecard struct {
	RubricVersion int         `json:"rubric_version"`
	Overall       int         `json:"overall"`
	OverallBand   ScoreBand   `json:"overall_band"`
	Dimensions    []Dimension `json:"dimensions"`
}

// DecisionBand is the top-level human decision for a report.
type DecisionBand string

// Decision bands combine the scorecard and active gate state.
const (
	DecisionBandFail           DecisionBand = "FAIL"
	DecisionBandNeedsAttention DecisionBand = "NEEDS_ATTENTION"
	DecisionBandHealthy        DecisionBand = "HEALTHY"
	DecisionBandAcceptable     DecisionBand = "ACCEPTABLE_WITH_WATCH_ITEMS"
)

// Report is the human-decision view model formatted by output adapters.
type Report struct {
	Band            DecisionBand
	Headline        string
	Blocking        int
	Advisory        int
	Overall         int
	OverallBand     ScoreBand
	Dimensions      []DimReport
	Recommendations Recommendations
	Delta           *Delta
}

// DimReport is the presentation view of one scorecard dimension.
type DimReport struct {
	Name       string
	Value      int
	Band       ScoreBand
	Confidence Confidence
	Meta       bool
	Why        string
	WhatMoves  string
}

// Rec is one actionable recommendation.
type Rec struct {
	Title  string
	Detail string
	RuleID string
}

// Recommendations groups findings by urgency tier.
type Recommendations struct {
	MustFix   []Rec
	ShouldFix []Rec
	Watch     []Rec
	Calibrate []Rec
	Ignore    []Rec
}

// Delta holds signed score changes between a base and current scorecard.
type Delta struct {
	Overall    int
	Dimensions []DimDelta
}

// DimDelta is the before, after, and signed change for one dimension.
type DimDelta struct {
	Name   string
	Before int
	After  int
	Change int
}
