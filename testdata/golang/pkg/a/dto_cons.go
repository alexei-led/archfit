package a

import "example.com/test/pkg/b"

// NewUser references only b.UserDTO — a pure-data struct (exported fields,
// empty method set). Expected strength hint: dto.
func NewUser() b.UserDTO { return b.UserDTO{ID: 1, Name: "u"} }
