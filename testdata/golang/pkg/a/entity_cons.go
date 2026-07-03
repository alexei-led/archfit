package a

import "example.com/test/pkg/b"

// NewEntity references only b.Entity — a struct WITH a method (non-empty
// method set). Expected strength hint: model, not dto.
func NewEntity() b.Entity { return b.Entity{ID: 1} }
