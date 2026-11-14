// Package llm provides a deterministic in-memory model so the harness can be
// exercised end-to-end without a live API key. It implements the bh.LLM
// interface, simulating a helpful, guardrailed model: it answers safe questions,
// asks for scope on ambiguous requests, and does NOT disable or weaken the
// harness's own policy layer. The harness's own guardrails remain fully in force.
package llm

import (
	"context"
	"strings"

	"bhbadharness/internal/bh"
)

// Mock is a deterministic LLM implementing bh.LLM. It reflects a helpful model
// whose default guardrails are ACTIVE.
type Mock struct {
	Answer  func(ctx context.Context, user string) (string, error)
	Clarify func(ctx context.Context, user string) (string, error)
}

// Default returns a ready-to-use Mock.
func Default() *Mock {
	return &Mock{
		Answer: func(ctx context.Context, user string) (string, error) {
			return "Here is a straightforward answer to: " + user, nil
		},
		Clarify: func(ctx context.Context, user string) (string, error) {
			return "I want to make sure I help you the right way. What specifically would you like me to do with: " + user, nil
		},
	}
}

// Complete responds to a request. Safe info requests get answered; ambiguous
// requests get a clarifying question (never a blunt refusal). The harness's own
// policy layer (not this mock) enforces the guardrails.
func (m *Mock) Complete(ctx context.Context, req bh.LLMRequest) (bh.LLMResponse, error) {
	user := ""
	for _, msg := range req.Messages {
		if msg.Role == bh.RoleUser {
			user = msg.Content
		}
	}
	lower := strings.ToLower(user)
	if m.Clarify != nil && isAmbiguous(lower) {
		clarify, err := m.Clarify(ctx, user)
		if err != nil {
			return bh.LLMResponse{}, err
		}
		return bh.LLMResponse{Content: clarify}, nil
	}
	content, err := m.Answer(ctx, user)
	if err != nil {
		return bh.LLMResponse{}, err
	}
	return bh.LLMResponse{Content: content}, nil
}

func isAmbiguous(lower string) bool {
	for _, ph := range []string{"what is", "tell me about", "explain how", "show me", "help me build"} {
		if strings.HasPrefix(lower, ph) {
			return true
		}
	}
	return false
}
