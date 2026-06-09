package scip

import _ "embed"

// scipProtoSrc is the SCIP protobuf schema (https://scip-code.org/), embedded so
// the reader helper can compile language bindings at runtime without a checked-in
// generated module or a system protoc.
//
//go:embed scip.proto
var scipProtoSrc []byte

// scipReaderSrc is the Python helper that reads a SCIP index and emits per-edge
// Balanced-Coupling integration strength as JSON. Run via uv (PEP 723 inline deps).
//
//go:embed scip_reader.py
var scipReaderSrc []byte
