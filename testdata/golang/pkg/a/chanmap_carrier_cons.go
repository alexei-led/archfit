package a

import "example.com/test/pkg/b"

// NewChanMapHolder references only b.ChanMapHolder — a map[string]chan int
// field carries behavior. Expected strength hint: model, not dto.
func NewChanMapHolder() b.ChanMapHolder { return b.ChanMapHolder{} }
