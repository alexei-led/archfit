package a

import "example.com/test/pkg/b"

// ReadDTOField reads a b.UserDTO field through a plain selector — the common
// consumer shape (not a composite literal). Expected strength hint: dto.
func ReadDTOField(u b.UserDTO) int { return u.ID }
