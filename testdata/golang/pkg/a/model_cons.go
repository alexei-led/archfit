package a

import "example.com/test/pkg/b"

// NewConfig creates a zero-value b.Config — struct type reference only, no field access.
// Expected strength hint: model (concrete TypeName, not interface, no function calls).
func NewConfig() b.Config { return b.Config{} }
