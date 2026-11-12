package policy3

import (
	"strings"
	"testing"
)

func TestOwnerOf(t *testing.T) {
	const acme = "acme"
	for _, tt := range []struct {
		remote, owner, err string
	}{
		{remote: "git@github.com:acme/shop.git", owner: acme},
		{remote: "git@github.com:acme/shop", owner: acme},
		{remote: "https://github.com/acme/shop.git", owner: acme},
		{remote: "https://x-access-token:secret@github.com/acme/shop", owner: acme}, //nolint:gosec // a fake token
		{remote: "ssh://git@github.com:22/acme/shop.git", owner: acme},
		{remote: "git://GitHub.com/acme/shop.git", owner: acme},
		{remote: "https://oauth2:secret@gitlab.com/acme/shop.git", err: "on gitlab.com, not github.com"}, //nolint:gosec // a fake token the error must not print
		{remote: "git@bitbucket.org:acme/shop.git", err: "on bitbucket.org, not github.com"},
		{remote: "/srv/git/shop.git", err: "local path"},
		{remote: "https://github.com/", err: "names no GitHub owner"},
	} {
		t.Run(tt.remote, func(t *testing.T) {
			owner, err := ownerOf(tt.remote)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("ownerOf = %q, %v; want an error that says %q", owner, err, tt.err)
				}
				if strings.Contains(err.Error(), "secret") {
					t.Errorf("the error prints the URL: %v", err)
				}
				return
			}
			if err != nil || owner != tt.owner {
				t.Errorf("ownerOf = %q, %v; want %q", owner, err, tt.owner)
			}
		})
	}
}
