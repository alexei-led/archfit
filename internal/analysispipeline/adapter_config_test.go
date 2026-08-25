package pipeline

import (
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
)

func TestOptionsThreadsOneMergedExclusionSetThroughEveryStage(t *testing.T) {
	options := Options(config.Config{Exclude: []string{"generated/**", "!generated/keep.go"}})
	goConfig := options.Extractors[config.LangGo]

	if !slices.Equal(goConfig.Exclusions, options.Exclusions) {
		t.Fatalf("Go extractor exclusions = %v, want shared merged exclusions %v", goConfig.Exclusions, options.Exclusions)
	}
	if !slices.Equal(options.Acquisition.Exclusions, options.Exclusions) {
		t.Fatalf("acquisition exclusions = %v, want shared merged exclusions %v", options.Acquisition.Exclusions, options.Exclusions)
	}
}
