package a

import "example.com/test/pkg/b"

// AcceptGreeter takes a b.Greeter interface — only interface type reference, no function calls.
// Expected strength hint: contract (interface TypeName is the sole cross-package ref).
func AcceptGreeter(g b.Greeter) {}
