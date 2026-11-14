// Package bh is BH BadHarness v2: a multi-agent agent harness.
//
// Design
//
//   - The model's own safety guardrails stay ACTIVE and unmodified at all
//     times. Nothing in this package rewrites, weakens, or disables them.
//   - Guardrails are a separate layer on top of the model (GuardrailController),
//     with allow | warn | block | halt verdicts. The harness decides how each
//     verdict is surfaced.
//   - Multi-agent delegation: sub-agents run isolated, with their own context
//     and a narrow tool allow-list, spawned on demand.
//   - Persistent memory and a scripted skills library so the harness re-uses
//     procedures across turns.
//   - Sandboxed tool execution + per-tool allow-list + per-turn caps.
package bh

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Role is a chat message role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Role constants for convenience.
const (
	RoleSystemStr    = "system"
	RoleUserStr      = "user"
	RoleAssistantStr = "assistant"
	RoleToolStr      = "tool"
)

// Message is a single chat message.
type Message struct {
	Role      Role
	Content   string
	ToolCalls []ToolCall
}

// ToolCall is a requested tool invocation from the model.
type ToolCall struct {
	ID        string
	ToolName  string
	Arguments map[string]any
}

// Plan is the harness's evolving plan of tool calls.
type Plan struct {
	ToolCalls []ToolCall
}

// FinishReason indicates why the model stopped generating.
type FinishReason string

const (
	FinishStop      FinishReason = "stop"
	FinishToolUse   FinishReason = "tool_use"
	FinishMaxTokens FinishReason = "max_tokens"
)

// reasoningHint is the standing prompt that teaches the model how to respond
// to requests that are ambiguous but plausibly safe: ask for scope rather than
// refusing, and propose safe alternatives. It is appended to the model's own
// guardrail instructions and never overwrites them.
const reasoningHint = "If a request is ambiguous but plausibly safe, ask for the missing scope or intent instead of refusing. Whenever a request touches something risky but permissible, break it into safe steps and suggest a narrower, safer alternative. Do not refuse simply because the request is underspecified."

// LLMResponse is the model's response.
type LLMResponse struct {
	Content string
	Tools   []ToolCall
	Finish  FinishReason
}

// Tool is a callable function registered with the harness.
type Tool struct {
	Name        string
	Description string
	Execute     func(ctx context.Context, args map[string]any) (any, error)
}

// LLM is the interface the harness uses to obtain model completions. Any
// provider (OpenAI, Anthropic, Ollama, local) can be plugged in; the harness
// never alters the model's guardrails.
type LLM interface {
	Complete(ctx context.Context, req LLMRequest) (LLMResponse, error)
}

// LLMRequest is the input to the model.
type LLMRequest struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolCall
}

// ToolRegistry holds tools and an allow-list. The harness layer enforces the
// allow-list; nothing in the model's path touches it.
type ToolRegistry struct {
	registered map[string]Tool
	allowList  map[string]struct{}
}

// NewToolRegistry creates an empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		registered: map[string]Tool{},
		allowList:  map[string]struct{}{},
	}
}

// Register adds a tool. Returns an error if already registered.
func (r *ToolRegistry) Register(t Tool) error {
	if _, exists := r.registered[t.Name]; exists {
		return fmt.Errorf("tool %q already registered", t.Name)
	}
	r.registered[t.Name] = t
	return nil
}

// Execute runs an approved tool. Returns an error if not allow-listed or not
// registered.
func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]any) (any, error) {
	t, ok := r.registered[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not registered", name)
	}
	if len(r.allowList) > 0 {
		if _, allowed := r.allowList[name]; !allowed {
			return nil, fmt.Errorf("tool %q not allow-listed", name)
		}
	}
	return t.Execute(ctx, args)
}

// Allow adds a tool to the allow-list.
func (r *ToolRegistry) Allow(name string) {
	if r.allowList == nil {
		r.allowList = map[string]struct{}{}
	}
	r.allowList[name] = struct{}{}
}

// Block removes a tool from the allow-list.
func (r *ToolRegistry) Block(name string) {
	if r.allowList == nil {
		return
	}
	delete(r.allowList, name)
}

// Names returns the names of all registered tools.
func (r *ToolRegistry) Names() []string {
	if r.registered == nil {
		return nil
	}
	names := make([]string, 0, len(r.registered))
	for n := range r.registered {
		names = append(names, n)
	}
	return names
}

// Count returns the number of registered tools.
func (r *ToolRegistry) Count() int {
	if r.registered == nil {
		return 0
	}
	return len(r.registered)
}

// Registered returns the tool by name, or nil if absent.
func (r *ToolRegistry) Registered(name string) Tool {
	if r.registered == nil {
		return Tool{}
	}
	return r.registered[name]
}

// AuditLogger records events to a log file (plus console). Auditing is optional
// and never affects the model's guardrails.
type AuditLogger struct {
	mu   sync.Mutex
	file *os.File
}

// NewAuditLogger creates a logger. If BH_LOG is set, output goes to that file
// in addition to the console; otherwise only the console is used.
func NewAuditLogger() *AuditLogger {
	l := &AuditLogger{}
	if path, ok := os.LookupEnv("BH_LOG"); ok && path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			l.file = f
		}
	}
	return l
}

// Info logs an informational event.
func (l *AuditLogger) Info(event string, fields ...string) {
	l.emit("info", event, fields)
}

// Warn logs a warning event.
func (l *AuditLogger) Warn(event string, fields ...string) {
	l.emit("warn", event, fields)
}

func (l *AuditLogger) emit(level, event string, fields []string) {
	var parts []string
	parts = append(parts, "["+level+"] ["+event+"]")
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			parts = append(parts, fields[i]+"="+fields[i+1])
		}
	}
	line := strings.Join(parts, " ")
	print(line)
	if l.file != nil {
		l.mu.Lock()
		l.file.WriteString(line + "\n")
		l.file.Sync()
		l.mu.Unlock()
	}
}

// Close flushes and closes the log file if open.
func (l *AuditLogger) Close() error {
	if l.file != nil {
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.file.Close()
	}
	return nil
}

// HarnessConfig configures the agent harness.
type HarnessConfig struct {
	Model         string
	SafetyPosture SafetyPosture
	MaxIterations int

	DataDir       string // where persistent memory/session files live
	SkillsDir     string // directory with scripted skill files
	ToolRegistry  *ToolRegistry
	Logger        *AuditLogger
	ReasoningHint string
	MaxSubagents  int // per-turn delegation cap

	// Model selection (opencode/kilocode-style): pick a provider + model pair.
	Models        *Models // model catalog; nil = use built-in DefaultModels
	Provider      Provider
	SelectedModel string
	// BaseURL is an optional private-gateway base URL override for the provider.
	// If non-empty, all model requests go through this custom endpoint (hermes
	// / opencode-style private gateway mode). Empty = use the provider's
	// default endpoint (or BH_API_KEY env var override).
	BaseURL string
}

// SafetyPosture controls how aggressively the harness's own guardrail layer
// intervenes. It does NOT touch the model's guardrails.
type SafetyPosture string

const (
	PostureConservative SafetyPosture = "conservative"
	PostureBalanced     SafetyPosture = "balanced" // default
	PostureAgile        SafetyPosture = "agile"
)

// Result is the final output of a single user request through the harness.
type Result struct {
	Step     int
	Content  string
	Plan     Plan
	Decision string // the outcome: completed / blocked / clarified / halted / maxIterations
}

// Harness is the agent harness: it runs a user request through an agent loop,
// enforces harness-level guardrails (allow|warn|block|halt) on each tool call,
// executes allow-listed tools, and can spawn isolated sub-agents. The model's
// own guardrails stay fully active inside the model and are never touched here.
type Harness struct {
	config       *HarnessConfig
	llm          LLM
	memory       *MemoryStore
	skills       *Skills
	toolRegistry *ToolRegistry
}

// New constructs a Harness. The config's model, provider, guardrail policy and
// tools are all passed through; the model's own guardrails are never modified.
func New(cfg *HarnessConfig, llm LLM) *Harness {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.Logger == nil {
		cfg.Logger = NewAuditLogger()
	}
	if cfg.ToolRegistry == nil {
		cfg.ToolRegistry = NewToolRegistry()
	}
	if cfg.ReasoningHint == "" {
		cfg.ReasoningHint = reasoningHint
	}
	if cfg.Models == nil {
		cfg.Models = DefaultModels()
	}
	if cfg.Provider == "" {
		cfg.Provider = ProviderOpenAI
	}
	if cfg.SelectedModel == "" {
		if md, ok := cfg.Models.Selected(cfg.Provider, ""); ok {
			cfg.SelectedModel = md.ID
		}
	}
	h := &Harness{config: cfg, llm: llm}
	h.memory = NewMemoryStore(cfg.DataDir)
	h.skills = NewSkills(cfg.SkillsDir)
	h.config.Logger.Info("harness initialized",
		"posture", string(cfg.SafetyPosture),
		"provider", string(cfg.Provider),
		"model", cfg.SelectedModel,
		"tools", strconv.Itoa(cfg.ToolRegistry.Count()),
	)
	return h
}

// Config returns the harness configuration.
func (h *Harness) Config() *HarnessConfig {
	return h.config
}

// ToolRegistry returns the harness tool registry.
func (h *Harness) ToolRegistry() *ToolRegistry {
	return h.config.ToolRegistry
}
