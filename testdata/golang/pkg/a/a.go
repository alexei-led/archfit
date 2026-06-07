package a

import "example.com/test/pkg/b"

// UseB calls the public Hello function from pkg/b.
func UseB() string { return b.Hello() }
