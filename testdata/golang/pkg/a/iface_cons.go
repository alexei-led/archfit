package a

import "example.com/test/pkg/b"

// NewIfaceHolder references only b.IfaceHolder — a struct with an
// interface-typed field (behavior carrier). Expected strength hint: model,
// not dto.
func NewIfaceHolder() b.IfaceHolder { return b.IfaceHolder{} }
