package main

import (
	"encoding/json"
	"testing"
)

func TestReportConnascenceFromRealGoProducer(t *testing.T) {
	t.Parallel()
	files := with(fixtureFiles(), map[string]string{
		"pkg/util/util.go": "package util\n\ntype Value struct { Name string }\nfunc New() Value { return Value{} }\n",
		tcNetem:            "package tc\n\nimport (\n \"fmt\"\n \"example.com/pumba/pkg/util\"\n)\nfunc Netem() string { var v util.Value = util.New(); return fmt.Sprint(v.Name) }\n",
	})
	dir := fixtureRepo(t, files, nil)
	got := runCLI(t, fixtureDate, cmdReport, "-C", dir, flagFormat, formatJSON)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	var doc struct {
		Evidence struct {
			Connascence struct {
				Summary struct {
					ByKind  map[string]int `json:"by_kind"`
					Roadmap []struct {
						Kind          string `json:"kind"`
						CurrentStatus string `json:"current_status"`
					} `json:"roadmap"`
				} `json:"summary"`
			} `json:"connascence"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatal(err)
	}
	summary := doc.Evidence.Connascence.Summary
	if summary.ByKind["name"] == 0 || summary.ByKind["type"] == 0 {
		t.Fatalf("actual Go producer did not report name/type: %+v", summary)
	}
	statuses := map[string]string{}
	for _, item := range summary.Roadmap {
		statuses[item.Kind] = item.CurrentStatus
	}
	if statuses["name"] != "deterministic_static" || statuses["type"] != "deterministic_static" || statuses["meaning"] != "unmeasured_static" || statuses["algorithm"] != "unmeasured_static" {
		t.Fatalf("roadmap: %+v", statuses)
	}
}
