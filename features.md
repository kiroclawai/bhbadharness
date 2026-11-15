# BH BadHarness v2 — Features

## Guardrail Layer (Two-Layer Design)

- **Model guardrails untouched** — the model's refusal decision stays fully intact. The harness never disables, weakens, or rewrites model guardrails.
- **Harness guardrail layer** — separate policy layer on top of the model:
  - `Decision` verdicts: `allow`, `warn`, `block`, `halt`
  - `GuardrailController` tracks per-signature observation, fail streaks, stall-loop detection, no-progress detection, per-turn caps
  - `SafetyPosture` configurable: `conservative` (blocks `shell`, `execute`, `run_command`), `balanced` (default), `agile`
  - Sensitive pattern matching (delete, drop, wipe, destroy, exfiltrate)

## Provider & Model Selection (opencode/kilocode-style)

- `Provider` enum (built-in + extended):
  - `openai`, `anthropic`, `gemini`, `xai`, `ollama`, `openrouter`
  - `commandcode`, `commandcode-anthropic`, `kilocode`, `opencode-zen`, `opencode-go`, `ollama-cloud`
- `Models` registry with `Provider` enum, `Model` struct (id, name, provider, isLocal)
- `ProviderDef` definition: id, name, transport, API key env vars, base URL, isAggregator, auth type
- `ModelSelector` + `ModelRegistry` for catalog and per-provider normalization
- `ProviderRegistry` for global + per-scope provider map
- `normalizeModelForProvider()` per-provider model name normalization:
  - Aggregators (OpenRouter, Nous, AI Gateway, Kilocode): prepend vendor prefix (`openai/gpt-4o`, `anthropic/claude-sonnet-4`)
  - Dot-to-hyphen (anthropic): `claude-sonnet-4.6` → `claude-sonnet-4-6`
  - Matching-prefix-strip (xai): strip `x-ai/grok-2` → `grok-2`
  - Authoritative-native (huggingface): pass through unchanged
  - Deepseek retired aliases: `deepseek-chat` → `deepseek-flash`
  - Custom/unknown: pass through as-is
- `Default()` mock LLM when no API key (end-to-end runs work without network/credentials)

## Extended Provider Families (hermes/opencode/kilocode/commandcode parity)

- `commandcode`: OpenAI-compatible, https://api.commandcode.ai/provider/v1 (env: COMMANDCODE_BASE_URL)
- `commandcode-anthropic`: Anthropic-compatible, https://api.commandcode.ai/provider/v1 (env: COMMANDCODE_ANTHROPIC_BASE_URL)
- `kilocode`: https://api.kilo.ai/api/gateway (env: KILOCODE_BASE_URL)
- `opencode-zen`: https://opencode.ai/zen/v1 (env: OPENCODE_ZEN_BASE_URL)
- `opencode-go`: https://opencode.ai/zen/go/v1 (env: OPENCODE_GO_BASE_URL)
- `ollama-cloud`: https://ollama.com/v1 (env: OLLAMA_CLOUD_BASE_URL)

## Backend Routing

- `NewLLM(provider, modelID, apiKey)` routes to:
  - Anthropic: `https://api.anthropic.com/v1/messages` (x-api-key header)
  - Gemini: `https://generativelanguage.googleapis.com/v1beta/openai/...`
  - Ollama: `http://localhost:11434/api/chat`
  - OpenAI-compatible: `https://api.openai.com/v1`, `https://api.x.ai/v1`, `https://openrouter.ai/api/v1`
- `NewLLMWithBase(provider, modelID, apiKey, baseURL)` — private gateway mode via custom endpoint
- `GetBaseURL(provider, apiKeyEnvVar)` — resolves effective base URL (env override first, then hardcoded default)
- `Client` implements OpenAI-compatible `/chat/completions` with `Authorization: Bearer ***`

## Guardrail Controller

`GuardrailController` with:
- `Decision` verdicts: `allow`, `warn`, `block`, `halt`
- `Signature` per tool name + canonical args hash
- `noProgressEntry` track idempotent read-only results
- `exactFailures` map: `Signature → int` (consecutive identical failing calls)
- `sameToolFailures` map: `string → int` (same tool, different args)
- `checkStall()`: consecutive identical (tool,args,result) streak
- `applyStallDetectors()`: stall detection when repeated identical read-only tool call
- Loop caps: web_search and subagent per-turn limits

## CLI (hermes/opencode/claude-code parity)

### Commands

| Command | Description |
|---------|-------------|
| `bh [request]` | Run a request (default); REPL when no args |
| `bh help` | Show help / command palette |
| `bh sessions` | List active sessions (pagination + search) |
| `bh session <id|index>` | Show session detail with message history |
| `bh new <title>` | Create a new session |
| `bh resume <id|index>` | Resume a session |
| `bh model <provider/model>` | Switch provider + model |
| `bh config` | Show current configuration |

### Flag Reference

| Flag | Env Var | Type | Default | Description |
|------|---------|------|---------|-------------|
| `--model` / `BH_MODEL` | — | string | `gpt-4o` | Model id (format: `provider/model`) |
| `--posture` | — | string | `balanced` | Harness guardrail posture: `conservative`, `balanced`, `agile` |
| `--clarify-hint` | — | string | (built-in) | Standing instruction appended to the prompt |
| `--log` / `BH_LOG` | — | string | (none) | Audit log file path |
| `--provider` | — | string | — | Override provider for private gateway mode |
| `--gateway` / `BH_BASE_URL` | — | string | — | Private gateway base URL override |

### Per-Provider Env Vars

| Provider | Env Var | Description |
|----------|---------|-------------|
| `openai` | `OPENAI_BASE_URL` | Custom base URL |
| `anthropic` | `ANTHROPIC_BASE_URL` | Custom base URL |
| `gemini` | `GEMINI_BASE_URL` | Custom base URL |
| `xai` | `XAI_BASE_URL` | Custom base URL |
| `openrouter` | `OPENROUTER_BASE_URL` | Custom base URL |
| `ollama` | `OLLAMA_BASE_URL` | Custom base URL |
| `commandcode` | `COMMANDCODE_BASE_URL` | Custom base URL |
| `kilocode` | `KILOCODE_BASE_URL` | Custom base URL |
| `opencode-zen` | `OPENCODE_ZEN_BASE_URL` | Custom base URL |
| `opencode-go` | `OPENCODE_GO_BASE_URL` | Custom base URL |
| `ollama-cloud` | `OLLAMA_CLOUD_BASE_URL` | Custom base URL |
| `bh_provider_<name>` | `BH_PROVIDER_<NAME>_BASE_URL` | User-configured gateway override |

### Interactive REPL

When invoked with no arguments, `bh` enters an **interactive REPL** (mirrors hermes `session_history.py` + opencode TUI):

```
🔍 BH BadHarness v2 — interactive mode
Type /help for commands, /exit to quit.
Repl> 
```

REPL commands:
- `/help` — Show help
- `/sessions` — List active sessions
- `/new <title>` — Create a new session
- `/resume <id|index>` — Resume a session
- `/model <provider/model>` — Switch provider + model
- `/config` — Show configuration
- `/exit` — Exit REPL

REPL keybindings:
- `Ctrl+L` — Redraw prompt (clear terminal drift)
- `Ctrl+C` — Interrupt current request
- `Ctrl+D` — Exit REPL

### Status Bar

```
[status] posture=balanced provider=openai model=gpt-4o | help | ctrl+c
```

### Rich Output

- **ANSI color verdicts**: `allow` (green), `warn` (yellow), `block` (red), `halt` (magenta)
- **Session history pagination**: long histories scroll with `-- More --`
- **Session detail view**: conversation history with message roles and tool calls

## Memory

- `MemoryStore` with `MemoryEntry` (Key, Label, Value, Source, Timestamp)
- One JSON file per key in `DataDir`
- `load()` and `persist()` for JSON persistence
- Stores user preferences, known facts, prior context across turns

## Skills

- `Skill` (Name, Description, Steps)
- `Skills` struct with `NewSkills(dir)`, `load()`, `parseSkill()`, `parseSkillBody()`, `parseFront()`
- Scripted skills library with markdown front-matter (`---` separated)

## Sub-Agent Delegation

- `SubAgent` struct with `NewSubAgent(id, parent, task, allowedTools)`
- `Run(ctx)` executes isolated agent loop with narrow tool allow-list and child harness
- `buildChildHarness(parent, allowedTools)` creates isolated child harness
- `MaxSubagents: 4` default cap

## Audit Logging

- `AuditLogger` writes to log file (if `BH_LOG` set) and console
- `Info()`, `Warn()` events with fields
- `Close()` to flush and close file

## Session Store (Persistence)

- `SessionStore` with `Session` struct (ID, Title, CreatedAt, UpdatedAt, Messages)
- One JSON file per session in `DataDir`
- `Create()`, `Get()`, `List()`, `Update()`, `Delete()` for session lifecycle

## Security & Sandboxing

- Tool allow-list: only registered + allow-listed tools run
- `read_file` and `web_search` registered by default
- `GuardrailController` checks each tool call (allow/warn/block/halt)
- All credentials redacted (API keys shown as `***` in logs)

## CLI

- Flags: `--model`/`BH_MODEL`, `--posture` (conservative|balanced|agile), `--clarify-hint`, `--log`
- Falls back to in-memory mock when `BH_API_KEY` unset (end-to-end runs work without network/credentials)

## Test Coverage

- 6 tests passing:
  - `internal/harness` suite (0.003s)
  - `internal/llm` suite (mock, selector tests, TestNewLLM)
