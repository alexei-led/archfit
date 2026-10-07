package a

import "example.com/test/pkg/b"

// CallIfaceMethod invokes Greet on a b.Greeter: an interface-method call is
// contract, not functional, and contributes no data evidence beyond the type.
func CallIfaceMethod(g b.Greeter) string { return g.Greet() }
