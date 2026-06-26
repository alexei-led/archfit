package main

import "testing"

func skipSlowPipelineTest(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("slow pipeline integration test; rerun without -short")
	}
}
