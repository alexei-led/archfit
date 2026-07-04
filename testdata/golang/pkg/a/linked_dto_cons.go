package a

import "example.com/test/pkg/b"

// NewLinkedNode references only b.LinkedNode — self-referential pure data.
// The *LinkedNode cycle must neither loop the purity check nor disqualify the
// DTO. Expected strength hint: dto.
func NewLinkedNode() b.LinkedNode { return b.LinkedNode{ID: 1} }
