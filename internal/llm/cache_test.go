package llm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	cachedAnswer = "answer"
	innerName    = "anthropic/m"
)

// countingProvider counts Complete calls and returns a fixed response.
type countingProvider struct {
	name  string
	calls int
	resp  Response
	err   error
}

func (p *countingProvider) Name() string { return p.name }
func (p *countingProvider) Complete(_ context.Context, _ Request) (Response, error) {
	p.calls++
	return p.resp, p.err
}

func TestCache_HitSkipsTransport(t *testing.T) {
	inner := &countingProvider{name: innerName, resp: Response{Text: cachedAnswer}}
	c := NewCache(inner, filepath.Join(t.TempDir(), "llm"))
	req := Request{System: "s", User: "u"}

	for i := 0; i < 3; i++ {
		resp, err := c.Complete(context.Background(), req)
		if err != nil || resp.Text != cachedAnswer {
			t.Fatalf("call %d: (%v, %v)", i, resp, err)
		}
	}
	if inner.calls != 1 {
		t.Errorf("inner calls = %d, want 1 (cache hits skip transport)", inner.calls)
	}
}

func TestCache_KeyVariesWithContent(t *testing.T) {
	inner := &countingProvider{name: innerName, resp: Response{Text: cachedAnswer}}
	c := NewCache(inner, filepath.Join(t.TempDir(), "llm"))

	reqs := []Request{
		{System: "s", User: "u"},
		{System: "s", User: "different"},
		{System: "different", User: "u"},
		{System: "s", User: "u", MaxTokens: 9},
	}
	for _, r := range reqs {
		if _, err := c.Complete(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	if inner.calls != len(reqs) {
		t.Errorf("inner calls = %d, want %d (every variation is a distinct key)", inner.calls, len(reqs))
	}

	// Different provider name → different key even with identical content.
	other := &countingProvider{name: "openai/m", resp: Response{Text: cachedAnswer}}
	c2 := NewCache(other, c.dir)
	if _, err := c2.Complete(context.Background(), reqs[0]); err != nil {
		t.Fatal(err)
	}
	if other.calls != 1 {
		t.Errorf("provider name must be part of the key")
	}
}

func TestCache_CorruptEntryRefetched(t *testing.T) {
	inner := &countingProvider{name: innerName, resp: Response{Text: cachedAnswer}}
	dir := filepath.Join(t.TempDir(), "llm")
	c := NewCache(inner, dir)
	req := Request{User: "u"}

	if _, err := c.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	// Corrupt the single cache entry.
	entries, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(entries) != 1 {
		t.Fatalf("entries = %v, want 1", entries)
	}
	if err := os.WriteFile(entries[0], []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}

	resp, err := c.Complete(context.Background(), req)
	if err != nil || resp.Text != cachedAnswer {
		t.Fatalf("after corruption: (%v, %v)", resp, err)
	}
	if inner.calls != 2 {
		t.Errorf("inner calls = %d, want 2 (corrupt entry refetched)", inner.calls)
	}
}

func TestCache_ErrorNotCached(t *testing.T) {
	inner := &countingProvider{name: innerName, err: errors.New("boom")}
	c := NewCache(inner, filepath.Join(t.TempDir(), "llm"))
	req := Request{User: "u"}

	if _, err := c.Complete(context.Background(), req); err == nil {
		t.Fatal("expected error")
	}
	inner.err = nil
	inner.resp = Response{Text: "recovered"}
	resp, err := c.Complete(context.Background(), req)
	if err != nil || resp.Text != "recovered" {
		t.Fatalf("recovery: (%v, %v) — errors must never be cached", resp, err)
	}
}
