package application

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/report"
)

// The four comparability fingerprints, by their wire names. They qualify a
// comparison, so they have exactly one home — the root comparison block. A
// second copy elsewhere in the document is a second answer to "may these two
// runs be compared", and the two copies would drift.
const (
	keyConfigHash    = "config_hash"
	keyModelHash     = "model_hash"
	keyLabelsHash    = "labels_hash"
	keyRubricVersion = "rubric_version"
)

// gitSource is the volatility-corroboration source name used by the fixtures.
const gitSource = "git"

var fingerprintKeys = []string{keyConfigHash, keyModelHash, keyLabelsHash, keyRubricVersion}

// TestMeasurementCarriesOnlyDeterministicFields pins the measurement block's
// field set. Every field here must be a property of the measured tree, the
// tools, or the history window — never of the run that happened to produce it.
//
// The failure this prevents is silent: one wall-clock timestamp, absolute path,
// or process ID added to this struct makes two identical runs differ in bytes,
// which retires the byte-identity contract every format baseline depends on
// without any test naming the cause.
func TestMeasurementCarriesOnlyDeterministicFields(t *testing.T) {
	want := []string{"history_depth", "history_window", "source_ref", "tool_versions"}

	typ := reflect.TypeFor[report.StateMeasurement]()
	got := make([]string, 0, typ.NumField())
	for field := range typ.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		got = append(got, name)
	}
	sort.Strings(got)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("StateMeasurement fields = %v, want %v.\n"+
			"A new field belongs here only if two identical runs over the same tree "+
			"produce the same value for it; anything run-specific is a diagnostic, not a measurement.",
			got, want)
	}
}

// TestMeasurementNamesTheTreeAndTheWindow asserts the measurement block always
// says what was measured and over what history window, including when there is
// no history at all.
//
// An empty window and a zero depth are indistinguishable from "the field was
// never populated", so a repository with no git history would publish the same
// bytes as one whose scan was simply never wired up.
func TestMeasurementNamesTheTreeAndTheWindow(t *testing.T) {
	t.Run("full run measures the worktree", func(t *testing.T) {
		diagnostic := stateFixture()
		diagnostic.Head = "" // full mode never resolves HEAD

		measurement := ProjectReport(diagnostic, score.Scorecard{}, nil, false).State.Measurement
		if measurement.SourceRef != "worktree" {
			t.Errorf("SourceRef = %q, want worktree: a full run measures files on disk, "+
				"and naming a commit would claim the bytes equal it even on a dirty tree", measurement.SourceRef)
		}
	})

	t.Run("delta run names the resolved ref", func(t *testing.T) {
		measurement := ProjectReport(stateFixture(), score.Scorecard{}, nil, false).State.Measurement
		if measurement.SourceRef != stateHeadRef {
			t.Errorf("SourceRef = %q, want %q", measurement.SourceRef, stateHeadRef)
		}
	})

	t.Run("no history records the window it did not cover", func(t *testing.T) {
		diagnostic := stateFixture()
		diagnostic.VolatilityCorroboration = nil

		state := ProjectReport(diagnostic, score.Scorecard{}, nil, false).State
		if state.Measurement.HistoryWindow != "unavailable" {
			t.Errorf("HistoryWindow = %q, want unavailable", state.Measurement.HistoryWindow)
		}
		if state.Measurement.HistoryDepth != 0 {
			t.Errorf("HistoryDepth = %d, want 0", state.Measurement.HistoryDepth)
		}
		if state.Dimensions.ChangeLocality.Status != report.MeasurementUnmeasured {
			t.Errorf("ChangeLocality.Status = %q, want unmeasured", state.Dimensions.ChangeLocality.Status)
		}
	})

	t.Run("a shallow window is recorded, not rounded away", func(t *testing.T) {
		diagnostic := stateFixture()
		diagnostic.VolatilityCorroboration = &evidence.VolatilityCorroboration{
			Source: gitSource, Status: evidence.StatusPartial, CommitWindow: 20, CommitsScanned: 3,
		}

		measurement := ProjectReport(diagnostic, score.Scorecard{}, nil, false).State.Measurement
		if measurement.HistoryWindow != "20 commits" || measurement.HistoryDepth != 3 {
			t.Errorf("window/depth = %q/%d, want 20 commits/3", measurement.HistoryWindow, measurement.HistoryDepth)
		}
	})
}

// TestFingerprintsLiveOnlyInTheComparisonBlock walks the serialised state and
// asserts each of the four comparability hashes appears at exactly one path.
//
// It walks the wire form rather than the struct because that is what a consumer
// reads: a fingerprint duplicated into a dimension delta or a seam record would
// let two parts of one document disagree about whether the run is comparable.
func TestFingerprintsLiveOnlyInTheComparisonBlock(t *testing.T) {
	diagnostic := stateFixture()
	diagnostic.ModelHash, diagnostic.LabelsHash = "model-hash", "labels-hash"
	diagnostic.Seams = []result.Seam{{
		ID: "seam-1", FromModule: "a", ToModule: "b", LabelEvidenceHash: "evidence-hash",
	}}

	encoded, err := json.Marshal(ProjectReport(diagnostic, score.Scorecard{}, nil, false).State)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	for _, key := range fingerprintKeys {
		paths := jsonPathsOfKey(decoded, key, "")
		if len(paths) != 1 || paths[0] != ".comparison."+key {
			t.Errorf("%s found at %v, want exactly [.comparison.%s]", key, paths, key)
		}
	}

	// The seam's own evidence hash is a different fact — it fingerprints the
	// edges behind one label, not the run — so it stays where it is.
	if got := jsonPathsOfKey(decoded, "label_evidence_hash", ""); len(got) != 1 {
		t.Errorf("label_evidence_hash found at %v, want the one seam record", got)
	}
}

// jsonPathsOfKey lists every dotted path at which key occurs in a decoded JSON
// document. Array elements are traversed without an index so a path names the
// shape rather than one element.
func jsonPathsOfKey(node any, key, path string) []string {
	var out []string
	switch typed := node.(type) {
	case map[string]any:
		names := make([]string, 0, len(typed))
		for name := range typed {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			child := path + "." + name
			if name == key {
				out = append(out, child)
			}
			out = append(out, jsonPathsOfKey(typed[name], key, child)...)
		}
	case []any:
		for _, item := range typed {
			out = append(out, jsonPathsOfKey(item, key, path+"[]")...)
		}
	}
	return out
}
