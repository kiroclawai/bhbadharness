// Package bh model/provider selector tests.
package bh

import (
	"testing"
)

func TestNormalizeProvider(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"openai", "openai"},               // OpenAI (canonical)
		{"claude", "anthropic"},            // "claude" is alias for "anthropic"
		{"claude-code", "anthropic"},       // "claude-code" is alias for "anthropic"
		{"x-ai", "xai"},                    // "x-ai" is alias for "xai"
		{"x.ai", "xai"},                    // "x.ai" is alias for "xai"
		{"grok", "xai"},                    // "grok" is alias for "xai"
		{"z-ai", "zai"},                    // "z-ai" is alias for "zai"
		{"glm", "zai"},                     // "glm" is alias for "zai"
		{"kilocode", "kilocode"},           // "kilocode" is canonical
		{"kilo-code", "kilocode"},          // "kilo-code" is alias for "kilocode"
		{"kimi-coding", "kimi-for-coding"}, // "kimi-coding" is alias for "kimi-for-coding"
		{"openai-codex", "openai-codex"},   // "openai-codex" is canonical
		{"custom", "custom"},               // "custom" is canonical
		{"ollama", "custom"},               // "ollama" is alias for "custom"
	}
	for _, c := range cases {
		if got := normalizeProvider(c.in); got != c.want {
			t.Errorf("normalizeProvider(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeModelForProvider(t *testing.T) {
	cases := []struct {
		model    string
		provider Provider
		want     string
		desc     string
	}{
		// Aggregators: prepend vendor
		{"gpt-4o", ProviderOpenRouter, "openai/gpt-4o", "openrouter prepend vendor"},
		{"claude-sonnet-4", ProviderOpenRouter, "anthropic/claude-sonnet-4", "openrouter prepend anthropic vendor"},
		{"grok-2", ProviderxAI, "grok-2", "xai is not aggregator; pass through as-is"},
		{"llama3.3", ProviderOpenRouter, "meta-llama/llama3.3", "openrouter prepend meta-llama vendor"},
		// Dot-to-hyphen (anthropic)
		{"claude-sonnet-4.6", ProviderAnthropic, "claude-sonnet-4-6", "anthropic dots-to-hyphens"},
		// Matching-prefix-strip (xai): strip provider/ when it matches target
		{"x-ai/grok-2", ProviderxAI, "grok-2", "xai strip vendor prefix"},
		// Authoritative native: pass through unchanged (huggingface)
		{"meta-llama/Llama-3", ProviderOpenRouter, "meta-llama/Llama-3", "huggingface pass through"},
		// Unknown / custom: pass through as-is
		{"my-custom-model", ProviderOpenAI, "my-custom-model", "custom pass through"},
	}
	for _, c := range cases {
		got := normalizeModelForProvider(c.model, c.provider)
		if got != c.want {
			t.Errorf("normalizeModelForProvider(%q, %q) = %q, want %q (%s)", c.model, c.provider, got, c.want, c.desc)
		}
	}
}

func TestProviderRegistry(t *testing.T) {
	pr := NewProviderRegistry()
	if pr == nil {
		t.Fatal("NewProviderRegistry() = nil")
	}

	// Register (anthropic alias "claude")
	p := ProviderDef{
		ID:            "anthropic",
		Name:          "Anthropic",
		Transport:     "anthropic_messages",
		APIKeyEnvVars: []string{"ANTHROPIC_API_KEY"},
		BaseURL:       "https://api.anthropic.com/v1",
		IsAggregator:  false,
		AuthType:      "api_key",
		Source:        "models.dev",
	}
	if err := pr.Register(p); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// List
	listed := pr.List()
	if len(listed) != 1 {
		t.Fatalf("List() len = %d, want 1", len(listed))
	}

	// Get by canonical id
	if _, ok := pr.Get("anthropic"); !ok {
		t.Errorf("Get(\"anthropic\") = nil, ok=false")
	}

	// Get by alias
	if _, ok := pr.Get("claude"); !ok {
		t.Errorf("Get(\"claude\") = nil, ok=false")
	}

	// Get by alias
	if _, ok := pr.Get("claude-code"); !ok {
		t.Errorf("Get(\"claude-code\") = nil, ok=false")
	}
}

func TestModelRegistry(t *testing.T) {
	mr := NewModelRegistry()

	// Register models
	mr.Register(ModelDef{
		ID:         "gpt-4o",
		Name:       "GPT-4o",
		Provider:   ProviderOpenAI,
		ContextLen: 128000,
		IsLocal:    false,
	})
	mr.Register(ModelDef{
		ID:         "claude-sonnet-4-6",
		Name:       "Claude Sonnet 4.6",
		Provider:   ProviderAnthropic,
		ContextLen: 200000,
		IsLocal:    false,
	})

	// ByProvider
	openaiModels := mr.ByProvider(ProviderOpenAI)
	if len(openaiModels) != 1 {
		t.Errorf("ByProvider(openai) len = %d, want 1", len(openaiModels))
	}

	// All
	all := mr.All()
	if len(all) != 2 {
		t.Errorf("All() len = %d, want 2", len(all))
	}

	// Find
	if _, ok := mr.Find(ProviderAnthropic, "claude-sonnet-4-6"); !ok {
		t.Errorf("Find(anthropic, claude-sonnet-4-6) = false")
	}
}

func TestNormalizeModelForProviderAggregator(t *testing.T) {
	cases := []struct {
		model    string
		provider Provider
		want     string
		desc     string
	}{
		{"gpt-4o", ProviderOpenRouter, "openai/gpt-4o", "openrouter prepend vendor"},
		{"claude-sonnet-4", ProviderOpenRouter, "anthropic/claude-sonnet-4", "openrouter prepend anthropic vendor"},
		{"llama3.3", ProviderOpenRouter, "meta-llama/llama3.3", "openrouter prepend meta-llama vendor"},
	}
	for _, c := range cases {
		got := normalizeModelForProvider(c.model, c.provider)
		if got != c.want {
			t.Errorf("aggregator normalize: model=%q provider=%q got=%q want=%q", c.model, c.provider, got, c.want)
		}
	}
}

func TestNormalizeModelForProviderVendor(t *testing.T) {
	// Non-aggregator providers with vendor prefix should strip it
	model := normalizeModelForProvider("anthropic/claude-sonnet-4", ProviderAnthropic)
	if model != "claude-sonnet-4" {
		t.Errorf("expected 'claude-sonnet-4', got %q", model)
	}
}

func TestNormalizeModelForProviderDotToHyphen(t *testing.T) {
	model := normalizeModelForProvider("claude-sonnet-4.6", ProviderAnthropic)
	if model != "claude-sonnet-4-6" {
		t.Errorf("expected 'claude-sonnet-4-6', got %q", model)
	}
}
