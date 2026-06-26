//go:build ignore

// fixture_test_fn.go is excluded from Go compilation (build tag: ignore).
// ast-grep parses it to exercise the go-test-fn rule (test_fn kind).
// Simulates a *_test.go file containing standard Go test functions.

package fixture

import "testing"

// TestHello exercises go-test-fn: func declaration with name matching ^Test.
func TestHello(t *testing.T) {
	_ = t
}
