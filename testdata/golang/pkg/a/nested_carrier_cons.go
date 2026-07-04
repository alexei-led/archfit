package a

import "example.com/test/pkg/b"

// NewNestedHolder references only b.NestedHolder — behavior held transitively
// through a nested struct's func field. Expected strength hint: model, not dto.
func NewNestedHolder() b.NestedHolder { return b.NestedHolder{} }
