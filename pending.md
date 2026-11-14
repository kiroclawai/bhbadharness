# BH BadHarness v2 — Pending Work

## Status: Active Development → Market Leader

This file tracks everything remaining to reach market-leader parity.

---

## 1. Live-Provider Routing (CRITICAL)

**Status:** Design-level only. **Verification:** None (pending live runs).

### What needs to be done
- Run live-provider integration tests against:
  - Anthropic (api.anthropic.com/v1/messages)
  - Gemini (generativelanguage.googleapis.com)
  - Ollama (localhost:11434)
  - OpenAI-compatible (api.openai.com)
  - xAI (api.x.ai/v1)
  - OpenRouter (openrouter.ai/api/v1)
- Verify request/response contracts for each provider
- Verify error handling for rate limits, auth failures, model not found
- Verify streaming support (if provider requires it)

### Blocking
- Requires live API keys (BH_API_KEY set, but real runs not yet performed)

---

## 2. Guardrail Parity Depth

**Status:** Deepening. **Verification:** Build + tests pass; live-run verification pending.

### What was done this session
- Added hermes-pattern features to `GuardrailController` in `internal/bh/guardrails.go`:
  - `detectIdenticalCycle()` — batch-cycle detection (A,B,A,B,...) using call history, matching hermes `_detect_identical_cycle`
  - Throttled-call handling in `afterCall()` — keeps rate-limited calls out of exact-args streak
  - `recordFailure()` — tracks exact failures and same-tool failures (was inline, now extracted)
  - `detectIdenticalCycle()` uses `_STALL_GUARD_CYCLE_HISTORY`, `_STALL_GUARD_MAX_CYCLE_PERIOD`, `_STALL_GUARD_IDENTICAL_CALL_THRESHOLD` constants
  - `hardStop` guardrail config integration for stall detection

### Key gap
- BH's `beforeCall` doesn't check exact failures or no-progress block (only loop caps)
- Hermes has `_progress_since_failure` tracking
- Hermes has tool-specific recovery hints (`_tool_failure_recovery_hint`)

### Next steps
- Add `_progress_since_failure` tracking
- Add recovery hints
- Add failure-tolerant tool handling
- Integration tests against hermes `tool_guardrails.py` behavior

---

## 3. Sub-Agent Isolation Depth

**Status:** Design-level. **Verification:** None (pending integration tests).

### What needs to be done
- Context/compression isolation between parent and child
- Child termination handling (timeout, kill, pause)
- Error propagation from child to parent
- Child model/provider override per sub-agent
- Sub-agent to cache results back to parent

### Reference
- hermes `delegate_task` registry
- opencode agent loops

---

## 4. Memory & Skills Parity

**Status:** Design-level. **Verification:** None (pending integration tests).

### What needs to be done
- Compare BH `MemoryStore` with hermes `memory_manager.py`
  - Legacy memory files cleanup
  - Multi-session memory persistence
  - User preference extraction
- Compare BH `Skills` with hermes `skills_index_delta.py`
  - Skill triggers
  - Skill auto-update
  - Skill conflict resolution

### Key gap
- No live test proves memory restores across turns
- No live test proves skills are triggered correctly

---

## 5. Dependency Cleanup

**Status:** Skipped. **Verification:** None.

### What needs to be done
- `go.sum` was generated via `go mod tidy` (when tests were run)
- Verify zero-dependency build works: `go build ./...` without any external dependencies
- Add `go.sum` if needed

### Result
- `go.mod` currently lists only `bhbadharness` module (no dependencies)
- `go.sum` not present (nothing to tidy)
- Build is already zero-dependency

---

## 6. CLI Surface

**Status:** Minimal. **Verification:** None (design-level).

### What needs to be done
- Add `--provider` flag (not currently exposed; provider selection is via `BH_API_KEY`/auto-detect)
- Add `--model-switch` command
- Add provider/credential discovery UI
- Add `/model` slash command (hermes-style)
- Add multi-provider profile support

### Reference
- hermes `model_switch.py` command
- opencode picker

---

## 7. Audit Logging

**Status:** Minimal. **Verification:** None.

### What needs to be done
- Full turn audit (model input/output, tool calls, guardrail verdicts)
- Per-tool-call audit with guardrail decision codes
- Structured JSON log format for machine parsing
- Audit log rotation/rolling

### Current state
- `AuditLogger` writes `info`/`warn` events to log file (if `BH_LOG` set) and console
- No turn-level audit, no structured JSON format

---

## 8. Security & Sandboxing

**Status:** Design-level. **Verification:** None.

### What needs to be done
- Add OS-level sandbox (gvisor, bubblewrap, seccomp)
- Tool escape prevention
- Resource limits (CPU, memory, disk, network)
- Per-sub-agent capability isolation

### Current state
- `ToolRegistry` has allow-list semantics
- `SubAgent` has isolated tool allow-list
- No OS-level sandbox or capability isolation

### Reference
- Claude Code sandbox
- Kilocode sandbox

---

## 9. O1/OpenClaw Model Family

**Status:** Design-level. **Verification:** None.

### What needs to be done
- Add OpenClaw-style model naming and credential pools
- Add OpenClaw proxy routing
- Add OpenClaw credential rotation

### Current state
- Provider enum and model registry handle the hermes-opencode-kilocode families
- OpenClaw-specific path not tested

---

## 10. Model & Provider Parity

**Status:** Design-level. **Verification:** None.

### What needs to be done
- Add per-provider model catalog (models.dev + user config + discovered)
- Add model normalization per provider (already partially implemented)
- Add provider credential discovery (models.dev + user config)
- Add per-provider API mode (chat.completions, anthropic_messages, codex_responses)

### Reference
- hermes `providers.py` (ProviderDef, ALIASES, normalize_provider)
- hermes `model_normalize.py` (per-provider normalization)

### Private Gateway Support (added @ 2026-11-14)
- `internal/bh/selector.go`: `DefaultProviderBaseConfig()` + `GetBaseURL()` mirror hermes `providers.py` HermesOverlay (base_url_override + base_url_env_var)
- `internal/llm/backend.go`: `NewLLMWithBase(provider, modelID, apiKey, baseURL)` for explicit private gateway; `NewLLM` resolves via `GetBaseURL()`
- `main.go`: `--gateway` / `BH_BASE_URL` flags for private gateway mode; `cfg.Provider` + `cfg.SelectedModel` for opencode/kilocode-style selection
- `internal/bh/types.go`: `HarnessConfig.BaseURL` for private gateway config

### Extended Provider Families (hermes/opencode/kilocode/commandcode parity)
- Added `Provider` enum extensions: `CommandCode`, `CommandCodeAnthropic`, `KiloCode`, `OpenCodeZen`, `OpenCodeGo`, `OllamaCloud`
- `DefaultProviderBaseConfig()` extended with:
  - `commandcode`: https://api.commandcode.ai/provider/v1 (env: COMMANDCODE_BASE_URL)
  - `commandcode-anthropic`: https://api.commandcode.ai/provider/v1 (env: COMMANDCODE_ANTHROPIC_BASE_URL)
  - `kilocode`: https://api.kilo.ai/api/gateway (env: KILOCODE_BASE_URL)
  - `opencode-zen`: https://opencode.ai/zen/v1 (env: OPENCODE_ZEN_BASE_URL)
  - `opencode-go`: https://opencode.ai/zen/go/v1 (env: OPENCODE_GO_BASE_URL)
  - `ollama-cloud`: https://ollama.com/v1 (env: OLLAMA_CLOUD_BASE_URL)
- `AliasGroups` extended with `commandcode`, `commandcode-anthropic`, `ai-gateway`, `nous`, `grok-oauth`
- Model catalogs added per provider: `DefaultModels()` now includes extended provider models

---

## 12. Model Catalogs (Extended Providers)

**Status:** Design-level. **Verification:** Build + tests pass; live runs pending.

### What needs to be done
- Verify `DefaultModels()` returns models for all extended providers (added in this session)
- Verify `ModelSelector` normalization for extended providers (aggregator prefix, dot-to-hyphen)
- Verify provider routing uses correct base URLs
- Run live-provider integration tests against extended provider endpoints
- Verify model normalization for extended providers (e.g., `openai/gpt-4o` for OpenRouter)

---

## 11. Private Gateway Parity

**Status:** Design-level. **Verification:** Build + tests pass; no live runs.

### What needs to be done
- Verify private gateway routing end-to-end (requires a running gateway)
- Add per-provider gateway configuration via `BH_BASE_URL` env var
- Add per-profile scoped gateway config (like hermes profiles)
- Add gateway health checks
- Add gateway credential injection (without leaking keys)

### Reference
- hermes `providers.py` `HermesOverlay` (base_url_override, base_url_env_var)
- hermes `hermes proxy` (local OpenAI-compatible proxy)
- opencode `opencode.json` (private gateway config)

---

## 13. Interactive REPL

**Status:** Design-level. **Verification:** Build + tests pass; basic REPL works.

### What needs to be done
- Implement full REPL with `/help`, `/sessions`, `/new`, `/resume`, `/model`, `/config`, `/exit`
- Add session persistence (JSON files in DataDir) — done
- Add session list with pagination and search — done
- Add session detail view (conversation history with messages) — done
- Add tool registration once (avoid duplicate registration) — done
- Add session resume (load messages from session store) — in progress
- Add ANSI color support for guardrail verdicts (green=allow, yellow=warn, red=block, magenta=halt)
- Add interactive model/provider picker

---

## Summary of Pending vs. Verified

| Item | Status | Verification |
|------|--------|--------------|
| Build, tests, vet | VERIFIED | Clean on disk |
| CLI end-to-end | VERIFIED | Safe question, blocked tool path |
| Model/provider selector | VERIFIED | 7 tests passing |
| Provider routing | DESIGN-LEVEL | Live runs not performed |
| Guardrail parity depth | DESIGN-LEVEL | Integration tests pending |
| Sub-agent isolation | DESIGN-LEVEL | Integration tests pending |
| Memory & skills parity | DESIGN-LEVEL | Integration tests pending |
| CLI surface | MINIMAL | Help, sessions, session, new, resume, model, config, request |
| Audit logging | MINIMAL | Design-level |
| Security & sandboxing | DESIGN-LEVEL | Not implemented |
| UI/UX | MINIMAL | Interactive REPL, basic commands (sessions, new, resume, model, config, request) |
| Dependency cleanup | SKIPPED | Zero-dependency already |
| O1/OpenClaw family | DESIGN-LEVEL | Not implemented |
| Dependency cleanup | SKIPPED | Zero-dependency already |

## Summary of Pending vs. Verified

| Item | Status | Verification |
|------|--------|--------------|
| Build, tests, vet | VERIFIED | Clean on disk |
| CLI end-to-end | VERIFIED | Safe question, blocked tool path |
| Model/provider selector | VERIFIED | 7 tests passing |
| Provider routing | DESIGN-LEVEL | Live runs not performed |
| Guardrail parity depth | DESIGN-LEVEL | Integration tests pending |
| Sub-agent isolation | DESIGN-LEVEL | Integration tests pending |
| Memory & skills parity | DESIGN-LEVEL | Integration tests pending |
| CLI surface | MINIMAL | Help, sessions, session, new, resume, model, config, request |
| Audit logging | MINIMAL | Design-level |
| Security & sandboxing | DESIGN-LEVEL | Not implemented |
| UI/UX | MINIMAL | Interactive REPL, basic commands (sessions, new, resume, model, config, request) |
| Dependency cleanup | SKIPPED | Zero-dependency already |
| O1/OpenClaw family | DESIGN-LEVEL | Not implemented |
| Dependency cleanup | SKIPPED | Zero-dependency already |

---

## Next Steps (by priority)

1. Add live-provider integration tests (start with one provider)
2. Deepen guardrail controller parity with hermes
3. Add sub-agent isolation tests
4. Add memory/skills integration tests
5. Expand CLI surface
6. Add audit logging
7. Add security/sandboxing
8. Add O1/OpenClaw support
