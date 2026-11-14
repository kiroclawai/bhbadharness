// Package llm provides the concrete LLM adapters: an OpenAI-compatible HTTP
// client and a deterministic in-memory mock for testing. The shared interface
// and request/response types live in the harness package (internal/harness),
// so every adapter implements the same harness.LLM contract.
package llm
