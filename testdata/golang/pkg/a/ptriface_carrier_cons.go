package a

import "example.com/test/pkg/b"

// NewPtrIfaceHolder references only b.PtrIfaceHolder — a *Greeter
// (pointer-to-interface) field carries behavior. Expected strength hint:
// model, not dto.
func NewPtrIfaceHolder() b.PtrIfaceHolder { return b.PtrIfaceHolder{} }
