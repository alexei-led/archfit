package factcache

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/toolrun"
)

const testAnalyzer = "ts"

var testCmd = toolrun.ToolCmd{Name: "depcruise", Args: []string{"--output-type", "json", "src"}}

// countingRunner returns a mock whose Run yields out/err on every call.
func countingRunner(out toolrun.Output, err error) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		RunFunc: func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
			return out, err
		},
	}
}

// TestRunner_Invalidation is the Task 2 invalidation table: a repeated run
// under the same key must hit (one subprocess), while a change in any key
// component — tool version, config slice, input tree content — must miss and
// re-execute.
func TestRunner_Invalidation(t *testing.T) {
	t.Parallel()

	cfgHashA, err := HashJSON(map[string]any{"include": []string{"src/**"}})
	if err != nil {
		t.Fatalf("HashJSON: %v", err)
	}
	cfgHashB, err := HashJSON(map[string]any{"include": []string{"lib/**"}})
	if err != nil {
		t.Fatalf("HashJSON: %v", err)
	}

	// Two input trees differing in one file's content, hashed the way a
	// Task 3 wiring would hash them.
	treeHash := func(content string) string {
		t.Helper()
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "main.ts"), []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		h, err := HashTree(root, []string{"main.ts"})
		if err != nil {
			t.Fatalf("HashTree: %v", err)
		}
		return h
	}
	treeA := treeHash("export const a = 1")
	treeB := treeHash("export const a = 2")

	baseKey := Key(testAnalyzer, "v1", cfgHashA, treeA)
	cases := []struct {
		name      string
		secondKey string
		wantCalls int // inner Run calls after two decorated runs
	}{
		{"same key hits", baseKey, 1},
		{"tool version change invalidates", Key(testAnalyzer, "v2", cfgHashA, treeA), 2},
		{"config slice change invalidates", Key(testAnalyzer, "v1", cfgHashB, treeA), 2},
		{"file content change invalidates", Key(testAnalyzer, "v1", cfgHashA, treeB), 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := NewStore(t.TempDir())
			inner := countingRunner(toolrun.Output{Stdout: []byte(`{"modules":[]}`)}, nil)

			first := &Runner{Inner: inner, Store: store, Analyzer: testAnalyzer, Key: baseKey}
			if _, err := first.Run(context.Background(), testCmd); err != nil {
				t.Fatalf("first run: %v", err)
			}
			second := &Runner{Inner: inner, Store: store, Analyzer: testAnalyzer, Key: tc.secondKey}
			if _, err := second.Run(context.Background(), testCmd); err != nil {
				t.Fatalf("second run: %v", err)
			}
			if got := len(inner.RunCalls()); got != tc.wantCalls {
				t.Errorf("inner Run calls = %d, want %d", got, tc.wantCalls)
			}
		})
	}
}

// TestRunner_HitReproducesOutput pins the byte-identical contract at the
// seam: a warm run must return exactly the Output the cold run produced —
// stdout, stderr, and exit code.
func TestRunner_HitReproducesOutput(t *testing.T) {
	t.Parallel()
	want := toolrun.Output{
		Stdout:   []byte(`{"modules":[{"source":"src/a.ts"}]}`),
		Stderr:   []byte("warning: 2 unresolved specifiers\n"),
		ExitCode: 1, // dependency-cruiser reporting violations: usable output
	}
	inner := countingRunner(want, nil)
	r := &Runner{Inner: inner, Store: NewStore(t.TempDir()), Analyzer: testAnalyzer, Key: Key(testAnalyzer, "v1", "cfg", "tree")}

	cold, err := r.Run(context.Background(), testCmd)
	if err != nil {
		t.Fatalf("cold run: %v", err)
	}
	warm, err := r.Run(context.Background(), testCmd)
	if err != nil {
		t.Fatalf("warm run: %v", err)
	}
	if len(inner.RunCalls()) != 1 {
		t.Errorf("inner Run calls = %d, want 1 (warm must not re-execute)", len(inner.RunCalls()))
	}
	if !reflect.DeepEqual(cold, want) {
		t.Errorf("cold output = %+v, want %+v", cold, want)
	}
	if !reflect.DeepEqual(warm, cold) {
		t.Errorf("warm output = %+v, want byte-identical to cold %+v", warm, cold)
	}
}

func TestRunner_ExecErrorNeverCached(t *testing.T) {
	t.Parallel()
	inner := countingRunner(toolrun.Output{}, errors.New("binary not found"))
	r := &Runner{Inner: inner, Store: NewStore(t.TempDir()), Analyzer: testAnalyzer, Key: Key(testAnalyzer, "v1", "cfg", "tree")}

	for range 2 {
		if _, err := r.Run(context.Background(), testCmd); err == nil {
			t.Fatal("expected exec error to propagate")
		}
	}
	if got := len(inner.RunCalls()); got != 2 {
		t.Errorf("inner Run calls = %d, want 2 (failures must not be cached)", got)
	}
}

func TestRunner_CorruptedBlobIsMissAndHeals(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	inner := countingRunner(toolrun.Output{Stdout: []byte("fact")}, nil)
	r := &Runner{Inner: inner, Store: store, Analyzer: testAnalyzer, Key: Key(testAnalyzer, "v1", "cfg", "tree")}

	// Plant a corrupted blob at the exact entry path the decorator will read.
	store.Put(testAnalyzer, r.entryKey(testCmd), []byte("{not json"))

	out, err := r.Run(context.Background(), testCmd)
	if err != nil {
		t.Fatalf("run over corrupted entry must not fail: %v", err)
	}
	if string(out.Stdout) != "fact" {
		t.Errorf("stdout = %q, want fresh subprocess output", out.Stdout)
	}
	if len(inner.RunCalls()) != 1 {
		t.Errorf("inner Run calls = %d, want 1 (corrupt blob is a miss)", len(inner.RunCalls()))
	}

	// The corrupt entry was replaced: the next run is a hit.
	if _, err := r.Run(context.Background(), testCmd); err != nil {
		t.Fatalf("healed run: %v", err)
	}
	if len(inner.RunCalls()) != 1 {
		t.Errorf("inner Run calls = %d, want 1 (entry should have healed)", len(inner.RunCalls()))
	}
}

// TestRunner_NilStoreBypassesReadAndWrite pins the --no-cache contract: a nil
// store disables reads (a prewarmed entry is ignored, because there is no
// store to read) AND writes (the run leaves nothing behind).
func TestRunner_NilStoreBypassesReadAndWrite(t *testing.T) {
	t.Parallel()
	inner := countingRunner(toolrun.Output{Stdout: []byte("fact")}, nil)
	r := &Runner{Inner: inner, Store: nil, Analyzer: testAnalyzer, Key: Key(testAnalyzer, "v1", "cfg", "tree")}

	for i := range 2 {
		if _, err := r.Run(context.Background(), testCmd); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if got := len(inner.RunCalls()); got != 2 {
		t.Errorf("inner Run calls = %d, want 2 (nil store must always execute)", got)
	}
}

func TestRunner_CacheableVetoSkipsWrite(t *testing.T) {
	t.Parallel()
	inner := countingRunner(toolrun.Output{Stdout: []byte("partial")}, nil)
	r := &Runner{
		Inner:     inner,
		Store:     NewStore(t.TempDir()),
		Analyzer:  testAnalyzer,
		Key:       Key(testAnalyzer, "v1", "cfg", "tree"),
		Cacheable: func(toolrun.Output) bool { return false },
	}

	for i := range 2 {
		if _, err := r.Run(context.Background(), testCmd); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if got := len(inner.RunCalls()); got != 2 {
		t.Errorf("inner Run calls = %d, want 2 (vetoed results must not be stored)", got)
	}
}

func TestRunner_DetectDelegatesUncached(t *testing.T) {
	t.Parallel()
	inner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
		},
	}
	r := &Runner{Inner: inner, Store: NewStore(t.TempDir()), Analyzer: testAnalyzer, Key: "k"}

	for range 2 {
		if _, ok := r.Detect(context.Background(), "node"); !ok {
			t.Fatal("expected detect hit")
		}
	}
	if got := len(inner.DetectCalls()); got != 2 {
		t.Errorf("inner Detect calls = %d, want 2 (probes are never cached)", got)
	}
}

// TestRunner_StreamCachesAndReplays: a cold Stream tees the live stdout into
// the cache; a warm Stream feeds consume the identical bytes without running
// the subprocess, preserving stderr and exit code.
func TestRunner_StreamCachesAndReplays(t *testing.T) {
	t.Parallel()
	const streamed = `{"kind":"syntax","matches":[1,2,3]}`
	inner := &toolrun.RunnerMock{
		StreamFunc: func(_ context.Context, _ toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
			if err := consume(strings.NewReader(streamed)); err != nil {
				return toolrun.Output{}, err
			}
			return toolrun.Output{Stderr: []byte("scanned 3 files\n")}, nil
		},
	}
	r := &Runner{Inner: inner, Store: NewStore(t.TempDir()), Analyzer: "astgrep", Key: Key("astgrep", "v1", "cfg", "tree")}

	read := func() (string, toolrun.Output) {
		t.Helper()
		var got strings.Builder
		out, err := r.Stream(context.Background(), testCmd, func(rd io.Reader) error {
			_, cerr := io.Copy(&got, rd)
			return cerr
		})
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		return got.String(), out
	}

	coldBytes, coldOut := read()
	warmBytes, warmOut := read()

	if len(inner.StreamCalls()) != 1 {
		t.Errorf("inner Stream calls = %d, want 1 (warm must replay from cache)", len(inner.StreamCalls()))
	}
	if coldBytes != streamed || warmBytes != coldBytes {
		t.Errorf("streamed bytes: cold %q, warm %q, want both %q", coldBytes, warmBytes, streamed)
	}
	if !reflect.DeepEqual(warmOut, coldOut) {
		t.Errorf("warm output = %+v, want %+v", warmOut, coldOut)
	}
	if warmOut.Stdout != nil {
		t.Error("Stream Output.Stdout must stay nil (toolrun contract)")
	}
}

// TestRunner_EntryKeyDiscriminatesCommands: two different commands under the
// same Key must not alias each other's entries; WorkDir is deliberately
// excluded so a --base temp worktree can share entries across runs.
func TestRunner_EntryKeyDiscriminatesCommands(t *testing.T) {
	t.Parallel()
	r := &Runner{Analyzer: testAnalyzer, Key: "k"}

	const name, arg = "cargo", "metadata"
	base := r.entryKey(toolrun.ToolCmd{Name: name, Args: []string{arg}})
	otherArgs := r.entryKey(toolrun.ToolCmd{Name: name, Args: []string{"modules"}})
	otherName := r.entryKey(toolrun.ToolCmd{Name: "jscpd", Args: []string{arg}})
	otherWorkDir := r.entryKey(toolrun.ToolCmd{Name: name, Args: []string{arg}, WorkDir: "/tmp/worktree-xyz"})

	if base == otherArgs || base == otherName {
		t.Error("different commands under one key must map to different entries")
	}
	if base != otherWorkDir {
		t.Error("WorkDir must not affect the entry address (worktree reuse)")
	}
}
