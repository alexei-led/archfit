package b

// Greeter is an interface for testing contract strength detection.
type Greeter interface {
	Greet() string
}

// Config is a concrete configuration type for testing model strength detection.
type Config struct{}
