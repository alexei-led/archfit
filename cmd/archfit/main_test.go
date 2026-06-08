package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_Version(t *testing.T) {
	var buf bytes.Buffer
	code := Run([]string{"--version"}, &buf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	out := buf.String()
	if !strings.Contains(out, "archfit version") {
		t.Errorf("expected version output, got: %q", out)
	}
}

func TestRun_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	code := Run([]string{"--help"}, &buf)
	// kong exits 0 on --help
	if code != 0 {
		t.Fatalf("expected exit 0 for --help, got %d (output: %q)", code, buf.String())
	}
}

func TestRun_Doctor(t *testing.T) {
	var buf bytes.Buffer
	code := Run([]string{"doctor"}, &buf)
	// Just verify it doesn't panic/crash (exit code may vary based on available tools)
	t.Logf("doctor exit %d output: %q", code, buf.String())
	_ = code
}
