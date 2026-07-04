package a

import "example.com/test/pkg/b"

// NewSliceCallbackHolder references only b.SliceCallbackHolder — a []func()
// field carries behavior through a composite element type. Expected strength
// hint: model, not dto.
func NewSliceCallbackHolder() b.SliceCallbackHolder { return b.SliceCallbackHolder{} }
