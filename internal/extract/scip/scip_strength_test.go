package scip

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseReaderEdges(t *testing.T) {
	tests := []struct {
		name    string
		stdout  string
		wantKey string
		wantVal string
		wantLen int
		wantErr bool
	}{
		{
			name:    "edges parsed and keyed by from\\x00to",
			stdout:  `{"edges":[{"from":"ccgram.handlers.hook_events","to":"ccgram.telegram_client","strength":"contract"},{"from":"ccgram.bootstrap","to":"ccgram.hook","strength":"intrusive"}]}`,
			wantKey: "ccgram.handlers.hook_events\x00ccgram.telegram_client",
			wantVal: "contract",
			wantLen: 2,
		},
		{
			name:    "helper error fails parse",
			stdout:  `{"error":"scip bindings: boom","edges":[]}`,
			wantErr: true,
		},
		{
			name:    "malformed json fails parse",
			stdout:  `not json`,
			wantErr: true,
		},
		{
			name:    "empty edge list yields empty map",
			stdout:  `{"edges":[]}`,
			wantLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseReaderEdges([]byte(tc.stdout))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(m) != tc.wantLen {
				t.Errorf("len = %d, want %d", len(m), tc.wantLen)
			}
			if tc.wantKey != "" && m[tc.wantKey] != tc.wantVal {
				t.Errorf("m[%q] = %q, want %q", tc.wantKey, m[tc.wantKey], tc.wantVal)
			}
		})
	}
}

func TestDetectPyPackage(t *testing.T) {
	dir := t.TempDir()
	// flat layout: <dir>/mypkg/__init__.py
	pkgDir := filepath.Join(dir, "mypkg")
	if err := os.MkdirAll(pkgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, pyInitFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := detectPyPackage(dir); got != "mypkg" {
		t.Errorf("detectPyPackage = %q, want mypkg", got)
	}
}
