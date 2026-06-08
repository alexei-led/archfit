package a

import "example.com/fixture-go/pkg/b"

// Dep demonstrates a dependency from module a to module b.
func Dep() string { return b.Hello() }
