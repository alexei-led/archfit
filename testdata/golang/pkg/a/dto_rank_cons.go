package a

import "example.com/test/pkg/b"

// Describe uses b.Greeter (interface → contract, rank 1) AND b.UserDTO
// (pure-data struct → dto, rank 2) without calling any function or method.
// The strongest rank wins: dto.
func Describe(g b.Greeter) b.UserDTO {
	_ = g
	return b.UserDTO{}
}
