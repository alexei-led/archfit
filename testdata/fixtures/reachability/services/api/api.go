// Package api declares the fixture's cross-module contract.
package api

// Greeter is the public contract consumed by the app module.
type Greeter interface {
	Greet() string
}

// DefaultGreeting gives the API module one coverable statement without adding
// another cross-module reference.
func DefaultGreeting() string {
	return "hello"
}
