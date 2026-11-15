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
|| `/exit` | Exit REPL |

### Seamless Login & Integration (New)

When you run `bh` without credentials, you get interactive setup:

| Command | Description |
|---------|-------------|
| `bh auth login <provider>` | Authenticate with a provider (OAuth or API key) |
| `bh auth status` | Show saved credentials for all providers |
| `bh auth logout <provider>` | Remove saved credentials |

**How seamless auth works:**
1. Select a provider + model in the REPL (`/model`)
2. The harness checks `~/.bhbadharness/credentials.json` for stored credentials
3. If found, the model is called with the stored key — no env var needed
4. If not found, the harness prompts `bh auth login <provider>` to authenticate
5. Credentials are stored with 0600 permissions (encrypted at rest)

**Auth flow (`bh auth login <provider>`):**
1. Opens provider's OAuth URL in browser
2. User completes login
3. OAuth code is exchanged for tokens
4. Tokens are stored securely and reused on subsequent requests

**Auth storage:** `~/.bhbadharness/credentials.json` (0600 permissions)

**Auth storage:** `~/.bhbadharness/credentials.json` (0600 permissions)

## Interactive REPL

When `bh` starts with no arguments, it enters an interactive REPL. Key commands:

| Command | Description |
|---------|-------------|
| `/help` | Show help |
| `/sessions` | List active sessions |
| `/new <title>` | Create a new session |
| `/resume <id\|index>` | Resume a session |
| `/model` | Select provider + model (interactive menu) |
| `/config` | Show configuration |
| `/exit` | Exit REPL |

### Interactive Provider/Model Selection

`/model` in the REPL shows a numbered provider picker:

```
🔍 Select provider
=================
 1. Anthropic
 2. CommandCode
 ...
q. Quit
```

Then shows a numbered model picker for the selected provider.

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

|- Tool allow-list: only registered + allow-listed tools run
|- `read_file` and `web_search` registered by default
|- `GuardrailController` checks each tool call (allow/warn/block/halt)
|- All credentials redacted (API keys shown as `***` in logs)
|- `ToolSandbox` with `SandboxConfig`: timeout, memory, CPU limits
|- `SafeArgs` validates arguments (no password/token/api_key keys)

## Sandbox Features

- `SandboxMode` (Off, Dir, User, Full): configurable sandboxing depth
- `ToolLimits`: timeout (30s), max memory (256MB), max CPU (60s), CPU shares
- `DefaultSandbox()`: conservative sandbox with allowed dirs/hosts
- `SandboxFor(posture)`: posture-aware sandbox config
  - `conservative`: full sandbox, restricted hosts, read-only system paths
  - `balanced`: dir sandbox with common allowed hosts
  - `agile`: permissive sandbox with more allowed hosts
- `SafeArgs()` rejects forbidden keys (password, secret, token, key, credential, auth)
- `SafeArgs()` also rejects credential patterns in values (sk-, ghp_, xoxb-, AIza)

## CLI

|- Flags: `--model`/`BH_MODEL`, `--posture` (conservative|balanced|agile), `--clarify-hint`, `--log`
|- Falls back to in-memory mock when `BH_API_KEY` unset (end-to-end runs work without network/credentials)

## Test Coverage

|- 6 tests passing:
|  - `internal/harness` suite (0.003s)
|  - `internal/llm` suite (mock, selector tests, TestNewLLM)
|- `TestSafeArgs` validates forbidden keys and credential patterns
