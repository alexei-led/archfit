package a

import "example.com/test/pkg/b"

// CallConcreteMethod calls b.MakeEntity (a function) and Entity.Rename (a method
// on a concrete receiver): functional, with model data evidence.
func CallConcreteMethod() { b.MakeEntity().Rename("x") }
