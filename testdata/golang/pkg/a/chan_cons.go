package a

import "example.com/test/pkg/b"

// NewChanHolder references b.ChanHolder — a struct with a chan-typed field
// (behavior carrier). Expected strength hint: model, not dto.
func NewChanHolder() b.ChanHolder { return b.ChanHolder{} }
