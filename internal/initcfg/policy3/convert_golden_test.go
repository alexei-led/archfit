package policy3

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition/checkfacts"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// snapshot is what the golden conversions read instead of a tree: the head
// facts and the generated globs of a repository at a pinned commit.
type snapshot struct {
	Generated []string            `json:"generated"`
	Facts     evidence.CheckFacts `json:"facts"`
}

// snapshotFile holds a snapshot, gzip-compressed JSON.
const snapshotFile = "snapshot.json.gz"

// minSnapshotLiteral is the shortest literal a snapshot keeps for its value:
// the duplication rule counts no literal under 12 runes. A snapshot also keeps
// one literal of every other file, because a file the facts name can add a
// test glob (factFiles).
const minSnapshotLiteral = 12

// TestConvertGolden converts the real Archfit 3 files of archfit (main
// 032509b8) and pumba (master 2d64fca) on snapshots of their trees. The
// policy loads and fits the tree (encode checks it); the policy and the "not
// converted" text match the reviewed golden files.
//
// ARCHFIT_UPDATE_INIT=1 rewrites the golden files. To take a new snapshot,
// export the commit (git archive <sha> | tar -x -C <dir>), copy its Archfit 3
// files into testdata/convert/<name>, and run with
// ARCHFIT_CONVERT_SNAPSHOT=<dir> and -run TestConvertGolden/<name>.
func TestConvertGolden(t *testing.T) {
	for _, name := range []string{"archfit", "pumba"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("testdata", "convert", name)
			old, err := readArchfit3(dir)
			if err != nil {
				t.Fatalf("readArchfit3: %v", err)
			}
			if src := os.Getenv("ARCHFIT_CONVERT_SNAPSHOT"); src != "" {
				takeSnapshot(t, old, src, filepath.Join(dir, snapshotFile))
			}
			snap := readSnapshot(t, filepath.Join(dir, snapshotFile))
			conv, data, err := convert(old, snap.Facts, snap.Generated, "unused")
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			pinConvertGolden(t, filepath.Join(dir, "policy.golden.yaml"), data)
			pinConvertGolden(t, filepath.Join(dir, "not-converted.golden.txt"), []byte(conv.Text()))
			if _, err := policyfile.Resolve(data); err != nil {
				t.Errorf("Resolve: %v", err)
			}
		})
	}
}

func takeSnapshot(t *testing.T, old archfit3, src, out string) {
	t.Helper()
	cfg, err := factsConfig(old)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := (&checkfacts.Acquirer{Runner: toolrun.New(), Config: cfg}).Head(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	named := map[string]bool{}
	facts.Literals = slices.DeleteFunc(facts.Literals, func(l evidence.LiteralOccurrence) bool {
		drop := len(l.Value) < minSnapshotLiteral && named[l.File]
		named[l.File] = true
		return drop
	})
	gen, err := generated(src, facts.Sources)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(snapshot{Generated: gen, Facts: facts})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := zw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readSnapshot(t *testing.T, file string) snapshot {
	t.Helper()
	f, err := os.Open(file) //nolint:gosec // a test fixture
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read only
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var s snapshot
	if err := json.NewDecoder(zr).Decode(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func pinConvertGolden(t *testing.T, file string, got []byte) {
	t.Helper()
	if os.Getenv("ARCHFIT_UPDATE_INIT") == "1" {
		if err := os.WriteFile(file, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file) //nolint:gosec // a test fixture
	if err != nil {
		t.Fatalf("%v (run with ARCHFIT_UPDATE_INIT=1)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the conversion (ARCHFIT_UPDATE_INIT=1 rewrites it):\n%s", file, got)
	}
}
