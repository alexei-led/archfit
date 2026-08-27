package evaluation

import "testing"

func TestValidationCommandQuotesConfigAndRoot(t *testing.T) {
	t.Parallel()
	got := ValidationCommand("/tmp/policy bundle/it's.yaml", "/tmp/repo root")
	want := "archfit check -c '/tmp/policy bundle/it'\"'\"'s.yaml' --root '/tmp/repo root'"
	if got != want {
		t.Fatalf("ValidationCommand = %q, want %q", got, want)
	}
}
