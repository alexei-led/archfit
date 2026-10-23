package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const aiEffortField = "ai.effort"

func TestAIConfiguration(t *testing.T) {
	cases := []struct{ name, ai, want string }{
		{"deepseek", "provider: deepseek\n  model: deepseek-flash\n  effort: high\n  max_tokens: 65536\n  timeout: 3m", ""},
		{"compatible", "provider: openai-compatible\n  model: example\n  base_url: https://example.test/v1\n  api_key_env: EXAMPLE_KEY", ""},
		{"keyless", "provider: openai-compatible\n  model: example\n  base_url: http://localhost:8000/v1", ""},
		{"custom openai", "provider: openai\n  model: gpt-6.1-sol\n  base_url: https://example.test/v1\n  effort: max", ""},
		{"missing model", "provider: deepseek", "ai.model"},
		{"missing provider", "model: example", "ai.provider"},
		{"unknown provider", "provider: typo\n  model: example", "ai.provider"},
		{"missing endpoint", "provider: openai-compatible\n  model: example", "ai.base_url"},
		{"credential url", "provider: openai-compatible\n  model: example\n  base_url: https://user:secret@example.test", "ai.base_url"},
		{"invalid effort", "provider: openai\n  model: gpt-6.1-sol\n  effort: urgent", aiEffortField},
		{"sol none", "provider: openai\n  model: gpt-6.1-sol\n  effort: none", aiEffortField},
		{"opus none", "provider: anthropic\n  model: claude-opus-5-5\n  effort: none", aiEffortField},
		{"deepseek medium", "provider: deepseek\n  model: deepseek-flash\n  effort: medium", aiEffortField},
		{"negative cap", "provider: openai\n  model: gpt-6.1-sol\n  max_tokens: -1", "ai.max_tokens"},
		{"bad timeout", "provider: openai\n  model: gpt-6.1-sol\n  timeout: tomorrow", "ai.timeout"},
		{"zero timeout", "provider: openai\n  model: gpt-6.1-sol\n  timeout: 0s", "ai.timeout"},
		{"misplaced key env", "provider: deepseek\n  model: deepseek-flash\n  api_key_env: KEY", "ai.api_key_env"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".archfit.yaml")
			if err := os.WriteFile(path, []byte("version: 2\nai:\n  "+tc.ai+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(t.Context(), path)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error=%v, want %s", err, tc.want)
				}
				if strings.Contains(err.Error(), "secret") {
					t.Fatalf("credential leaked: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			resolved, ok := cfg.LLM()
			if !ok || resolved.Provider == "" || resolved.Model == "" {
				t.Fatalf("unusable configuration: %+v", resolved)
			}
			if tc.name == "deepseek" && (resolved.Effort != "high" || resolved.MaxTokens != 65536 || resolved.Timeout != 3*time.Minute) {
				t.Fatalf("settings lost: %+v", resolved)
			}
		})
	}
}
