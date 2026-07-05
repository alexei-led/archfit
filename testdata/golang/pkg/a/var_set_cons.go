package a

import "example.com/test/pkg/b"

// SetDefaultName WRITES another package's exported var. Reads and writes
// classify alike (assignment position is not inspected). Expected strength
// hint: model, same as the read in var_cons.go.
func SetDefaultName() { b.DefaultName = "a" }
