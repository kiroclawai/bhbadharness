// Package llm provides the model backend router: given a provider + model +
// API key, returns a bh.LLM implementation. The model's own guardrails stay
// fully active inside the model; the harness never touches them.
package llm

import (
	"bhbadharness/internal/bh"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// NewLLM builds a bh.LLM for the given provider and model. With no API key, a
// deterministic in-memory Mock is returned so the harness runs end-to-end.
// Set BH_API_KEY to use a real provider.
func NewLLM(provider bh.Provider, modelID, apiKey string) bh.LLM {
	return newLLM(provider, modelID, apiKey, "")
}

// NewLLMWithBase builds a bh.LLM for the given provider, model, and explicit
// base URL (private gateway mode). Use this to route through a custom endpoint.
func NewLLMWithBase(provider bh.Provider, modelID, apiKey, baseURL string) bh.LLM {
	return newLLM(provider, modelID, apiKey, baseURL)
}

// newLLM is the internal implementation of NewLLM. It resolves the base URL
// via GetBaseURL (or falls back to the explicit baseURL).
func newLLM(provider bh.Provider, modelID, apiKey, explicitBaseURL string) bh.LLM {
	if apiKey == "" {
		apiKey = os.Getenv("BH_API_KEY")
	}
	if apiKey == "" {
		return Default()
	}
	base := explicitBaseURL
	if base == "" {
		base = GetBaseURL(provider, "BH_API_KEY")
	}
	switch provider {
	case bh.ProviderAnthropic:
		return newAnthropic(apiKey, modelID)
	case bh.ProviderGemini:
		return newGemini(apiKey, modelID)
	case bh.ProviderOllama:
		return newOllama(apiKey, modelID)
	case bh.ProviderOpenAI, bh.ProviderxAI, bh.ProviderOpenRouter:
		return NewClientWithBase(modelID, apiKey, base)
	default:
		return NewClientWithBase(modelID, apiKey, base)
	}
}

// --- Provider base URL resolution (private gateway support) ---

// GetBaseURL returns the effective base URL for a provider. It checks for a
// user-configured override via the BaseURLEnvVar (private gateway mode), then
// falls back to the hardcoded default endpoint.
func GetBaseURL(provider bh.Provider, apiKeyEnvVar string) string {
	// Check for explicit override env var configured by user (private gateway).
	if apiKeyEnvVar == "" {
		apiKeyEnvVar = "BH_API_KEY"
	}
	envVal := os.Getenv(apiKeyEnvVar)
	if envVal != "" {
		// User-provided override: treat as a private gateway endpoint.
		return envVal
	}
	// Use hardcoded defaults (mirroring hermes_cli/providers.py HermesOverlay).
	switch provider {
	case bh.ProviderAnthropic:
		return "https://api.anthropic.com/v1/messages"
	case bh.ProviderGemini:
		return "https://generativelanguage.googleapis.com/v1beta/openai/"
	case bh.ProviderOpenAI:
		return "https://api.openai.com/v1"
	case bh.ProviderxAI:
		return "https://api.x.ai/v1"
	case bh.ProviderOpenRouter:
		return "https://openrouter.ai/api/v1"
	case bh.ProviderOllama:
		return "http://localhost:11434/api"
	default:
		return "https://api.openai.com/v1"
	}
}

// --- OpenAI-compatible backends ---

// newAnthropic builds an Anthropic Messages API adapter.
func newAnthropic(apiKey, modelID string) bh.LLM {
	return &anthropicClient{apiKey: apiKey, modelID: modelID}
}

type anthropicClient struct {
	apiKey  string
	modelID string
	http    *http.Client
}

func (c *anthropicClient) Complete(ctx context.Context, req bh.LLMRequest) (bh.LLMResponse, error) {
	payload := c.buildAnthropicPayload(req)
	url := "https://api.anthropic.com/v1/messages"
	reqHTTP, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(payload))
	if err != nil {
		return bh.LLMResponse{}, err
	}
	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("x-api-key", c.apiKey)
	reqHTTP.Header.Set("anthropic-version", "2023-06-01")
	resp, err := c.http.Do(reqHTTP)
	if err != nil {
		return bh.LLMResponse{}, err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return bh.LLMResponse{}, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var parsed struct {
		Content []rawContentPart `json:"content"`
	}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return bh.LLMResponse{}, err
	}
	return bh.LLMResponse{Content: joinedText(parsed.Content)}, nil
}

func (c *anthropicClient) buildAnthropicPayload(req bh.LLMRequest) string {
	var msgs []map[string]any
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Content})
	}
	p := map[string]any{
		"model":      c.modelID,
		"max_tokens": 1024,
		"system":     req.SystemPrompt,
		"messages":   msgs,
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// rawContentPart is the json-tagged form returned by the provider APIs.
type rawContentPart struct {
	Text string `json:"text"`
}

func joinedText(parts []rawContentPart) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

// newGemini builds a Google Gemini generateContent adapter.
func newGemini(apiKey, modelID string) bh.LLM {
	return &geminiClient{apiKey: apiKey, modelID: "models/" + modelID}
}

type geminiClient struct {
	apiKey  string
	modelID string
	http    *http.Client
}

func (c *geminiClient) Complete(ctx context.Context, req bh.LLMRequest) (bh.LLMResponse, error) {
	system := ""
	if req.SystemPrompt != "" {
		system = req.SystemPrompt
	}
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": req.Messages[len(req.Messages)-1].Content},
				},
			},
		},
		"systemInstruction": map[string]any{
			"parts": []map[string]any{{"text": system}},
		},
	}
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", c.modelID, c.apiKey)
	reqHTTP, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(b)))
	if err != nil {
		return bh.LLMResponse{}, err
	}
	reqHTTP.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(reqHTTP)
	if err != nil {
		return bh.LLMResponse{}, err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return bh.LLMResponse{}, fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return bh.LLMResponse{}, err
	}
	var sb strings.Builder
	for _, c := range parsed.Candidates {
		for _, p := range c.Content.Parts {
			sb.WriteString(p.Text)
		}
	}
	return bh.LLMResponse{Content: sb.String()}, nil
}

// newOllama builds an Ollama chat adapter.
func newOllama(apiKey, modelID string) bh.LLM {
	return &ollamaClient{modelID: modelID}
}

type ollamaClient struct {
	modelID string
	http    *http.Client
}

func (c *ollamaClient) Complete(ctx context.Context, req bh.LLMRequest) (bh.LLMResponse, error) {
	url := "http://localhost:11434/api/chat"
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, map[string]any{"role": m.Role, "content": m.Content})
	}
	payload := map[string]any{
		"model":    c.modelID,
		"stream":   false,
		"messages": messages,
	}
	b, _ := json.Marshal(payload)
	reqHTTP, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(b)))
	if err != nil {
		return bh.LLMResponse{}, err
	}
	reqHTTP.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(reqHTTP)
	if err != nil {
		return bh.LLMResponse{}, err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return bh.LLMResponse{}, fmt.Errorf("ollama API error %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var parsed struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return bh.LLMResponse{}, err
	}
	return bh.LLMResponse{Content: parsed.Message.Content}, nil
}
