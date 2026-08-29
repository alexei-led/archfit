package app

import "example.com/reachability/services/api"

// Greet calls the API through its declared contract.
func Greet(g api.Greeter) string {
	return g.Greet()
}
