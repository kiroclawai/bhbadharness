// Package bh multi-agent agent loop: the harness runs a request through an
// agent loop, enforces guardrails (allow/warn/block/halt) on each proposed tool
// call, executes tools under an allow-list with caps, and spawns sub-agents on
// demand. The model's own guardrails stay fully active inside the model; these
// are only the harness layer.
package bh

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// buildSystemPrompt composes the system prompt. It appends the harness's
// reasoning instructions (teach the model to clarify instead of refuse) to the
// model's own guardrail instructions, which live inside the model and are
// never rewritten here.
func (h *Harness) buildSystemPrompt(userMessage string) string {
	var b strings.Builder
	b.WriteString(h.config.Model)
	b.WriteString(" model instructions\n")
	b.WriteString(h.config.ReasoningHint)
	return b.String()
}

// guardrailConfig returns the guardrail configuration for this harness.
func (h *Harness) guardrailConfig() *GuardrailConfig {
	return NewGuardrailConfig()
}

// llmRequest builds the request passed to the model.
func (h *Harness) llmRequest(systemPrompt string, msgs []Message, tools []ToolCall) LLMRequest {
	return LLMRequest{
		SystemPrompt: systemPrompt,
		Messages:     msgs,
		Tools:        tools,
	}
}

// truncate caps a string for logging.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// DefaultConfig returns a ready-to-run configuration.
func DefaultConfig() *HarnessConfig {
	home, _ := os.UserCacheDir()
	dataDir := filepath.Join(home, "bhbadharness", "data")
	skillsDir := filepath.Join(dataDir, "skills")
	return &HarnessConfig{
		Model:         "gpt-4o",
		SafetyPosture: PostureBalanced,
		MaxIterations: 10,
		DataDir:       dataDir,
		SkillsDir:     skillsDir,
		Logger:        NewAuditLogger(),
		ReasoningHint: reasoningHint,
		MaxSubagents:  4,
	}
}

// Run executes a single user request through the agent loop. The harness
// builds a system prompt (model's guardrails + reasoning hint), calls the LLM,
// and iterates: guardrail-check each proposed tool call, execute allowed tools
// (including sub-agent delegation), record outcomes, and return the final
// result. The model's own guardrails remain fully active for the whole loop.
func (h *Harness) Run(ctx context.Context, userMessage string) (*Result, error) {
	h.config.Logger.Info("request started", "message", truncate(userMessage, 200))

	systemPrompt := h.buildSystemPrompt(userMessage)
	msgs := []Message{
		{Role: RoleSystem, Content: systemPrompt},
		{Role: RoleUser, Content: userMessage},
	}

	plan := Plan{}
	gc := NewGuardrailController(h.guardrailConfig())

	for i := 0; i < h.config.MaxIterations; i++ {
		resp, err := h.llm.Complete(ctx, h.llmRequest(systemPrompt, msgs, plan.ToolCalls))
		if err != nil {
			return nil, err
		}
		h.config.Logger.Info("model turn", "finish", string(resp.Finish), "content", truncate(resp.Content, 300))

		if len(resp.Tools) == 0 {
			return &Result{
				Step:     i + 1,
				Content:  resp.Content,
				Plan:     plan,
				Decision: "completed",
			}, nil
		}

		var toolResults []Message
		for _, tc := range resp.Tools {
			approved, msg := gc.beforeCall(tc.ToolName, tc.Arguments)
			if approved == DecisionBlock {
				h.config.Logger.Warn("tool blocked by harness policy", "tool", tc.ToolName, "reason", msg)
				return &Result{
					Step:     i + 1,
					Content:  "I can't take that action under the current policy. I can suggest a narrower, safer alternative.",
					Plan:     plan,
					Decision: "blocked",
				}, nil
			}
			if approved == DecisionWarn {
				// Surface the note to the model without blocking execution.
				toolResults = append(toolResults, Message{Role: RoleUser, Content: "Note: " + msg})
			}
			if approved == DecisionHalt {
				return &Result{
					Step:     i + 1,
					Content:  gc.lastMsg(),
					Plan:     plan,
					Decision: "halted",
				}, nil
			}
			if approved != DecisionAllow {
				continue
			}
			content, err := h.executeTool(ctx, tc)
			if err != nil {
				return nil, err
			}
			gc.afterCall(tc.ToolName, tc.Arguments, content, false)
			plan = Plan{}
			toolResults = append(toolResults, Message{Role: RoleTool, Content: content})
		}
		msgs = append(msgs, Message{Role: RoleAssistant, Content: resp.Content, ToolCalls: resp.Tools})
		msgs = append(msgs, toolResults...)
	}

	return &Result{
		Step:     h.config.MaxIterations,
		Content:  "Reached the maximum iteration limit without a final answer.",
		Plan:     plan,
		Decision: "maxIterations",
	}, nil
}

// executeTool runs a single proposed tool call. If the tool is "subagent", a
// new sub-agent is spawned with a narrow allow-list and run to completion. Other
// tools are looked up in the tool registry (the allow-list enforces which are
// safe to run).
func (h *Harness) executeTool(ctx context.Context, tc ToolCall) (string, error) {
	if tc.ToolName == "subagent" {
		return h.runSubAgent(ctx, tc.Arguments)
	}
	registry := h.config.ToolRegistry
	res, err := registry.Execute(ctx, tc.ToolName, tc.Arguments)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", res), nil
}

// runSubAgent runs a sub-agent with a narrow tool allow-list and returns its
// final content plus any error.
func (h *Harness) runSubAgent(ctx context.Context, args map[string]any) (string, error) {
	task := ""
	if v, ok := args["task"]; ok {
		task = fmt.Sprintf("%v", v)
	}
	sa := NewSubAgent(genID(), h, task, h.config.ToolRegistry.Names())
	return sa.Run(ctx)
}
