// Command bhbadharness is the CLI entry point for BH BadHarness v2, an
// agent harness that wraps an LLM and runs requests through an agent loop with
// a separate guardrail layer, sub-agent delegation, memory, and skills. The
// model's own safety guardrails stay fully active and are never modified.
package main

import (
	"flag"
	"fmt"
	"os"

	"bhbadharness/internal/bh"
)

func main() {
	cfg := bh.DefaultConfig()
	var postureStr string
	var providerStr string
	flag.StringVar(&cfg.Model, "model", os.Getenv("BH_MODEL"), "model id passed through to the LLM")
	flag.StringVar(&postureStr, "posture", string(cfg.SafetyPosture), "harness guardrail posture")
	flag.StringVar(&cfg.ReasoningHint, "clarify-hint", cfg.ReasoningHint, "standing instruction appended to the prompt")
	flag.StringVar(&providerStr, "provider", "", "provider to select (openai, anthropic, gemini, xai, ollama, openrouter)")
	flag.StringVar(&cfg.BaseURL, "gateway", "", "private gateway base URL override (private gateway mode)")
	flag.Parse()

	// SafetyPosture is a custom type; convert from the string flag value.
	posture := bh.SafetyPosture(postureStr)
	switch posture {
	case bh.PostureConservative, bh.PostureBalanced, bh.PostureAgile:
	default:
		posture = bh.PostureBalanced
	}
	cfg.SafetyPosture = posture

	// Create a persistent session store for session management.
	sessionStore := bh.NewSessionStore(cfg.DataDir)

	if err := runCommand(cfg, flag.Args(), sessionStore); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
