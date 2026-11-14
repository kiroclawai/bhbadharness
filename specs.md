# BH BadHarness v2 — Specifications

## Architecture Specification

### Two-Layer Guardrail Design
- **Layer 1 (Model Guardrails):** Model's own safety guardrails remain ACTIVE and UNMODIFIED. The model's refusal decision stays fully intact.
- **Layer 2 (Harness Guardrail Policy):** Separate policy layer on top of the model:
  - Allow/warn/block/halt verdicts per tool call
  - Per-tool allow-lists
  - Sandboxing (design-level)
  - Audit logging

### Agents
- `Harness` is the root agent
- `SubAgent` for isolated sub-agent loops with narrow tool allow-lists
- `buildChildHarness(parent, allowedTools)` creates isolated child harness
- `MaxSubagents: 4` default cap

### Model/Provider Selection
- `Provider` enum: `openai`, `anthropic`, `gemini`, `xai`, `ollama`, `openrouter`
- `Model` struct: id, name, provider, isLocal
- `ProviderDef` struct: id, name, transport, API key env vars, base URL, auth type
- `ModelSelector` + `ModelRegistry` for catalog and per-provider normalization
- `ProviderRegistry` for global + per-scope provider map

### Backend Routing
- `NewLLM(provider, modelID, apiKey)` routes to:
  - Anthropic: `https://api.anthropic.com/v1/messages`
  - Gemini: `https://generativelanguage.googleapis.com/v1beta/models/{id}:generateContent`
  - Ollama: `http://localhost:11434/api/chat`
  - OpenAI-compatible: `https://api.openai.com/v1` (xAI: `https://api.x.ai/v1`, OpenRouter: `https://openrouter.ai/api/v1`)

## Core Types

### Guardrail
- `Decision` — `allow`, `warn`, `block`, `halt`
- `GuardrailController` — per-turn tool-call observation, stall detection, loop caps
- `Signature` — tool name + args hash
- `ToolGuardrailDecision` — verdict with action, code, message

### Agent
- `Harness` — root agent with `config *HarnessConfig`, `llm LLM`, `memory *MemoryStore`, `skills *Skills`, `toolRegistry *ToolRegistry`
- `HarnessConfig` — model, posture, maxIterations, dataDir, skillsDir, toolRegistry, logger, reasoningHint, maxSubagents
- `SubAgent` — isolated sub-agent loop

## Backend Specification

### OpenAI-compatible Client
- `NewClient(model, token)` — creates OpenAI-compatible client
- `NewClientWithBase(model, token, base)` — custom base URL (xAI, OpenRouter)
- `Complete(ctx, req)` — sends `/chat/completions` with `Authorization: Bearer {token}`
- Payload: `model`, `system`, `messages`, `tools`, `max_tokens`

### Anthropic Client
- `newAnthropic(apiKey, modelID)` — Anthropic Messages API adapter
- Endpoint: `https://api.anthropic.com/v1/messages`
- Headers: `Content-Type: application/json`, `x-api-key`, `anthropic-version: 2023-06-01`
- Payload: `model`, `max_tokens: 1024`, `system`, `messages`

### Gemini Client
- `newGemini(apiKey, modelID)` — Google Gemini adapter
- Endpoint: `https://generativelanguage.googleapis.com/v1beta/models/{id}:generateContent?key={key}`

### Ollama Client
- `newOllama(apiKey, modelID)` — Ollama chat adapter
- Endpoint: `http://localhost:11434/api/chat`

### Mock LLM
- `Default()` returns deterministic in-memory mock
- `Complete(ctx, req)` — answers safe questions, asks for scope on ambiguous requests

## CLI Specification

### Flags
- `--model` / `BH_MODEL` — model id
- `--posture` — `conservative`, `balanced`, `agile`
- `--clarify-hint` — standing instruction appended to the prompt
- `--log` / `BH_LOG` — audit log file path

### Behavior
- Falls back to in-memory mock when `BH_API_KEY` unset
- Registers `read_file` and `web_search` tools by default
- Only allow-listed tools may run (`registry.Allow()`)
- Guardrail controller checks each tool call

## Configuration Specification

### DefaultConfig
- `Model: "gpt-4o"`
- `SafetyPosture: PostureBalanced`
- `MaxIterations: 10`
- `DataDir` under user cache
- `SkillsDir` under data
- `Logger: NewAuditLogger()`
- `ReasoningHint: reasoningHint` constant
- `MaxSubagents: 4`

### SafetyPosture
- `PostureConservative` — blocks `shell`, `execute`, `run_command`
- `PostureBalanced` — default
- `PostureAgile` — permissive (observed)

## Audit Logging Specification

### AuditLogger
- `NewAuditLogger()` — creates logger, opens `BH_LOG` file if set
- `Info(event string, fields ...string)` — informational event
- `Warn(event string, fields ...string)` — warning event
- `Close()` — flushes and closes log file
- Output format: `[level] [event] key=value key=value`

## Memory Specification

### MemoryStore
- `MemoryEntry` — Key, Label, Value, Source, Timestamp
- `MemoryStore` — mu sync.Mutex, dir string, store map[string]string, loaded bool
- `NewMemoryStore(dir)` — creates store, loads data if exists
- `load()` — reads data from disk
- Persists as JSON in `DataDir`

## Skills Specification

### Skill
- `Skill` — Name, Description, Steps
- Markdown front-matter parsing (`---` separated)

### Skills
- `NewSkills(dir)` — creates skills store
- `load()` — loads skills from directory
- `parseSkill()`, `parseSkillBody()`, `parseFront()` — front-matter parsing

## Sub-Agent Specification

### SubAgent
- `SubAgent` struct — id, parent, task, allowedTools
- `NewSubAgent(id, parent, task, allowedTools)` — creates sub-agent
- `Run(ctx)` — isolated agent loop with narrow tool allow-list and child harness

## Guardrail Controller Specification

### GuardrailController
- `NewGuardrailController(cfg *GuardrailConfig)` — creates controller
- `resetForTurn()` — clears per-turn state
- `beforeCall(toolName, args)` — inspects proposed tool call, returns `(Decision, string)`
- `afterCall(toolName, args, result, failed)` — records outcome, returns decision
- `halt()` — returns controlled halt decision
- `lastCode()`, `lastMsg()` — expose last guardrail code/message

### GuardrailConfig
- `WeaknessEnabled` — warnings always surface; never block on their own
- `HardStopEnabled` — hard stops enabled
- `ExactFailureBlockAfter` — consecutive identical failing calls that hard-stop
- `SameToolFailureHaltAfter` — same tool failing N times this turn halts
- `NoProgressBlockAfter` — idempotent read-only tool returning the same result N times blocks
- `IdenticalCallThreshold` — consecutive identical (tool,args,result) calls before stall notice
- `LoopCaps` — maxWebSearches, maxSubagents

## Provider Selection Specification

### Provider
- `Provider` enum (built-in + extended):
  - `openai`, `anthropic`, `gemini`, `xai`, `ollama`, `openrouter`
  - `commandcode`, `commandcode-anthropic`, `kilocode`, `opencode-zen`, `opencode-go`, `ollama-cloud`
- `IsOpenAICompatible()` — true for `openai`, `xai`, `openrouter`, `commandcode`, `commandcode-anthropic`, `opencode-zen`, `opencode-go`, `ollama-cloud`

### Models
- `Models` registry grouped by provider
- `DefaultModels()` — built-in catalog (6 built-in + extended providers)
- `List(provider)` — models for a provider
- `All()` — all models across providers
- `Find(provider, modelID)` — model by (provider, modelID)
- `Selected(provider, modelID)` — specific model, or first for provider

### Model
- `Model` struct — ID, Name, Provider, IsLocal
- `String()` — `"provider/model"`

---
