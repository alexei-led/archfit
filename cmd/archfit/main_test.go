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
	code := Run([]string{}, &buf)
	if code != 0 {
		t.Fatalf("expected exit 0 (help), got %d", code)
	}
}
