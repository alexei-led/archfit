package a

import "example.com/test/pkg/b"

// CallDefaultHandler invokes b.DefaultHandler — a func-typed var. Stored
// behavior, not data. Expected strength hint: functional, not model.
func CallDefaultHandler() string { return b.DefaultHandler() }
