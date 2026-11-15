// Package bh model registry: named models grouped by provider, with the
// ability to select a provider + model (opencode/kilocode-style selection).
package bh

import (
	"fmt"
	"sort"
	"strings"
)

// Provider is a model provider.
type Provider string

const (
	ProviderOpenAI     Provider = "openai"
	ProviderAnthropic  Provider = "anthropic"
	ProviderGemini     Provider = "gemini"
	ProviderxAI        Provider = "xai"
	ProviderOllama     Provider = "ollama"
	ProviderOpenRouter Provider = "openrouter"
	// Extended providers (opencode/kilocode/commandcode/ollama-cloud families)
	ProviderCommandCode          Provider = "commandcode"
	ProviderCommandCodeAnthropic Provider = "commandcode-anthropic"
	ProviderKiloCode             Provider = "kilocode"
	ProviderOpenCodeZen          Provider = "opencode-zen"
	ProviderOpenCodeGo           Provider = "opencode-go"
	ProviderOllamaCloud          Provider = "ollama-cloud"
)

func (p Provider) String() string {
	return string(p)
}

// NormalizeProvider normalizes a provider name to its canonical form.
func NormalizeProvider(s string) string {
	switch {
	case strings.Contains(strings.ToLower(s), "claude"):
		return "anthropic"
	case strings.Contains(strings.ToLower(s), "grok") || strings.Contains(strings.ToLower(s), "x-ai"):
		return "xai"
	case strings.Contains(strings.ToLower(s), "gemini"):
		return "gemini"
	case strings.Contains(strings.ToLower(s), "chatgpt") || strings.Contains(strings.ToLower(s), "gpt"):
		return "openai"
	case strings.Contains(strings.ToLower(s), "llama"):
		return "llama" // will be normalized by model selection
	case strings.Contains(strings.ToLower(s), "kimi"):
		return "kimi"
	case strings.Contains(strings.ToLower(s), "glm"):
		return "glm"
	case strings.Contains(strings.ToLower(s), "qwen"):
		return "qwen"
	case strings.Contains(strings.ToLower(s), "minimax"):
		return "minimax"
	case strings.Contains(strings.ToLower(s), "deepseek"):
		return "deepseek"
	case strings.Contains(strings.ToLower(s), "codex"):
		return "codex"
	default:
		return s
	}
}

// IsOpenAIStarWay reports whether the provider is OpenAI-compatible (chat
// completions format).
func (p Provider) IsOpenAICompatible() bool {
	switch p {
	case ProviderOpenAI, ProviderxAI, ProviderOpenRouter:
		return true
	}
	return false
}

// Model is a named model with a provider, display name, and local flag.
type Model struct {
	ID       string // provider-specific model id
	Name     string // display name
	Provider Provider
	IsLocal  bool // runs on-device (e.g. Ollama, local GGUF)
}

// Models is the registry of available models, grouped by provider.
type Models struct {
	byProvider map[Provider][]Model
}

// DefaultModels returns a built-in catalog of commonly available models.
func DefaultModels() *Models {
	m := &Models{byProvider: map[Provider][]Model{}}
	add := func(provider Provider, models ...Model) {
		m.byProvider[provider] = models
	}
	add(ProviderOpenAI,
		Model{ID: "gpt-4o", Name: "GPT-4o", IsLocal: false},
		Model{ID: "gpt-4o-mini", Name: "GPT-4o mini", IsLocal: false},
		Model{ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo", IsLocal: false},
	)
	add(ProviderxAI,
		Model{ID: "grok-2", Name: "Grok 2", IsLocal: false},
		Model{ID: "grok-2-lite", Name: "Grok 2 Lite", IsLocal: false},
	)
	add(ProviderAnthropic,
		Model{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", IsLocal: false},
		Model{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", IsLocal: false},
	)
	add(ProviderGemini,
		Model{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", IsLocal: false},
		Model{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", IsLocal: false},
	)
	add(ProviderOllama,
		Model{ID: "llama3.3", Name: "Llama 3.3 70B", IsLocal: true},
		Model{ID: "qwen3-8b", Name: "Qwen3 8B", IsLocal: true},
		Model{ID: "phi4", Name: "Phi-4", IsLocal: true},
	)
	add(ProviderOpenRouter,
		Model{ID: "openai/gpt-4o", Name: "GPT-4o (OpenRouter)", IsLocal: false},
		Model{ID: "anthropic/claude-sonnet-4", Name: "Claude Sonnet 4 (OpenRouter)", IsLocal: false},
	)
	// Extended providers (opencode/kilocode/commandcode/ollama-cloud families)
	add(ProviderCommandCode,
		Model{ID: "deepseek/deepseek-v4-pro", Name: "DeepSeek V4 Pro (CommandCode)", IsLocal: false},
		Model{ID: "deepseek/deepseek-v4-flash", Name: "DeepSeek V4 Flash (CommandCode)", IsLocal: false},
		Model{ID: "Qwen/Qwen3.7-Max", Name: "Qwen3.7-Max (CommandCode)", IsLocal: false},
		Model{ID: "Qwen/Qwen3.6-Plus", Name: "Qwen3.6-Plus (CommandCode)", IsLocal: false},
		Model{ID: "moonshotai/Kimi-K2.6", Name: "Kimi K2.6 (CommandCode)", IsLocal: false},
		Model{ID: "zai-org/GLM-5.1", Name: "GLM-5.1 (CommandCode)", IsLocal: false},
		Model{ID: "gpt-5.5", Name: "GPT-5.5 (CommandCode)", IsLocal: false},
	)
	add(ProviderCommandCodeAnthropic,
		Model{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6 (CommandCode)", IsLocal: false},
		Model{ID: "claude-opus-4-7", Name: "Claude Opus 4.7 (CommandCode)", IsLocal: false},
		Model{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5 (CommandCode)", IsLocal: false},
	)
	add(ProviderKiloCode,
		Model{ID: "google/gemini-3.6-flash", Name: "Gemini 3.6 Flash (KiloCode)", IsLocal: false},
		Model{ID: "openai/gpt-4o", Name: "GPT-4o (KiloCode)", IsLocal: false},
		Model{ID: "anthropic/claude-sonnet-4", Name: "Claude Sonnet 4 (KiloCode)", IsLocal: false},
	)
	add(ProviderOpenCodeZen,
		Model{ID: "gemini-3-flash", Name: "Gemini 3 Flash (OpenCode Zen)", IsLocal: false},
		Model{ID: "gpt-4o", Name: "GPT-4o (OpenCode Zen)", IsLocal: false},
	)
	add(ProviderOpenCodeGo,
		Model{ID: "glm-5", Name: "GLM-5 (OpenCode Go)", IsLocal: false},
		Model{ID: "gemini-3-flash", Name: "Gemini 3 Flash (OpenCode Go)", IsLocal: false},
		Model{ID: "deepseek/deepseek-v4-flash", Name: "DeepSeek V4 Flash (OpenCode Go)", IsLocal: false},
	)
	add(ProviderOllamaCloud,
		Model{ID: "nemotron-3-nano:30b", Name: "Nemotron 3 Nano 30B (Ollama Cloud)", IsLocal: false},
		Model{ID: "llama3.3", Name: "Llama 3.3 70B (Ollama Cloud)", IsLocal: false},
		Model{ID: "qwen3-8b", Name: "Qwen3 8B (Ollama Cloud)", IsLocal: false},
	)
	return m
}

// List returns the models for a provider.
func (m *Models) List(provider Provider) []Model {
	models := m.byProvider[provider]
	out := make([]Model, len(models))
	copy(out, models)
	return out
}

// All returns every model across all providers.
func (m *Models) All() []Model {
	seen := map[string]Model{}
	for _, models := range m.byProvider {
		for _, md := range models {
			seen[md.ID] = md
		}
	}
	out := make([]Model, 0, len(seen))
	for _, md := range seen {
		out = append(out, md)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Find returns a model by (provider, modelID).
func (m *Models) Find(provider Provider, modelID string) (Model, bool) {
	for _, md := range m.byProvider[provider] {
		if md.ID == modelID {
			return md, true
		}
	}
	return Model{}, false
}

// Selected resolves the effective model for a provider + modelID selection.
// If provider has no matching model, returns the first model for that provider.
func (m *Models) Selected(provider Provider, modelID string) (Model, bool) {
	if md, ok := m.Find(provider, modelID); ok {
		return md, true
	}
	models := m.byProvider[provider]
	if len(models) == 0 {
		return Model{}, false
	}
	return models[0], true
}

// String returns a "provider/model" label for a model.
func (m Model) String() string {
	return fmt.Sprintf("%s/%s", m.Provider, m.ID)
}
