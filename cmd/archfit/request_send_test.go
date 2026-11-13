package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestRequestSendsFromCLI(t *testing.T) {
	const token = "cli-secret-token"
	const receipt = "https://github.com/acme/shop/issues/431"
	for _, tc := range []struct {
		name          string
		first         []string
		upstream      bool
		pulls         string
		status        int
		noCredentials bool
		code          int
		next          string
		posts         int
	}{
		{"pull comment", []string{cmdCheck}, true, "[{\"number\":431}]", 201, false, 0, "", 1},
		{"can issue", []string{cmdCan, pkgTC, pkgUtil}, false, "[]", 201, false, 0, "", 1},
		{"check no upstream", []string{cmdCheck}, false, "[]", 0, false, 2, "open a pull request first", 0},
		{"check no pull", []string{cmdCheck}, true, "[]", 0, false, 2, "open a pull request first", 0},
		{"ambiguous", []string{cmdCheck}, true, "[{\"number\":431},{\"number\":432}]", 0, false, 2, "#431, #432", 0},
		{"API failure", []string{cmdCheck}, true, "[{\"number\":431}]", 403, false, 2, "HTTP 403", 1},
		{"no credentials", []string{cmdCheck}, true, "[]", 0, true, 2, "next: gh auth login", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GH_TOKEN", token)
			t.Setenv("GITHUB_TOKEN", "")
			if tc.noCredentials {
				t.Setenv("GH_TOKEN", "")
			}
			dir := fixtureRepo(t, fixtureFiles(), map[string]string{tcNetem: tcImportsUtil})
			gitFixture(t, dir, "remote", "add", "origin", "https://github.com/acme/shop.git")
			if tc.upstream {
				gitFixture(t, dir, "branch", "--set-upstream-to=origin/main")
			}
			first := runCLI(t, fixtureDate, append(tc.first, "-C", dir)...)
			before, err := os.ReadFile(filepath.Join(dir, lastFile)) //nolint:gosec // controlled fixture checkout
			if err != nil {
				t.Fatal(err)
			}
			calls, posts := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer "+token {
					t.Error("missing credential")
				}
				if r.Method == http.MethodGet {
					_, _ = fmt.Fprint(w, tc.pulls)
					return
				}
				posts++
				var payload struct{ Body string }
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if !strings.HasPrefix(payload.Body, "/archfit request "+whyGraph+"\n") {
					t.Errorf("body=%q", payload.Body)
				}
				if tc.name == "can issue" && payload.Body != "/archfit request "+whyGraph+"\n\n"+strings.Repeat("`", 3)+"text\n"+first.stdout+strings.Repeat("`", 3)+"\n" {
					t.Errorf("issue does not contain the can record verbatim: %q", payload.Body)
				}
				w.WriteHeader(tc.status)
				if tc.status == http.StatusCreated {
					_ = json.NewEncoder(w).Encode(map[string]any{"number": 431, "id": 7, "html_url": receipt})
				} else {
					_, _ = fmt.Fprint(w, token)
				}
			}))
			defer server.Close()
			target, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			transport := http.DefaultTransport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: requestTransport(func(r *http.Request) (*http.Response, error) {
				r = r.Clone(r.Context())
				r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
				return transport.RoundTrip(r)
			})}
			processRunner := toolrun.New()
			runner := &toolrun.RunnerMock{RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
				if cmd.Name == "gh" {
					return toolrun.Output{ExitCode: 1, Stdout: []byte(token), Stderr: []byte(token)}, nil
				}
				return processRunner.Run(ctx, cmd)
			}}
			var stdout, stderr bytes.Buffer
			code := runDeps([]string{cmdRequest, "-C", dir, "-m", whyGraph}, &appDeps{Runner: runner, Stdout: &stdout, Stderr: &stderr, requestClient: client, now: day(fixtureDate)})
			if code != tc.code || posts != tc.posts {
				t.Errorf("code=%d posts=%d, want %d/%d; stderr=%s", code, posts, tc.code, tc.posts, stderr.String())
			}
			if tc.code == 0 {
				if stdout.String() != receipt+"\n" || stderr.Len() != 0 {
					t.Errorf("stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			} else if stdout.Len() != 0 || !strings.Contains(stderr.String(), tc.next) {
				t.Errorf("stdout=%q stderr=%q; want next %q", stdout.String(), stderr.String(), tc.next)
			}
			if strings.Contains(stdout.String()+stderr.String(), token) {
				t.Error("credential in command output")
			}
			after, err := os.ReadFile(filepath.Join(dir, lastFile)) //nolint:gosec // controlled fixture checkout
			if err != nil || !bytes.Equal(before, after) {
				t.Errorf("sender changed the last result: %v", err)
			}
			if tc.noCredentials && calls != 0 {
				t.Errorf("%d API calls without credentials", calls)
			}
		})
	}
}

type requestTransport func(*http.Request) (*http.Response, error)

func (f requestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
