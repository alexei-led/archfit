package a

import "example.com/test/pkg/b"

// NewGenericBox references only b.GenericBox[int] — a generic pure-data-shaped
// struct. Expected strength hint: model (containsBehaviorCarrier rejects the
// type-parameter field via *types.Interface, since TypeParam.Underlying()
// resolves to the constraint interface).
func NewGenericBox() b.GenericBox[int] { return b.GenericBox[int]{Value: 1} }
