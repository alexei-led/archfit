package evaluation

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/relationship"
)

func TestProjectSeamsPublishesSameOnlyWhenBothSidesAreKnown(t *testing.T) {
	tests := []struct {
		name               string
		raw                relationship.SeamDistance
		wantOwner, wantDep string
	}{
		{
			name:      "nothing declared is unknown, not different",
			wantOwner: "null", wantDep: "null",
		},
		{
			name:      "one undeclared side is unknown",
			raw:       relationship.SeamDistance{FromOwner: "team-a", FromDeployUnit: "api"},
			wantOwner: "null", wantDep: "null",
		},
		{
			name: "both declared and different",
			raw: relationship.SeamDistance{FromOwner: "team-a", ToOwner: "team-b",
				FromDeployUnit: "api", ToDeployUnit: "worker"},
			wantOwner: "false", wantDep: "false",
		},
		{
			name: "both declared and equal",
			raw: relationship.SeamDistance{FromOwner: "team-a", ToOwner: "team-a", SameOwner: true,
				FromDeployUnit: "api", ToDeployUnit: "api", SameDeployUnit: true},
			wantOwner: "true", wantDep: "true",
		},
	}
	show := func(b *bool) string {
		if b == nil {
			return "null"
		}
		if *b {
			return "true"
		}
		return "false"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectSeams([]relationship.Seam{{FromModule: "a", ToModule: "b", RawDistance: tt.raw}})[0].RawDistance
			if show(got.SameOwner) != tt.wantOwner || show(got.SameDeployUnit) != tt.wantDep {
				t.Errorf("same_owner = %s, same_deploy_unit = %s; want %s, %s",
					show(got.SameOwner), show(got.SameDeployUnit), tt.wantOwner, tt.wantDep)
			}
		})
	}
}
