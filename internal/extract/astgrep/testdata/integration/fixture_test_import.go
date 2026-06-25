//go:build ignore

// fixture_test_import.go is excluded from Go compilation (build tag: ignore).
// ast-grep parses it to exercise the go-test-import-* rules.
// This simulates a production file (no _test.go suffix) that imports a test framework —
// the canonical pumba case: mock_*.go files with package X that import testify/mock.

package fixture

import "github.com/stretchr/testify/mock"

// MockWidget is a mock type that ships testify/mock into production code.
type MockWidget struct {
	mock.Mock
}
