package balance_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/balance"
)

const (
	low    = relationship.VolatilityLow
	medium = relationship.VolatilityMedium
	high   = relationship.VolatilityHigh
)

// bySourceThenTarget holds the expected balance for each source and target volatility.
type bySourceThenTarget map[relationship.Volatility]map[relationship.Volatility]int

// The diagonal is the coupling table of design 6.2: contract or model for a
// use of the public part, functional for a use past it, symmetric for a copy.
func TestOfMatrix(t *testing.T) {
	matrix := map[relationship.Strength]bySourceThenTarget{
		relationship.StrengthContract: {
			low:    {low: 9, medium: 9, high: 9},
			medium: {low: 9, medium: 9, high: 9},
			high:   {low: 9, medium: 9, high: 9},
		},
		relationship.StrengthModel: {
			low:    {low: 8, medium: 7, high: 7},
			medium: {low: 8, medium: 7, high: 7},
			high:   {low: 8, medium: 7, high: 7},
		},
		relationship.StrengthFunctional: {
			low:    {low: 8, medium: 5, high: 2},
			medium: {low: 5, medium: 5, high: 2},
			high:   {low: 2, medium: 2, high: 2},
		},
		relationship.StrengthSymmetric: {
			low:    {low: 8, medium: 5, high: 1},
			medium: {low: 5, medium: 5, high: 1},
			high:   {low: 1, medium: 1, high: 1},
		},
		relationship.StrengthIntrusive: {
			low:    {low: 8, medium: 5, high: 2},
			medium: {low: 5, medium: 5, high: 2},
			high:   {low: 2, medium: 2, high: 2},
		},
	}
	for strength, grid := range matrix {
		for source, row := range grid {
			for target, want := range row {
				got, ok := balance.Of(strength, source, target)
				if !ok || got != want {
					t.Errorf("Of(%s, %s -> %s) = %d, %t; want %d, true", strength, source, target, got, ok, want)
				}
			}
		}
	}
}

// Appendix A: pkg_tc (high) uses or copies a rule of pkg_util (low).
func TestOfWorkedExample(t *testing.T) {
	tests := []struct {
		name     string
		strength relationship.Strength
		want     int
	}{
		{"pkg/tc imports pkg/util", relationship.StrengthContract, 9},
		{"pkg/tc keeps a copy of the rule", relationship.StrengthSymmetric, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := balance.Of(tt.strength, high, low); !ok || got != tt.want {
				t.Errorf("got %d, %t; want %d, true", got, ok, tt.want)
			}
		})
	}
}

func TestOfUnknownStrengthHasNoBalance(t *testing.T) {
	if got, ok := balance.Of(relationship.StrengthUnknown, low, low); ok {
		t.Errorf("Of(unknown) = %d, true; want no balance", got)
	}
}

func TestOfFrozen(t *testing.T) {
	frozen := relationship.VolatilityFrozen
	tests := []struct {
		name           string
		source, target relationship.Volatility
		want           int
	}{
		{"frozen source into low target", frozen, low, 8},
		{"low source into frozen target", low, frozen, 8},
		{"both frozen", frozen, frozen, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := balance.Of(relationship.StrengthFunctional, tt.source, tt.target); !ok || got != tt.want {
				t.Errorf("got %d, %t; want %d, true", got, ok, tt.want)
			}
		})
	}
}

func TestOfUndeclaredVolatilityScoresAsHigh(t *testing.T) {
	tests := []struct {
		name           string
		source, target relationship.Volatility
		want           int
	}{
		{"undeclared target", low, relationship.VolatilityUndeclared, 2},
		{"undeclared source", relationship.VolatilityUndeclared, low, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := balance.Of(relationship.StrengthFunctional, tt.source, tt.target); !ok || got != tt.want {
				t.Errorf("got %d, %t; want %d, true", got, ok, tt.want)
			}
		})
	}
}
