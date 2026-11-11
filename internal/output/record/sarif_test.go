package record_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/output/record"
	"github.com/alexei-led/archfit/v3/internal/testutil"
)

// TestSARIFGoldensMatchDesignExamples renders the result of every check
// example of design 8.1 and compares it byte for byte with its golden. The
// goldens are written from the design, not generated.
func TestSARIFGoldensMatchDesignExamples(t *testing.T) {
	for _, name := range checkExamples {
		t.Run(name, func(t *testing.T) {
			var r result.Result
			decodeExample(t, name, &r)
			var buf bytes.Buffer
			if err := record.ResultSARIF(&buf, r); err != nil {
				t.Fatalf("ResultSARIF: %v", err)
			}
			path := filepath.Join("testdata", name+".sarif.json")
			want, err := os.ReadFile(path) //nolint:gosec // fixed golden names
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), want) {
				t.Errorf("SARIF differs from %s:\n--- got\n%s--- golden\n%s", path, buf.Bytes(), want)
			}
			if words := testutil.NotUsedIn(string(want)); len(words) > 0 {
				t.Errorf("%s uses %q (design 5, the Not used column)", path, words)
			}
		})
	}
}

const (
	policyFile   = ".archfit.yaml"
	fileC        = "c.go"
	ruleCoupling = "coupling"
	hint         = "a hint"
	requestCmd   = "archfit request"
)

// sarifOut is the part of a SARIF log the mapping sets.
type sarifOut struct {
	Version string `json:"version"`
	Runs    []struct {
		Tool struct {
			Driver struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"driver"`
		} `json:"tool"`
		Invocations []struct {
			ExecutionSuccessful bool `json:"executionSuccessful"`
			Notifications       []struct {
				Level     string       `json:"level"`
				Message   sarifMessage `json:"message"`
				Locations []sarifLoc   `json:"locations"`
			} `json:"toolExecutionNotifications"`
		} `json:"invocations"`
		Results []struct {
			RuleID              string            `json:"ruleId"`
			Level               string            `json:"level"`
			Message             sarifMessage      `json:"message"`
			Locations           []sarifLoc        `json:"locations"`
			PartialFingerprints map[string]string `json:"partialFingerprints"`
		} `json:"results"`
	} `json:"runs"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLoc struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
		Region *struct {
			StartLine int `json:"startLine"`
		} `json:"region"`
	} `json:"physicalLocation"`
}

func renderSARIF(t *testing.T, r result.Result) (sarifOut, string) {
	t.Helper()
	var buf bytes.Buffer
	if err := record.ResultSARIF(&buf, r); err != nil {
		t.Fatalf("ResultSARIF: %v", err)
	}
	var out sarifOut
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal SARIF: %v\n%s", err, buf.Bytes())
	}
	if out.Version != "2.1.0" || len(out.Runs) != 1 || len(out.Runs[0].Invocations) != 1 {
		t.Fatalf("want one SARIF 2.1.0 run with one invocation:\n%s", buf.Bytes())
	}
	return out, buf.String()
}

func TestSARIFMapping(t *testing.T) {
	t.Run("a pass has no result and no notification", func(t *testing.T) {
		out, raw := renderSARIF(t, result.Result{Status: result.StatusPass, Summary: passLine, Engine: engine})
		run := out.Runs[0]
		if run.Results == nil || len(run.Results) != 0 {
			t.Errorf("want results: [], got:\n%s", raw)
		}
		if inv := run.Invocations[0]; !inv.ExecutionSuccessful || len(inv.Notifications) != 0 {
			t.Errorf("want a successful run with no notification, got:\n%s", raw)
		}
		if d := run.Tool.Driver; d.Name != "archfit" || d.Version != engine {
			t.Errorf("driver = %+v, want archfit %s", d, engine)
		}
	})

	t.Run("one result per error, in order, with its rule, line and key", func(t *testing.T) {
		out, _ := renderSARIF(t, result.Result{Status: result.StatusFail, Errors: []result.Error{
			{Path: "b.go", Line: 2, Rule: ruleGraph, What: "w1", Key: "k1", Next: result.Next{Text: fixCode}},
			{Path: "a.go", Line: 1, Rule: "cycle", What: "w2", Key: "k2", Next: result.Next{Text: fixCode}},
		}})
		got := make([]string, 0, len(out.Runs[0].Results))
		for _, res := range out.Runs[0].Results {
			loc := res.Locations[0].PhysicalLocation
			got = append(got, res.RuleID+" "+res.Level+" "+loc.ArtifactLocation.URI+" "+strconv.Itoa(loc.Region.StartLine)+" "+res.PartialFingerprints["archfitKey/v1"])
		}
		want := []string{"graph error b.go 2 k1", "cycle error a.go 1 k2"}
		if !slices.Equal(got, want) {
			t.Errorf("results = %q, want %q", got, want)
		}
	})

	t.Run("an error of the policy file has its path and no region", func(t *testing.T) {
		out, _ := renderSARIF(t, result.Result{Status: result.StatusFail, Errors: []result.Error{
			{Path: policyFile, Rule: "exception", What: "w", Next: result.Next{Text: "wait for an owner: #431"}},
		}})
		loc := out.Runs[0].Results[0].Locations[0].PhysicalLocation
		if loc.ArtifactLocation.URI != policyFile || loc.Region != nil {
			t.Errorf("location = %+v, want .archfit.yaml with no region", loc)
		}
	})

	t.Run("the message holds the record's lines in the record's order", func(t *testing.T) {
		out, _ := renderSARIF(t, result.Result{Status: result.StatusFail, Errors: []result.Error{{
			Path: file, Line: 5, Rule: ruleCoupling, What: "w", Why: "y", Instead: "i",
			Balance: &result.Balance{Value: 3, Min: 7}, Next: result.Next{Text: requestCmd, Note: hint},
		}}})
		want := "w\nwhy: y\ninstead: i\nbalance: 3/10 (min 7)\nnext: archfit request     (a hint)"
		if got := out.Runs[0].Results[0].Message.Text; got != want {
			t.Errorf("message:\n--- got\n%s\n--- want\n%s", got, want)
		}
	})

	t.Run("an error without a key has no fingerprint", func(t *testing.T) {
		out, raw := renderSARIF(t, result.Result{Status: result.StatusFail, Errors: []result.Error{
			{Path: file, Line: 5, Rule: ruleGraph, What: "w", Next: result.Next{Text: fixCode}},
		}})
		if out.Runs[0].Results[0].PartialFingerprints != nil {
			t.Errorf("want no partialFingerprints, got:\n%s", raw)
		}
	})

	t.Run("unknown reasons are notifications, not results", func(t *testing.T) {
		out, raw := renderSARIF(t, result.Result{Status: result.StatusUnknown, Unknown: []result.Unknown{
			{Cause: result.CauseBase, What: "no merge base", Why: "nothing to compare with", Next: result.Next{Text: "git fetch --unshallow"}},
			{Path: fileC, Line: 3, Cause: result.CauseImport, What: "an import cannot be resolved", Next: result.Next{Text: "fix the import"}},
		}})
		run := out.Runs[0]
		if len(run.Results) != 0 {
			t.Errorf("want no result, got:\n%s", raw)
		}
		inv := run.Invocations[0]
		if inv.ExecutionSuccessful {
			t.Error("an unknown status is an execution that did not succeed")
		}
		if len(inv.Notifications) != 2 {
			t.Fatalf("want 2 notifications, got:\n%s", raw)
		}
		first, second := inv.Notifications[0], inv.Notifications[1]
		if first.Level != "warning" || first.Message.Text != "base: no merge base\nwhy: nothing to compare with\nnext: git fetch --unshallow" || first.Locations != nil {
			t.Errorf("first notification = %+v", first)
		}
		loc := second.Locations[0].PhysicalLocation
		if second.Message.Text != "import: an import cannot be resolved\nnext: fix the import" ||
			loc.ArtifactLocation.URI != fileC || loc.Region == nil || loc.Region.StartLine != 3 {
			t.Errorf("second notification = %+v", second)
		}
	})
}
