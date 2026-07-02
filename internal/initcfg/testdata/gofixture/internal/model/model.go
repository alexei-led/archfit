// Package model is the innermost layer of the gofixture test project. It
// imports internal/engine — a back-edge (foundation layer depending on the
// outermost layer) that the generated forbidden-dependency rule must flag.
package model

import "example.com/gofixture/internal/engine"

// Describe reaches into the engine layer, the back-edge under test.
func Describe() string { return engine.Run() }
