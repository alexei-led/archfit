package b

// Hello returns a greeting string.
func Hello() string { return "hello" }

// MakeEntity returns a concrete Entity so a caller can invoke a concrete-receiver method without naming the type.
func MakeEntity() *Entity { return &Entity{} }
