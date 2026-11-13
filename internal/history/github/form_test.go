package requestsend

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVendoredLines(t *testing.T) {
	paths, err := filepath.Glob("testdata/line/*.json")
	if err != nil || len(paths) != 3 {
		t.Fatalf("goldens: %v, %d found", err, len(paths))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var r Input
			readJSON(t, path, &r)
			want, err := os.ReadFile(strings.TrimSuffix(path, ".json") + ".txt")
			if err != nil {
				t.Fatal(err)
			}
			if got := Line(r); got != string(want) {
				t.Errorf("line:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestVendoredIssueForm(t *testing.T) {
	var input struct {
		Reason, Record string
		Answer         struct{ From, To string }
	}
	readJSON(t, "testdata/form/can.json", &input)
	r := Input{Reason: input.Reason, Can: &CanRecord{From: input.Answer.From, To: input.Answer.To, Record: input.Record}}
	want, err := os.ReadFile("testdata/form/can.md")
	if err != nil {
		t.Fatal(err)
	}
	if got := issueBody(r); got != string(want) {
		t.Errorf("form:\n%s\nwant:\n%s", got, want)
	}
}

func TestRecordFence(t *testing.T) {
	r := Input{Reason: testReason, Can: &CanRecord{From: "a", To: "b", Record: "a -> b: graph: uses ````literal\n"}}
	got := issueBody(r)
	if !strings.Contains(got, "\n"+strings.Repeat("`", 5)+"text\n") {
		t.Errorf("unsafe fence:\n%s", got)
	}
}

func TestSchemaSource(t *testing.T) {
	source, err := os.ReadFile("testdata/SCHEMA_SOURCE")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "e1933460e828a6fbf8445782346581a52dca5fff") {
		t.Fatal("source commit missing")
	}
	for _, line := range strings.Split(string(source), "\n")[3:] {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		data, err := os.ReadFile(filepath.Join("testdata", fields[1])) //nolint:gosec // paths from the vendored checksum manifest
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != fields[0] {
			t.Errorf("%s: checksum differs", fields[1])
		}
	}
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // fixed package testdata paths
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}
