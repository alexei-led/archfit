package requestsend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	testReason       = "why"
	testRepo         = "acme/shop"
	unknownOutcome   = "outcome unknown"
	secondCredential = "second"
	thirdCredential  = "third"
	forkHead         = "worker:feature"
	originCommand    = "git config --get remote.origin.url"
	upstreamCommand  = "git rev-parse --abbrev-ref --symbolic-full-name @{upstream}"
)

const testToken = "secret-test-token"
const issueURL = "https://github.com/acme/shop/issues/431"
const commentURL = "https://github.com/acme/shop/pull/431#issuecomment-9"
const onePull = "[{\"number\":431}]"

func TestSendPaths(t *testing.T) {
	for _, tc := range []struct {
		name              string
		can               bool
		pulls             string
		wantPath, wantURL string
		wantError         string
	}{
		{"check comment", false, onePull, "/repos/acme/shop/issues/431/comments", commentURL, ""},
		{"can comment", true, onePull, "/repos/acme/shop/issues/431/comments", commentURL, ""},
		{"can issue", true, "[]", "/repos/acme/shop/issues", issueURL, ""},
		{"check without pull", false, "[]", "", "", "next: open a pull request first"},
		{"ambiguous", false, "[{\"number\":431},{\"number\":432}]", "", "", "#431, #432"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GH_TOKEN", testToken)
			var posts []string
			r := Input{Reason: testReason}
			if tc.can {
				r.Can = canAnswer()
			}
			s := fakeSender(t, nil, func(w http.ResponseWriter, req *http.Request) {
				if req.Header.Get("Authorization") != "Bearer "+testToken {
					t.Error("missing credential")
				}
				if req.Method == http.MethodGet {
					if req.URL.Path != "/repos/acme/shop/pulls" || req.URL.Query().Get("state") != "open" || req.URL.Query().Get("head") != "acme:feature" {
						t.Errorf("lookup: %s", req.URL)
					}
					_, _ = fmt.Fprint(w, tc.pulls)
					return
				}
				posts = append(posts, req.URL.Path)
				if req.URL.Path != tc.wantPath {
					t.Errorf("POST path = %s", req.URL.Path)
				}
				var payload struct{ Title, Body string }
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if tc.wantPath == "/repos/acme/shop/issues" {
					if payload.Title != "archfit request: a -> b" || payload.Body != issueBody(r) {
						t.Errorf("issue: %+v", payload)
					}
				} else if payload.Body != Line(r) || payload.Title != "" {
					t.Errorf("comment: %+v", payload)
				}
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{"number": 431, "id": 9, "html_url": tc.wantURL})
			})
			got, err := s.Send(t.Context(), r)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || got != (Outcome{}) || len(posts) != 0 {
					t.Fatalf("delivery=%+v err=%v posts=%v", got, err, posts)
				}
			} else if err != nil || got.URL != tc.wantURL || got.Number != 431 || len(posts) != 1 {
				t.Fatalf("delivery=%+v err=%v posts=%v", got, err, posts)
			}
		})
	}
}

func TestAPIFailuresNeverRetryOrLeak(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		lookupStatus, sendStatus int
		response                 string
		want                     string
		posts                    int
	}{
		{"lookup unauthorized", 401, 0, testToken, "HTTP 401", 0},
		{"lookup rate limit", 429, 0, testToken, "HTTP 429", 0},
		{"lookup invalid", 200, 0, testToken, "lookup unreadable", 0},
		{"send forbidden", 200, 403, testToken, "HTTP 403", 1},
		{"send validation", 200, 422, testToken, "HTTP 422", 1},
		{"send server", 200, 500, testToken, unknownOutcome, 1},
		{"send invalid receipt", 200, 201, testToken, unknownOutcome, 1},
		{"send wrong url", 200, 201, "{\"number\":1,\"html_url\":\"https://evil.invalid/secret-test-token\"}", unknownOutcome, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GH_TOKEN", testToken)
			posts := 0
			s := fakeSender(t, nil, func(w http.ResponseWriter, req *http.Request) {
				if req.Method == http.MethodGet {
					w.WriteHeader(tc.lookupStatus)
					if tc.lookupStatus == 200 && tc.name != "lookup invalid" {
						_, _ = fmt.Fprint(w, onePull)
					} else {
						_, _ = fmt.Fprint(w, tc.response)
					}
					return
				}
				posts++
				w.WriteHeader(tc.sendStatus)
				_, _ = fmt.Fprint(w, tc.response)
			})
			got, err := s.Send(t.Context(), Input{Reason: testReason})
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), testToken) || got != (Outcome{}) || posts != tc.posts {
				t.Fatalf("delivery=%+v err=%v posts=%d", got, err, posts)
			}
		})
	}
}

func TestSendOutcomeUnknownAfterConnectionLost(t *testing.T) {
	t.Setenv("GH_TOKEN", testToken)
	var posts atomic.Int32
	s := fakeSender(t, nil, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			_, _ = fmt.Fprint(w, onePull)
			return
		}
		posts.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	})
	got, err := s.Send(t.Context(), Input{Reason: testReason})
	if err == nil || !strings.Contains(err.Error(), unknownOutcome) || got != (Outcome{}) || posts.Load() != 1 {
		t.Fatalf("delivery=%+v err=%v posts=%d", got, err, posts.Load())
	}
}

func TestNoCredentialsSendsNothing(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	calls := 0
	s := fakeSender(t, map[string]toolrun.Output{"gh auth token": {ExitCode: 1, Stdout: []byte(testToken), Stderr: []byte(testToken)}}, func(http.ResponseWriter, *http.Request) { calls++ })
	got, err := s.Send(t.Context(), Input{Reason: testReason})
	if err == nil || !strings.Contains(err.Error(), "next: gh auth login") || strings.Contains(err.Error(), testToken) || calls != 0 || got != (Outcome{}) {
		t.Fatalf("delivery=%+v err=%v calls=%d", got, err, calls)
	}
}

func TestCredentialsPrecedence(t *testing.T) {
	for _, tc := range []struct {
		gh, github, cli, want string
		ghCalls               int
	}{
		{"first", secondCredential, thirdCredential, "first", 0},
		{"", secondCredential, thirdCredential, secondCredential, 0},
		{"", "", " third\n", thirdCredential, 1},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Setenv("GH_TOKEN", tc.gh)
			t.Setenv("GITHUB_TOKEN", tc.github)
			calls := 0
			s := New(".", &toolrun.RunnerMock{RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
				calls++
				if cmd.Name != "gh" || !reflect.DeepEqual(cmd.Args, []string{"auth", "token"}) {
					t.Errorf("command: %+v", cmd)
				}
				return toolrun.Output{Stdout: []byte(tc.cli)}, nil
			}})
			got, err := s.credentials(t.Context())
			if err != nil || got != tc.want || calls != tc.ghCalls {
				t.Fatalf("credential mismatch, err=%v, calls=%d", err, calls)
			}
		})
	}
}

func TestRepositoryRouting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		override   map[string]toolrun.Output
		repo, head string
	}{
		{"origin", nil, testRepo, "acme:feature"},
		{"fork push", map[string]toolrun.Output{
			"git for-each-ref --format=%(push:remotename) refs/heads/feature": {Stdout: []byte("fork\n")},
			"git config --get remote.fork.url":                                {Stdout: []byte("git@github.com:worker/shop.git\n")},
		}, testRepo, forkHead},
		{"origin fork upstream", map[string]toolrun.Output{
			originCommand:                          {Stdout: []byte("https://github.com/worker/shop.git")},
			"git config --get remote.upstream.url": {Stdout: []byte("https://github.com/acme/shop.git")},
		}, testRepo, forkHead},
		{"no upstream", map[string]toolrun.Output{upstreamCommand: {ExitCode: 128}}, testRepo, ""},
		{"detached", map[string]toolrun.Output{"git symbolic-ref --quiet --short HEAD": {ExitCode: 1}}, testRepo, ""},
		{"non GitHub", map[string]toolrun.Output{originCommand: {Stdout: []byte("git@gitlab.com:acme/shop.git")}}, "", ""},
		{"no origin", map[string]toolrun.Output{originCommand: {ExitCode: 1}}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(".", gitRunner(t, tc.override))
			repo, head, err := s.repository(t.Context())
			if err != nil || repo != tc.repo || head != tc.head {
				t.Fatalf("repo=%q head=%q err=%v", repo, head, err)
			}
		})
	}
}

func TestNoUpstreamDoesNotLookupOrSendCheck(t *testing.T) {
	s := New(".", gitRunner(t, map[string]toolrun.Output{upstreamCommand: {ExitCode: 128}}))
	got, err := s.Send(t.Context(), Input{Reason: testReason})
	if err == nil || !strings.Contains(err.Error(), "open a pull request first") || got != (Outcome{}) {
		t.Fatalf("delivery=%+v err=%v", got, err)
	}
}

func TestOfflineDoesNotReadCredentials(t *testing.T) {
	s := New(".", gitRunner(t, map[string]toolrun.Output{originCommand: {ExitCode: 1}}))
	got, err := s.Send(t.Context(), Input{Reason: testReason})
	if err != nil || got != (Outcome{}) {
		t.Fatalf("delivery=%+v err=%v", got, err)
	}
}

func TestGitHubRemote(t *testing.T) {
	for _, tc := range []struct{ remote, want string }{
		{"https://github.com/acme/shop.git", testRepo},
		{"ssh://git@github.com/acme/shop.git", testRepo},
		{"git@github.com:acme/shop.git", testRepo},
		{"https://github.com.evil.invalid/acme/shop", ""},
		{"https://github.com/acme/../shop", ""},
		{"file:///acme/shop", ""},
		{"https://github.com/acme/shop?token=secret", ""},
	} {
		if got := githubRepository(tc.remote); got != tc.want {
			t.Errorf("repository(%q) = %q, want %q", tc.remote, got, tc.want)
		}
	}
}

func TestTransportFailureAndTimeoutDoNotLeak(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		s := New(".", gitRunner(t, nil))
		s.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if _, ok := r.Context().Deadline(); !ok {
				t.Error("no HTTP timeout")
			}
			if method == http.MethodPost && r.GetBody != nil {
				t.Error("POST can be replayed")
			}
			return nil, errors.New(testToken)
		})}
		var out any
		_, err := s.call(t.Context(), testToken, method, "/test", map[string]string{"body": "request"}, &out)
		if err == nil || strings.Contains(err.Error(), testToken) {
			t.Fatalf("error=%v", err)
		}
		if method == http.MethodPost && !strings.Contains(err.Error(), unknownOutcome) {
			t.Fatalf("error=%v", err)
		}
	}
}

func TestRedirectNeverForwardsCredentials(t *testing.T) {
	t.Setenv("GH_TOKEN", testToken)
	calls := 0
	s := fakeSender(t, nil, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Location", "https://evil.invalid/"+testToken)
		w.WriteHeader(http.StatusTemporaryRedirect)
	})
	_, err := s.Send(t.Context(), Input{Reason: testReason})
	if err == nil || strings.Contains(err.Error(), testToken) || calls != 1 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}

func canAnswer() *result.CanAnswer {
	return &result.CanAnswer{From: "a", To: "b", Rule: "graph", What: "not an allowed dependency", Policy: result.Policy{Protected: true}}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fakeSender(t *testing.T, override map[string]toolrun.Output, handler http.HandlerFunc) *Sender {
	t.Helper()
	var logs bytes.Buffer
	server := httptest.NewUnstartedServer(handler)
	server.Config.ErrorLog = log.New(&logs, "", 0)
	server.Start()
	t.Cleanup(func() {
		server.Close()
		if strings.Contains(logs.String(), testToken) {
			t.Error("token leaked to server logs")
		}
	})
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	s := New(".", gitRunner(t, override))
	s.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
		return transport.RoundTrip(r)
	})}
	return s
}

func gitRunner(t *testing.T, override map[string]toolrun.Output) *toolrun.RunnerMock {
	t.Helper()
	commands := map[string]toolrun.Output{
		originCommand:                           {Stdout: []byte("https://github.com/acme/shop.git\n")},
		"git config --get remote.upstream.url":  {ExitCode: 1},
		"git symbolic-ref --quiet --short HEAD": {Stdout: []byte("feature\n")},
		upstreamCommand:                         {Stdout: []byte("origin/feature\n")},
		"git for-each-ref --format=%(push:remotename) refs/heads/feature": {},
	}
	for key, value := range override {
		commands[key] = value
	}
	return &toolrun.RunnerMock{RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		key := cmd.Name + " " + strings.Join(cmd.Args, " ")
		if got, ok := commands[key]; ok {
			return got, nil
		}
		t.Errorf("unexpected command %s", key)
		return toolrun.Output{}, errors.New("unexpected command")
	}}
}
