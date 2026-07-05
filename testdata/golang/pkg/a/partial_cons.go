package a

import "example.com/test/pkg/b"

// NewPartial references only b.Partial — a struct with an unexported field.
// Expected strength hint: model, not dto.
func NewPartial() b.Partial { return b.Partial{Name: "p"} }
