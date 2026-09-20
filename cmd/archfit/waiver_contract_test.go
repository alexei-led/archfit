package main

import (
	"bytes"
	"strings"
	"testing"
)

const waiverFixtureRules = `rules:
  - id: no_internal_access
    type: forbidden_dependency
    gate: fail
    from: pkg/a/**
    to: pkg/b/internal/**
`

func TestBaselineDisclosesSkippedTemporaryWaiver(t *testing.T) {
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+waiverFixtureRules+`waivers:
  - rule: no_internal_access
    from: pkg/a/**
    to: pkg/b/internal/**
    reason: staged migration
    approved_by: architecture-owner
    expires: "2099-01-01"
`)
	var stdout, stderr bytes.Buffer
	if code := RunWithStderr([]string{cmdBaseline, "-c", cfgPath, flagRefresh}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d; stderr=%s", code, &stderr)
	}
	if !strings.Contains(stdout.String(), "skipped 1 finding(s) covered by temporary waivers") {
		t.Fatalf("temporary finding not disclosed: %s", &stdout)
	}
}

func TestInvalidWaiverCannotSuppressGate(t *testing.T) {
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+waiverFixtureRules+"waivers:\n  - reason: make green\n")
	var stdout, stderr bytes.Buffer
	if code := RunWithStderr([]string{cmdCheck, "-c", cfgPath, flagJSON}, &stdout, &stderr); code != 3 {
		t.Fatalf("invalid waiver exit=%d, want 3; stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	if !strings.Contains(stderr.String(), "waivers") {
		t.Fatalf("missing actionable config error: %s", &stderr)
	}
}
