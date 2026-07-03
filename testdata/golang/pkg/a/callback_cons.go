package a

import "example.com/test/pkg/b"

// NewCallbackHolder references only b.CallbackHolder — a struct with a
// func-typed field (behavior carrier). Expected strength hint: model, not dto.
func NewCallbackHolder() b.CallbackHolder { return b.CallbackHolder{} }
