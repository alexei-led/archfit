package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestMeasurementIdentityDoesNotDependOnCache(t *testing.T) {
	_, root := materializeFixtureRepo(t, fixtureSingleModule)
	var first json.RawMessage
	for _, extra := range [][]string{nil, nil, {flagRefresh}} {
		args := []string{"analyze", "-c", filepath.Join(root, defaultConfigPath), "--json"}
		args = append(args, extra...)
		var stdout, stderr bytes.Buffer
		if code := RunWithStderr(args, &stdout, &stderr); code != 0 {
			t.Fatalf("exit=%d stderr=%s", code, &stderr)
		}
		var doc struct {
			Comparison struct {
				Profile json.RawMessage `json:"measurement_profile"`
			} `json:"comparison"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Comparison.Profile) == 0 || bytes.Equal(doc.Comparison.Profile, []byte("null")) {
			t.Fatal("measurement profile missing")
		}
		if first == nil {
			first = doc.Comparison.Profile
		} else if !bytes.Equal(first, doc.Comparison.Profile) {
			t.Fatalf("cache execution changed measurement identity: first=%s current=%s", first, doc.Comparison.Profile)
		}
	}
}
