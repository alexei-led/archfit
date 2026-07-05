package a

import "example.com/test/pkg/b"

// Name references only b.DefaultName — a pure-data var read, no calls,
// no type references. Expected strength hint: model (book Ch7 data sharing).
func Name() string { return b.DefaultName }
