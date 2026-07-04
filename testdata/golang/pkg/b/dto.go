package b

// UserDTO is a pure-data transfer struct: exported, only exported data fields,
// empty method set. Referenced across a declared public boundary it is the
// book's canonical integration Contract (strength hint "dto").
type UserDTO struct {
	ID   int
	Name string
}

// Entity is a domain type with behavior — a non-empty method set disqualifies
// it from the DTO upgrade; it stays model.
type Entity struct {
	ID int
}

// Rename gives Entity a method so its method set is non-empty.
func (e *Entity) Rename(name string) { _ = name }

// Partial has an unexported field — not a pure data contract; stays model.
type Partial struct {
	Name   string
	secret int //nolint:unused // presence of an unexported field is the test signal
}

// CallbackHolder carries behavior through a func-typed field — not pure data;
// stays model.
type CallbackHolder struct {
	OnDone func()
}

// ChanHolder carries behavior through a chan-typed field — not pure data;
// stays model.
type ChanHolder struct {
	Events chan int
}

// IfaceHolder carries behavior through an interface-typed field — not pure
// data; stays model.
type IfaceHolder struct {
	G Greeter
}

// SliceCallbackHolder carries behavior through a []func() field — a composite
// element type is still a behavior carrier; not pure data, stays model.
type SliceCallbackHolder struct {
	Handlers []func()
}

// ChanMapHolder carries behavior through a map value chan type — not pure
// data; stays model.
type ChanMapHolder struct {
	Streams map[string]chan int
}

// PtrIfaceHolder carries behavior through a pointer-to-interface field — not
// pure data; stays model.
type PtrIfaceHolder struct {
	G *Greeter
}

// NestedHolder carries behavior transitively through a nested struct whose
// field is func-typed — not pure data; stays model.
type NestedHolder struct {
	Inner CallbackHolder
}

// LinkedNode is self-referential pure data: the cycle through *LinkedNode is
// not a behavior carrier, so it stays a DTO.
type LinkedNode struct {
	ID   int
	Next *LinkedNode
}

// Marker is a zero-field struct: carries no data model, NOT a DTO.
type Marker struct{}
