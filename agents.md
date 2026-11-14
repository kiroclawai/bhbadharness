# BH BadHarness v2 — Agents & Architecture

## Project Overview

**BH BadHarness v2** is a Go (Go 1.22.5) agent harness that:
- Rarely refuses legitimate requests while keeping the model's own safety guardrails **active and unmodified**
- Adds a separate harness guardrail layer: allow/warn/block/halt verdicts
- Per-tool allow-lists, sandboxing, audit logging
- Sub-agent isolation
- Persistent memory
- Scripted skills library
- opencode/kilocode-style model/provider selection

## Design Principles

1. **Model guardrails untouched** — the model's refusal decision stays fully intact. The harness never disables, weakens, or rewrites model guardrails.
2. **Rarely refuses** — clarify ambiguous-but-safe requests, offer staged delegation, propose safer alternatives, scope sub-agents.
3. **Go over Python** — explicitly built in Go 1.22.5.
4. **Two-layer guardrail design** — model guardrails untouched; harness own policy layer with `SafetyPosture` (conservative|balanced|agile).

## Package Structure

```
bhbadharness/
├── go.mod                    # module bhbadharness, Go 1.22
├── main.go                   # CLI entry point
├── internal/
│   └── bh/
│   │   ├── types.go          # roles, messages, ToolCall, LLM/LLMRequest/LLMResponse, ToolRegistry, AuditLogger, HarnessConfig, Harness
│   │   ├── lib.go            # shared helpers (genID, lowercase, SafeJSON, readFileOr, SkillsDir, buildChildHarness)
│   │   ├── memory.go         # MemoryEntry, MemoryStore (JSON persistence)
│   │   ├── skills.go         # Skill, Skills, NewSkills, load, parseSkill, parseSkillBody
│   │   ├── delegate.go       # SubAgent, NewSubAgent, Run (isolated sub-agent loop)
│   │   ├── guardrails.go     # Decision verdicts (allow/warn/block/halt), GuardrailController, Signature, noProgressEntry, per-signature observation
│   │   ├── harness.go        # Run (agent loop), buildSystemPrompt, guardrailConfig, DefaultConfig, New
│   │   ├── models.go         # Provider enum, Model struct, Models registry
│   │   └── selector.go       # ProviderDef, ModelDef, ProviderRegistry, ModelRegistry, ModelSelector, normalizeProvider, normalizeModelForProvider
│   └── llm/
│       ├── backend.go        # NewLLM router: Anthropic/Gemini/Ollama/OpenAI/xAI/OpenRouter → bh.LLM
│       ├── openai.go         # Client (OpenAI-compatible), NewClient, NewClientWithBase, Complete
│       ├── mock.go           # Mock LLM for no-key end-to-end runs
│       └── mock_test.go      # Tests (TestMockClarify, TestMockAnswer, TestNewLLM)
└── README.md
```

## Configuration

| Flag / Env | Type | Default | Description |
|------------|------|---------|-------------|
| `--model` / `BH_MODEL` | string | `gpt-4o` | Model id passed through to the LLM |
| `--posture` | string | `balanced` | Harness guardrail posture: `conservative`, `balanced`, `agile` |
| `--clarify-hint` | string | (built-in) | Standing instruction appended to the prompt |
| `--log` / `BH_LOG` | string | (none) | Audit log file path |

`DefaultConfig()` sets:
- `Model: "gpt-4o"`
- `SafetyPosture: PostureBalanced`
- `MaxIterations: 10`
- `DataDir` under user cache
- `SkillsDir` under data
- `Logger: NewAuditLogger()`
- `ReasoningHint: reasoningHint` constant
- `MaxSubagents: 4`

## Guardrail Controller

`GuardrailController` implements allow/warn/block/halt verdicts with:
- Per-signature observation tracking (`Signature` = tool name + args hash)
- Fail streaks (`exactFailures`, `sameToolFailures`)
- Stall-loop detection (identical-call streak, `checkStall`, `applyStallDetectors`)
- No-progress detection (idempotent read-only results)
- Per-turn caps (web_search, subagent)

## Agent Architecture

The harness runs a user request through an agent loop:
1. `Run(ctx, request)` — main entry point
2. Build system prompt (with `ReasoningHint` constant)
3. Call model via `llm.Complete(ctx, req)`
4. Parse tool calls (if any)
5. GuardrailController checks each tool call (allow/warn/block/halt)
6. Execute allow-listed tools
7. Repeat until completion or guardrail halt
8. Return `Result`

## Multiple Agents

- `Harness` is the root agent
- `buildChildHarness(parent, allowedTools)` creates isolated sub-agents with narrow tool allow-lists
- `SubAgent` struct in `delegate.go` manages isolated agent loops
- `MaxSubagents: 4` default cap

## Key Data Structures

| Type | Description |
|------|-------------|
| `Decision` | Guardrail verdict: `allow`, `warn`, `block`, `halt` |
| `GuardrailController` | Per-turn tool-call observation and stall detection |
| `Signature` | Stable identity: tool name + args hash |
| `ToolGuardrailDecision` | Verdict with action, code, message |
| `HarnessConfig` | Configuration: model, posture, maxIterations, data dir, skills dir, logger, reasoning hint, maxSubagents |
| `Harness` | Root agent with config, LLM, memory, skills, tool registry |
| `MemoryStore` | Persistent memory (one JSON file per key) |
| `Skills` | Scripted skills library with front-matter parsing |
| `SubAgent` | Isolated sub-agent loop |
