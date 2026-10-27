package evaluation

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/relationship"
)

func TestProjectSeamsPublishesSameOnlyWhenBothSidesAreKnown(t *testing.T) {
	const ownerA = "team-a"
	yes, no := true, false
	tests := []struct {
		name               string
		raw                relationship.SeamDistance
		wantOwner, wantDep *bool
	}{
		{name: "nothing declared is unknown, not different"},
		{
			name: "one undeclared side is unknown",
			raw:  relationship.SeamDistance{FromOwner: ownerA, FromDeployUnit: modAPI},
		},
		{
			name: "both declared and different",
			raw: relationship.SeamDistance{FromOwner: ownerA, ToOwner: "team-b",
				FromDeployUnit: modAPI, ToDeployUnit: "worker"},
			wantOwner: &no, wantDep: &no,
		},
		{
			name: "both declared and equal",
			raw: relationship.SeamDistance{FromOwner: ownerA, ToOwner: ownerA, SameOwner: true,
				FromDeployUnit: modAPI, ToDeployUnit: modAPI, SameDeployUnit: true},
			wantOwner: &yes, wantDep: &yes,
		},
	}
	same := func(a, b *bool) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectSeams([]relationship.Seam{{FromModule: "a", ToModule: "b", RawDistance: tt.raw}})[0].RawDistance
			if !same(got.SameOwner, tt.wantOwner) || !same(got.SameDeployUnit, tt.wantDep) {
				t.Errorf("same_owner = %v, same_deploy_unit = %v; want %v, %v",
					got.SameOwner, got.SameDeployUnit, tt.wantOwner, tt.wantDep)
			}
		})
	}
}
