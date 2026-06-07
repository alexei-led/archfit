package a

import "example.com/fixture-go/pkg/b/internal/impl"

func UseSecret() string { return impl.Secret() }
