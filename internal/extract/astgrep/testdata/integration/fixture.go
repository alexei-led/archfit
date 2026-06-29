package fixture

// Hello is an exported function used by the JSON-shape integration test.
func Hello() string {
	return "hello"
}

// unexported is intentionally not exported — it must not appear in Syntax() facts.
func unexported() {}

// Widget is an exported struct — exercises go-struct.
type Widget struct {
	Name string
}

// Renderer is an exported interface — exercises go-interface.
type Renderer interface {
	Render() string
}
