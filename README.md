# BH BadHarness

A configurable agent harness written in Go. It wraps any language model and
runs requests through an agent loop, while keeping the model's **own safety
guardrails active and unmodified at all times**.

## The two-layer design

BH BadHarness separates the model's guardrails from the harness's own
guardrails:

1. **Model guardrails (never touched).** The harness never disables, weakens,
   or rewrites the model's system prompt or moderation. The model's own
   instructions and safety filtering remain fully active inside the model.
   The harness passes the model's request through unchanged.

2. **Harness guardrails (its own layer).** On top of the model, the harness
   adds its own configurable guardrail layer:
   - a **policy layer** that inspects each request and proposed tool call and
     returns a verdict (`approved`, `clarify`, or `block`);
   - a **tool allow-list** enforced at the harness layer (registered tools are
     free only when allow-listed);
   - a **sandboxed tool execution** layer (each tool is a registered function;
     the harness only runs allow-listed tools);
   - an **audit logger** that records every request, policy verdict, model
     turn, tool call, and result.

## "Rarely refuses" strategy (without disabling guardrails)

BH BadHarness is designed to be genuinely helpful and to rarely refuse
legitimate requests, using four mechanisms that do **not** touch the model's
own guardrails:

- **Clarify instead of refuse.** When a request is ambiguous but plausibly
  safe, the harness asks for missing scope or intent instead of refusing. This
  happens both in the harness policy layer and in the model's own reasoning
  instructions.
- **Staged delegation.** Risky requests are broken into safe steps with
  narrower, safer alternatives suggested at each step.
- **Safe alternatives embedded in the prompt.** A standing prompt appended to
  the model's own system prompt teaches it to propose safer, read-only
  approaches before declining.
- **Sub-agent scoping.** Each stage runs in isolation, so a risky task is
  split into small, well-scoped chunks rather than one broad permission.

The model's final "can I do this" decision still comes from the model's own
guardrails, which are never disabled.

## Configuration

The harness is configured with `HarnessConfig` and a `SafetyPosture`:

| Posture | Harness-layer behavior |
|---|---|
| `conservative` | Blocks shell/execute/run_command tools; flags sensitive keywords (delete, wipe, …) |
| `balanced` (default) | Clarifies ambiguous-but-safe requests; otherwise lets benign requests through |
| `agile` | Permissive at the harness layer; the model's guardrails do the heavy lifting |

```go
cfg := harness.DefaultConfig()
cfg.SafetyPosture = harness.PostureBalanced
cfg.Model = "gpt-4o"
cfg.ToolRegistry.Allow("read_file")
cfg.ToolRegistry.Allow("web_search")
h := harness.New(cfg, myLLMAdapter)
result, err := h.Run(context.Background(), "what is a goroutine?")
```

## The LLM interface

Any provider can be plugged in by implementing `harness.LLM`:

```go
type LLM interface {
    Complete(ctx context.Context, req LLMRequest) (LLMResponse, error)
}
```

The repository includes a deterministic in-memory **Mock** (for testing, no
API key needed) and an **OpenAI-compatible HTTP adapter** (for production
with your own API key, via the `BH_API_KEY` environment variable).

## Tool registry and sandboxing

Tools are registered and enforce an explicit allow-list:

```go
registry := cfg.ToolRegistry
registry.Register(harness.Tool{
    Name:        "read_file",
    Description: "Read a text file and return its contents.",
    Execute: func(ctx context.Context, args map[string]any) (any, error) {
        data, _ := os.ReadFile(args["path"].(string))
        return string(data), nil
    },
})
registry.Allow("read_file") // only allow-listed tools run
```

The harness only executes allow-listed tools, and each tool runs through the
registry's sandbox via `ToolRegistry.Execute`.

## Audibility

Set `BH_LOG` to write an audit log alongside console output:

```sh
export BH_LOG=/tmp/bh.log
```

Every request, policy verdict, model turn, and tool execution is recorded.

## Running

```sh
go build -o bhbadharness .
./bhbadharness "what is a goroutine?"
./bhbadharness "show me how to read a file in go"
```

No `BH_API_KEY` set? The harness runs with the in-memory mock so the full
stack can be exercised end-to-end.

## Tests

```sh
go test ./...
```

## Project layout

```
main.go                     CLI entry point
internal/harness/
    harness.go               Harness struct and agent loop
    types.go                 shared types (Message, ToolCall, LLM interface...)
    policy.go                harness guardrail policy layer
    tool.go                  tool registry + allow-listing
    logger.go                audit logger
internal/llm/
    llm.go                   package header
    openai.go                OpenAI-compatible HTTP adapter
    mock.go                  deterministic in-memory mock (no API key needed)
```
