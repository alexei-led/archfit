//go:build extractortest

package a

import "example.com/test/pkg/b/internal/impl"

// UseSecret accesses the internal implementation — a boundary violation.
func UseSecret() string { return impl.Secret() }
