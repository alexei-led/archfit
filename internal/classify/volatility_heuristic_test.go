package classify

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/coupling"
)

// TestDomainVolatilityFromPath covers the path-pattern heuristic table:
// generic-indicator paths → low, supporting-indicator paths → medium,
// everything else → unknown (never guess core/high).
func TestDomainVolatilityFromPath(t *testing.T) {
	tests := []struct {
		path string
		want coupling.Volatility
	}{
		// Generic subdomain indicators → low.
		{"vendor/some/lib.go", coupling.VolatilityLow},
		{"lib/crypto/hash.go", coupling.VolatilityLow},
		{"util/parser.go", coupling.VolatilityLow},
		{"utils/strings.go", coupling.VolatilityLow},
		{"pkg/common/types.go", coupling.VolatilityLow},
		{"pkg/util/format.go", coupling.VolatilityLow},
		{"shared/config/env.go", coupling.VolatilityLow},
		{"common/errors.go", coupling.VolatilityLow},

		// Supporting subdomain indicators → medium.
		{"infra/terraform/main.tf", coupling.VolatilityMedium},
		{"platform/k8s/deploy.yaml", coupling.VolatilityMedium},
		{"storage/s3/client.go", coupling.VolatilityMedium},
		{"db/postgres/repo.go", coupling.VolatilityMedium},
		{"database/migration.go", coupling.VolatilityMedium},
		{"cache/redis.go", coupling.VolatilityMedium},
		{"queue/rabbitmq.go", coupling.VolatilityMedium},
		{"messaging/kafka.go", coupling.VolatilityMedium},

		// Paths that do not match any pattern → unknown (conservative).
		// The heuristic never guesses core/high from a path.
		{"internal/classify/classify.go", coupling.VolatilityUnknown},
		{"cmd/archfit/main.go", coupling.VolatilityUnknown},
		{"core/domain/model.go", coupling.VolatilityUnknown},
		{"services/payment/processor.go", coupling.VolatilityUnknown},
		{"api/handler.go", coupling.VolatilityUnknown},
		{"", coupling.VolatilityUnknown},

		// Leading slash is normalized away.
		{"/vendor/pkg/x.go", coupling.VolatilityLow},
		{"/infra/network.go", coupling.VolatilityMedium},

		// Case-insensitive matching.
		{"Vendor/lib.go", coupling.VolatilityLow},
		{"INFRA/main.go", coupling.VolatilityMedium},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := domainVolatilityFromPath(tc.path)
			if got != tc.want {
				t.Errorf("domainVolatilityFromPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
