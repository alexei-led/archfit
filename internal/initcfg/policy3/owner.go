package policy3

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// githubHost is the host whose owners are GitHub logins.
const githubHost = "github.com"

// originOwner returns the owner of the repository's origin remote on GitHub:
// the first owner of the policy.
func originOwner(ctx context.Context, root string, runner toolrun.Runner) (string, error) {
	remote, ok, err := git.OriginURL(ctx, root, runner)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("the repository has no origin remote: add its GitHub remote as origin, then run archfit init again")
	}
	return ownerOf(remote)
}

// ownerOf returns the owner in a GitHub remote URL: scp-like
// (git@github.com:owner/repo.git), ssh://, https:// or git://. The error never
// prints the URL, which may hold a token.
func ownerOf(remote string) (string, error) {
	var host, repoPath string
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return "", errors.New("the origin remote is not a URL archfit init can read")
		}
		host, repoPath = u.Hostname(), u.Path
	} else {
		hostPart, p, ok := strings.Cut(remote, ":")
		if !ok {
			return "", errors.New("the origin remote is a local path, not a GitHub repository")
		}
		host, repoPath = hostPart[strings.LastIndex(hostPart, "@")+1:], p
	}
	if !strings.EqualFold(host, githubHost) {
		return "", fmt.Errorf("the origin remote is on %s, not %s: policy owners are GitHub logins", host, githubHost)
	}
	owner, _, _ := strings.Cut(strings.Trim(repoPath, "/"), "/")
	if owner == "" {
		return "", errors.New("the origin remote names no GitHub owner")
	}
	return owner, nil
}
