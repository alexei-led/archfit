package a

import "example.com/test/pkg/b"

// UseMax references both b.Greeter (interface → contract, rank 1) and b.Hello()
// (function → functional, rank 3). Expected strength hint: functional (max rank wins).
func UseMax(g b.Greeter) string { return b.Hello() }
