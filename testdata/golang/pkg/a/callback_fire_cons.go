package a

import "example.com/test/pkg/b"

// FireCallback invokes the func-typed field of b.CallbackHolder — calling
// behavior stored in a field outranks the holder type's model hint.
// Expected strength hint: functional.
func FireCallback(h b.CallbackHolder) { h.OnDone() }
