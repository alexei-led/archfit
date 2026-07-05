package a

import "example.com/test/pkg/b"

// NewMarker references b.Marker — a zero-field struct carries no data model
// and is NOT a DTO. Expected strength hint: model.
func NewMarker() b.Marker { return b.Marker{} }
