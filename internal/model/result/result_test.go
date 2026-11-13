package result_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/result"
)

// TestGoldensMatchTheTypes reads the golden of every example of design 8.1
// (and the App's waiting document, design 12.5), decodes it strictly into its
// type, and encodes it again. Equal bytes prove the golden has no field the
// type lacks, misses no field the type always writes, and keeps field order.
func TestGoldensMatchTheTypes(t *testing.T) {
	results := []struct {
		name   string
		status result.Status
		errors int
		causes []result.Cause
	}{
		{name: "pass", status: result.StatusPass},
		{name: "deny", status: result.StatusFail, errors: 1},
		{name: "graph", status: result.StatusFail, errors: 1},
		{name: "duplication", status: result.StatusFail, errors: 1},
		{name: "policy_differs", status: result.StatusUnknown, causes: []result.Cause{result.CausePolicy}},
		{name: "expired_exception", status: result.StatusFail, errors: 1},
		{name: "waiting", status: result.StatusWaiting, errors: 1},
		{name: "repository_clean", status: result.StatusPass},
		{name: "repository_drift", status: result.StatusPass},
		{name: "repository_expired", status: result.StatusFail, errors: 1},
	}
	for _, tc := range results {
		t.Run(tc.name, func(t *testing.T) {
			raw := readGolden(t, tc.name)
			var got result.Result
			decodeStrict(t, raw, &got)
			if got.Schema != result.SchemaVersion {
				t.Errorf("schema = %q, want %q", got.Schema, result.SchemaVersion)
			}
			if got.Status != tc.status {
				t.Errorf("status = %q, want %q", got.Status, tc.status)
			}
			if len(got.Errors) != tc.errors {
				t.Errorf("errors = %d, want %d", len(got.Errors), tc.errors)
			}
			if len(got.Unknown) != len(tc.causes) {
				t.Fatalf("unknown = %d, want %d", len(got.Unknown), len(tc.causes))
			}
			for i, cause := range tc.causes {
				if got.Unknown[i].Cause != cause {
					t.Errorf("unknown[%d].cause = %q, want %q", i, got.Unknown[i].Cause, cause)
				}
			}
			assertReencodes(t, raw, got)
		})
	}

	t.Run("can", func(t *testing.T) {
		raw := readGolden(t, "can")
		var got result.CanAnswer
		decodeStrict(t, raw, &got)
		if got.Schema != result.CanSchemaVersion {
			t.Errorf("schema = %q, want %q", got.Schema, result.CanSchemaVersion)
		}
		if got.Answer != result.ReplyNo {
			t.Errorf("answer = %q, want %q", got.Answer, result.ReplyNo)
		}
		if got.Next == nil || got.Next.Note != "" {
			t.Errorf("next = %+v, want a next step without a note", got.Next)
		}
		assertReencodes(t, raw, got)
	})
}

// TestExpiredExceptionIsNotFromTheChange pins the two fields that set an
// expired exception apart from an error the change added.
func TestExpiredExceptionIsNotFromTheChange(t *testing.T) {
	var got result.Result
	decodeStrict(t, readGolden(t, "expired_exception"), &got)
	e := got.Errors[0]
	if e.FromChange || e.Rule != "exception" || e.Exception == nil || e.Exception.Rule != "duplication" {
		t.Errorf("expired exception error = %+v, want rule exception, from_change false, exception.rule duplication", e)
	}

	var copied result.Result
	decodeStrict(t, readGolden(t, "duplication"), &copied)
	if e.Key == copied.Errors[0].Key {
		t.Errorf("expired exception key %q equals the key of the copy it covers: a change that adds one more site would list two errors with one key", e.Key)
	}

	var graph result.Result
	decodeStrict(t, readGolden(t, "graph"), &graph)
	if e := graph.Errors[0]; !e.FromChange || e.Exception != nil || e.Next.Note != "do not copy the code" {
		t.Errorf("graph error = %+v, want from_change true, no exception, the copy note", e)
	}
}

// TestWaitingAsksNothingOfTheAuthor pins design 7.1 and 12.5: on waiting the
// worker's next step is to wait for the owner, never another request.
func TestWaitingAsksNothingOfTheAuthor(t *testing.T) {
	var got result.Result
	decodeStrict(t, readGolden(t, "waiting"), &got)
	for i, e := range got.Errors {
		if e.Next.Kind != result.NextWait || e.Next.Note != "" {
			t.Errorf("errors[%d].next = %+v, want kind wait and no note", i, e.Next)
		}
	}
}

// TestRepositoryState pins design 7.1 and 7.2 in the push-run goldens: drift
// is never from the change, and an expired exception is listed once, in
// errors, beside the state that names it.
func TestRepositoryState(t *testing.T) {
	cases := []struct {
		golden          string
		state           result.RepositoryState
		drift, expiring int
	}{
		{golden: "repository_clean", state: result.RepositoryClean, expiring: 1},
		{golden: "repository_drift", state: result.RepositoryDrift, drift: 1},
		{golden: "repository_expired", state: result.RepositoryExpired},
	}
	for _, c := range cases {
		t.Run(c.golden, func(t *testing.T) {
			var got result.Result
			decodeStrict(t, readGolden(t, c.golden), &got)
			r := got.Repository
			if r == nil {
				t.Fatal("no repository state")
			}
			if r.State != c.state || len(r.Drift) != c.drift || len(r.Expiring) != c.expiring {
				t.Errorf("repository = %+v, want state %s, %d drift, %d expiring", r, c.state, c.drift, c.expiring)
			}
			for _, e := range r.Drift {
				if e.FromChange {
					t.Errorf("drift %s is from the change", e.Key)
				}
			}
			if expired := len(got.Errors) == 1 && got.Errors[0].Rule == "exception"; expired != (c.state == result.RepositoryExpired) {
				t.Errorf("errors = %+v, want the expired exception only in the expired state", got.Errors)
			}
		})
	}
	t.Run("a pull-request result has none", func(t *testing.T) {
		var got result.Result
		decodeStrict(t, readGolden(t, "graph"), &got)
		if got.Repository != nil {
			t.Errorf("repository = %+v, want none", got.Repository)
		}
	})
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name+".json")) //nolint:gosec // fixed golden names
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	return raw
}

func decodeStrict(t *testing.T, raw []byte, v any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func assertReencodes(t *testing.T, raw []byte, v any) {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), raw) {
		t.Errorf("golden differs from the encoded type:\n--- golden\n%s\n--- encoded\n%s", raw, buf.Bytes())
	}
}
