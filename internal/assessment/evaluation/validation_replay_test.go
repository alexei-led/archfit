package evaluation

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestValidationReplayQuotesEveryArgument(t *testing.T) {
	got := validationCommand("policy's rules.yaml", "repo path", "--base", "refs/heads/review's", "--lang", "go", "--require-tools")
	cmd := exec.Command("sh", "-s")
	cmd.Stdin = strings.NewReader("archfit() { printf '%s\\n' \"$@\"; }; " + got)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"check", "-c", "policy's rules.yaml", "--root", "repo path", "--base", "refs/heads/review's", "--lang", "go", "--require-tools"}
	if args := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"); !reflect.DeepEqual(args, want) {
		t.Fatalf("replay args=%q want=%q", args, want)
	}
}
