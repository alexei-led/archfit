package fixture

// Hello is an exported function used by the JSON-shape integration test.
func Hello() string {
	return "hello"
}

// unexported is intentionally not exported — it must not appear in Syntax() facts.
func unexported() {}
