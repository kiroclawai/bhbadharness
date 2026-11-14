// Package llm provides the OpenAI-compatible completion adapter implementing
// bh.LLM.
package llm

import (
	"bhbadharness/internal/bh"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client wraps an http.Client for OpenAI-compatible APIs.
type Client struct {
	HTTP  *http.Client
	Token string
	Base  string
	Model string
}

// NewClient builds an OpenAI-compatible client (OpenAI, xAI, OpenRouter).
func NewClient(model, token string) *Client {
	if token == "" {
		token = "sk-noop"
	}
	return &Client{HTTP: http.DefaultClient, Token: token, Base: "https://api.openai.com/v1", Model: model}
}

// NewClientWithBase builds a client with an explicit base URL (for x.ai,
// OpenRouter, etc.).
func NewClientWithBase(model, token, base string) *Client {
	if token == "" {
		token = "sk-noop"
	}
	return &Client{HTTP: http.DefaultClient, Token: token, Base: base, Model: model}
}

// Complete sends a completion request to the model.
func (c *Client) Complete(ctx context.Context, req bh.LLMRequest) (bh.LLMResponse, error) {
	payload := c.encode(req)
	body := bytes.NewReader(payload)
	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/chat/completions", body)
	if err != nil {
		return bh.LLMResponse{}, err
	}
	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(reqHTTP)
	if err != nil {
		return bh.LLMResponse{}, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return bh.LLMResponse{}, err
	}
	if resp.StatusCode >= 400 {
		return bh.LLMResponse{}, fmt.Errorf("model API error %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return bh.LLMResponse{}, err
	}
	if parsed.Error.Message != "" {
		return bh.LLMResponse{}, fmt.Errorf("model API error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return bh.LLMResponse{}, fmt.Errorf("model returned no choice")
	}
	return bh.LLMResponse{
		Content: parsed.Choices[0].Message.Content,
	}, nil
}

func (c *Client) encode(req bh.LLMRequest) []byte {
	type payload struct {
		Model     string        `json:"model"`
		System    string        `json:"system"`
		Messages  []bh.Message  `json:"messages"`
		Tools     []bh.ToolCall `json:"tools"`
		MaxTokens int           `json:"max_tokens,omitempty"`
	}
	p := payload{
		Model:     c.Model,
		System:    req.SystemPrompt,
		Messages:  req.Messages,
		Tools:     req.Tools,
		MaxTokens: 1024,
	}
	b, _ := json.Marshal(p)
	return b
}
