package requestsend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestEffectivePushURLAndTrackingBranch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override map[string]toolrun.Output
		want     string
	}{
		{"origin pushurl is a fork", map[string]toolrun.Output{
			"git remote get-url --push origin": {Stdout: []byte("https://github.com/worker/shop.git\n")},
		}, forkHead},
		{"renamed local branch", map[string]toolrun.Output{
			upstreamCommand: {Stdout: []byte("origin/remote-work\n")},
		}, "acme:remote-work"},
		{"tracking branch with slash", map[string]toolrun.Output{
			upstreamCommand: {Stdout: []byte("origin/feature/part\n")},
		}, "acme:feature/part"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, head, err := New(".", gitRunner(t, tc.override)).repository(t.Context())
			if err != nil || head != tc.want {
				t.Fatalf("head=%q err=%v, want %q", head, err, tc.want)
			}
		})
	}
}

func TestCanonicalReceiptCasing(t *testing.T) {
	for _, can := range []bool{false, true} {
		t.Run(fmt.Sprintf("can=%v", can), func(t *testing.T) {
			t.Setenv("GH_TOKEN", testToken)
			posts := 0
			s := fakeSender(t, map[string]toolrun.Output{originCommand: {Stdout: []byte("https://github.com/Acme/Shop.git")}}, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if can {
						_, _ = fmt.Fprint(w, "[]")
					} else {
						_, _ = fmt.Fprint(w, onePull)
					}
					return
				}
				posts++
				receipt := commentURL
				if can {
					receipt = issueURL
				}
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{"number": 431, "id": 9, "html_url": receipt})
			})
			in := Input{Reason: testReason}
			if can {
				in.Can = canAnswer()
			}
			got, err := s.Send(t.Context(), in)
			want := commentURL
			if can {
				want = issueURL
			}
			if err != nil || got.URL != want || posts != 1 {
				t.Fatalf("outcome=%+v err=%v posts=%d", got, err, posts)
			}
		})
	}
}
