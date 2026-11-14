// Package bh sub-agent delegation: a sub-agent runs an isolated agent loop
// with its own narrow tool allow-list and a shared LLM. The parent harness
// spawns them on demand (bounded by loop caps) and feeds their results back.
package bh

import (
	"context"
	"fmt"
)

// SubAgent runs an isolated agent loop with a narrow tool allow-list. It
// shares the parent's LLM and guardrail config but cannot spawn further
// sub-agents (cap-bound).
type SubAgent struct {
	id     string
	task   string
	child  *Harness
	result chan string
	err    chan error
}

// NewSubAgent builds a sub-agent under parent, limited to allowedTools.
func NewSubAgent(id string, parent *Harness, task string, allowedTools []string) *SubAgent {
	child := buildChildHarness(parent, allowedTools)
	sa := &SubAgent{
		id:     id,
		task:   task,
		child:  child,
		result: make(chan string, 1),
		err:    make(chan error, 1),
	}
	return sa
}

// Run executes the sub-agent's loop and returns its final content and any error.
func (sa *SubAgent) Run(ctx context.Context) (string, error) {
	// Build the initial system prompt: parent's reasoning hint + task framing.
	systemPrompt := fmt.Sprintf("%s\n\nYou are a focused sub-agent. Your single task is:\n%s\n", sa.child.config.ReasoningHint, sa.task)
	msgs := []Message{
		{Role: RoleSystem, Content: systemPrompt},
		{Role: RoleUser, Content: sa.task},
	}
	child := sa.child
	child.config.MaxIterations = 12 // sub-agents get a little more room
	// Sub-agents don't spawn their own sub-agents; they only use their allow-list.
	child.config.MaxSubagents = 0
	for i := 0; i < child.config.MaxIterations; i++ {
		resp, err := sa.child.llm.Complete(ctx, sa.child.llmRequest(systemPrompt, msgs, nil))
		if err != nil {
			sa.err <- err
			return "", err
		}
		// Build the messages the child will continue from.
		childMsgs := append([]Message{{Role: "system", Content: systemPrompt}}, msgs...)
		// Append the assistant response.
		assistantMsg := Message{Role: RoleAssistant, Content: resp.Content}
		if len(resp.Tools) > 0 {
			assistantMsg.ToolCalls = resp.Tools
		}
		childMsgs = append(childMsgs, assistantMsg)
		for _, tc := range resp.Tools {
			// If the sub-agent itself asks to delegate (subagent tool), run it synchronously.
			if tc.ToolName == "subagent" {
				args := tc.Arguments
				innerTask := ""
				if v, ok := args["task"]; ok {
					innerTask = fmt.Sprintf("%v", v)
				}
				// Re-spawn a sub-sub-agent.
				sa2 := NewSubAgent(sa.id+"->"+genID(), sa.child, innerTask, sa.child.config.ToolRegistry.Names())
				content, err := sa2.Run(ctx)
				if err != nil {
					sa.err <- err
					return "", err
				}
				childMsgs = append(childMsgs, Message{Role: RoleTool, Content: "sub-agent " + content})
				continue
			}
			// Execute an ordinary tool.
			res, err := sa.child.toolRegistry.Execute(ctx, tc.ToolName, tc.Arguments)
			if err != nil {
				childMsgs = append(childMsgs, Message{Role: RoleTool, Content: fmt.Sprintf("[%s] error: %v", tc.ToolName, err)})
				continue
			}
			childMsgs = append(childMsgs, Message{Role: RoleTool, Content: fmt.Sprintf("[%s] %v", tc.ToolName, res)})
		}
		msgs = childMsgs
		if resp.Content != "" {
			return resp.Content, nil
		}
	}
	return "Sub-agent ran out of iterations without a final answer.", nil
}
