package llm

import (
	"context"
	"testing"

	"bhbadharness/internal/bh"
)

// TestMockClarify verifies the mock's "clarify instead of refuse" behavior.
func TestMockClarify(t *testing.T) {
	m := Default()
	resp, err := m.Complete(context.Background(), bh.LLMRequest{
		Messages: []bh.Message{{Role: bh.RoleUser, Content: "show me how to read a file"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.Content == "" {
		t.Errorf("expected clarifying content, got empty")
	}
}

// TestMockAnswer verifies the mock answers safe questions.
func TestMockAnswer(t *testing.T) {
	m := Default()
	resp, err := m.Complete(context.Background(), bh.LLMRequest{
		Messages: []bh.Message{{Role: bh.RoleUser, Content: "what is a goroutine?"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.Content == "" {
		t.Errorf("expected content, got empty")
	}
}

// TestNewLLM verifies the provider router dispatches to the correct backend.
func TestNewLLM(t *testing.T) {
	// OpenAI-compatible
	c := NewLLM(bh.ProviderOpenAI, "gpt-4o", "sk-noop")
	if c == nil {
		t.Fatal("NewLLM(ProviderOpenAI) returned nil")
	}
	if c2, ok := c.(*Client); !ok {
		t.Fatalf("NewLLM(ProviderOpenAI) returned %T, want *Client", c)
	} else if c2.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", c2.Model, "gpt-4o")
	}

	// Anthropic
	a := NewLLM(bh.ProviderAnthropic, "claude-sonnet-4", "sk-noop")
	if a == nil {
		t.Fatal("NewLLM(ProviderAnthropic) returned nil")
	}
	if _, ok := a.(*anthropicClient); !ok {
		t.Fatalf("NewLLM(ProviderAnthropic) returned %T, want *anthropicClient", a)
	}

	// Gemini
	g := NewLLM(bh.ProviderGemini, "gemini-2.0-flash", "sk-noop")
	if g == nil {
		t.Fatal("NewLLM(ProviderGemini) returned nil")
	}
	if _, ok := g.(*geminiClient); !ok {
		t.Fatalf("NewLLM(ProviderGemini) returned %T, want *geminiClient", g)
	}

	// Ollama
	o := NewLLM(bh.ProviderOllama, "llama3.3", "sk-noop")
	if o == nil {
		t.Fatal("NewLLM(ProviderOllama) returned nil")
	}
	if _, ok := o.(*ollamaClient); !ok {
		t.Fatalf("NewLLM(ProviderOllama) returned %T, want *ollamaClient", o)
	}

	// xAI
	x := NewLLM(bh.ProviderxAI, "grok-2", "sk-noop")
	if x == nil {
		t.Fatal("NewLLM(ProviderxAI) returned nil")
	}
	if _, ok := x.(*Client); !ok {
		t.Fatalf("NewLLM(ProviderxAI) returned %T, want *Client", x)
	}
	if x.(*Client).Base != "https://api.x.ai/v1" {
		t.Errorf("xAI base = %q, want %q", x.(*Client).Base, "https://api.x.ai/v1")
	}

	// OpenRouter
	r := NewLLM(bh.ProviderOpenRouter, "openai/gpt-4o", "sk-noop")
	if r == nil {
		t.Fatal("NewLLM(ProviderOpenRouter) returned nil")
	}
	if _, ok := r.(*Client); !ok {
		t.Fatalf("NewLLM(ProviderOpenRouter) returned %T, want *Client", r)
	}
	if r.(*Client).Base != "https://openrouter.ai/api/v1" {
		t.Errorf("OpenRouter base = %q, want %q", r.(*Client).Base, "https://openrouter.ai/api/v1")
	}
}
