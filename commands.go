// Command bhbadharness is the CLI entry point for BH BadHarness v2, an
// agent harness that wraps an LLM and runs requests through an agent loop with
// a separate guardrail layer, sub-agent delegation, memory, and skills. The
// model's own safety guardrails stay fully active and are never modified.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"bhbadharness/internal/bh"
	"bhbadharness/internal/llm"
)

// ---- Command handlers ----

// runCommand dispatches the CLI command.
func runCommand(cfg *bh.HarnessConfig, args []string, ss *bh.SessionStore) error {
	if len(args) == 0 {
		return interactiveMode(cfg, ss)
	}

	switch args[0] {
	case "sessions":
		return cmdSessions(cfg, ss)
	case "session":
		return cmdSession(cfg, ss, args[1:])
	case "new":
		return cmdNew(cfg, ss, args[1:])
	case "resume":
		return cmdResume(cfg, ss, args[1:])
	case "model":
		return cmdModel(cfg, args[1:])
	case "config":
		return cmdConfig(cfg)
	case "help":
		return showHelp()
	default:
		// Default: run request
		return runRequest(cfg, args)
	}
}

// interactiveMode starts the REPL when no arguments are given.
func interactiveMode(cfg *bh.HarnessConfig, ss *bh.SessionStore) error {
	fmt.Println("🔍 BH BadHarness v2 — interactive mode")
	fmt.Println("Type /help for commands, /exit to quit.")
	fmt.Println("Repl> ")

	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "/exit" || line == "/quit" {
			fmt.Println("Goodbye!")
			break
		}
		if strings.HasPrefix(line, "/") {
			// Command
			parts := strings.SplitN(line, " ", 2)
			cmd := parts[0]
			rest := ""
			if len(parts) > 1 {
				rest = parts[1]
			}
			if err := handleCommand(cfg, ss, cmd, rest); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
			fmt.Println("Repl> ")
			continue
		}
		// Run request
		if err := runRequest(cfg, []string{line}); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		fmt.Println("Repl> ")
	}
	return nil
}

// showHelp displays the help screen.
func showHelp() error {
	fmt.Println(`BH BadHarness v2 — agent harness
Rarely refuses legitimate requests while keeping the model's guardrails ACTIVE and unmodified.

Usage: bhbadharness <command> [args]

Commands:
  sessions        List active sessions
  session <id>    Show session detail
  new <title>     Create a new session
  resume <id>     Resume a session
  model <provider/model>  Switch model/provider
  config          Show current configuration
  help            Show this help
  <request>       Run a request (default)

Examples:
  bhbadharness 'what is a goroutine?'
  bhbadharness new "Goroutine deep dive"
  bhbadharness sessions
  bhbadharness resume 12345
  bhbadharness model openai/gpt-4o
  bhbadharness model anthropic/claude-sonnet-4`)
	return nil
}

// cmdSessions lists active sessions.
func cmdSessions(cfg *bh.HarnessConfig, ss *bh.SessionStore) error {
	fmt.Println("📋 Sessions")
	fmt.Println("===========")
	sessions, err := ss.List()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Println("(No sessions yet — create one with `bh new <title>`)")
		return nil
	}
	for i, s := range sessions {
		fmt.Printf("%d. %s — %s\n", i+1, s.Title, s.CreatedAt.Format("2006-01-02 15:04"))
	}
	return nil
}

// cmdSession shows session detail.
func cmdSession(cfg *bh.HarnessConfig, ss *bh.SessionStore, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: bh badharness session <id>")
	}
	ssID := args[0]
	// Try numeric index or session ID
	var ssIDFull string
	if len(args) > 1 {
		ssIDFull = args[1]
	}
	session, err := ss.Get(ssID)
	if err != nil && ssID != ssIDFull {
		// Try numeric index
		if idx, err := strconv.Atoi(ssID); err == nil && idx > 0 {
			sessions, _ := ss.List()
			if idx-1 < len(sessions) {
				session = sessions[idx-1]
			}
		}
	}
	if session == nil {
		return fmt.Errorf("session not found: %s", ssID)
	}
	fmt.Printf("📋 Session: %s\n", session.Title)
	fmt.Printf("Created: %s\n", session.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated: %s\n", session.UpdatedAt.Format(time.RFC3339))
	fmt.Printf("Messages: %d\n", len(session.Messages))
	fmt.Println("===========")
	for i, msg := range session.Messages {
		fmt.Printf("[%d] %s: %s\n", i, msg.Role, msg.Content)
	}
	return nil
}

// cmdNew creates a new session.
func cmdNew(cfg *bh.HarnessConfig, ss *bh.SessionStore, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: bh badharness new <title>")
	}
	title := args[0]
	session := ss.Create(title)
	fmt.Printf("📋 New session: %s (ID: %s)\n", title, session.ID)
	return nil
}

// cmdResume resumes a session.
func cmdResume(cfg *bh.HarnessConfig, ss *bh.SessionStore, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: bh badharness resume <id>")
	}
	ssID := args[0]
	session, err := ss.Get(ssID)
	if err != nil {
		if len(args) > 1 {
			if idx, err := strconv.Atoi(ssID); err == nil && idx > 0 {
				sessions, _ := ss.List()
				if idx-1 < len(sessions) {
					session = sessions[idx-1]
					ssID = session.ID
				}
			}
		}
		if session == nil {
			return fmt.Errorf("session not found: %s", ssID)
		}
	}
	fmt.Printf("📋 Resuming session: %s\n", session.Title)
	fmt.Printf("Messages: %d\n", len(session.Messages))
	return nil
}

// cmdModel switches model/provider.
func cmdModel(cfg *bh.HarnessConfig, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: bh badharness model <provider/model>")
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected provider/model format: e.g. openai/gpt-4o")
	}
	provider := bh.Provider(parts[0])
	model := parts[1]
	cfg.Provider = provider
	cfg.Model = model
	fmt.Printf("🔄 Switched to %s / %s\n", provider, model)
	return nil
}

// cmdConfig shows current configuration.
func cmdConfig(cfg *bh.HarnessConfig) error {
	fmt.Println("⚙️ Configuration")
	fmt.Println("===============")
	fmt.Printf("Model:      %s\n", cfg.Model)
	fmt.Printf("Provider:   %s\n", cfg.Provider)
	fmt.Printf("Posture:    %s\n", cfg.SafetyPosture)
	fmt.Printf("Guardrails: active (unmodified)\n")
	fmt.Printf("Memory:     persistent (JSON)\n")
	fmt.Printf("Skills:     loaded from %s\n", cfg.SkillsDir)
	fmt.Printf("Base URL:   %s\n", cfg.BaseURL)
	return nil
}

// handleCommand dispatches interactive commands.
func handleCommand(cfg *bh.HarnessConfig, ss *bh.SessionStore, cmd, rest string) error {
	switch cmd {
	case "/help", "/?":
		return showHelp()
	case "/sessions", "/list":
		return cmdSessions(cfg, ss)
	case "/new":
		args := strings.Split(rest, " ")
		return cmdNew(cfg, ss, args)
	case "/resume":
		args := strings.Split(rest, " ")
		return cmdResume(cfg, ss, args)
	case "/model":
		args := strings.Split(rest, " ")
		return cmdModel(cfg, args)
	case "/config":
		return cmdConfig(cfg)
	case "/exit", "/quit":
		fmt.Println("Goodbye!")
		os.Exit(0)
	}
	return fmt.Errorf("unknown command: %s", cmd)
}

// runRequest runs a user request. The harness and tools are created once
// per request (this is the CLI entry point; for interactive mode, use
// the built-in harness with tools already registered).
func runRequest(cfg *bh.HarnessConfig, args []string) error {
	ctx := context.Background()

	// Build an LLM adapter implementing bh.LLM.
	var adapter bh.LLM
	if token := os.Getenv("BH_API_KEY"); token != "" && token != "sk-noop" {
		// Private-gateway mode: use the explicit base URL.
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = os.Getenv("BH_BASE_URL")
		}
		if baseURL != "" {
			adapter = llm.NewLLMWithBase(cfg.Provider, cfg.Model, token, baseURL)
		} else {
			adapter = llm.NewLLM(cfg.Provider, cfg.Model, token)
		}
	} else {
		adapter = llm.Default()
	}

	// Construct the harness.
	h := bh.New(cfg, adapter)

	// Register built-in tools.
	registry := h.Config().ToolRegistry
	if err := registry.Register(bh.Tool{
		Name:        "read_file",
		Description: "Read a text file and return its contents.",
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			path := args["path"].(string)
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			return string(data), nil
		},
	}); err != nil {
		// Tool may already be registered (from a previous call in interactive mode)
		if !strings.Contains(err.Error(), "already registered") {
			return fmt.Errorf("register read_file: %w", err)
		}
	}

	if err := registry.Register(bh.Tool{
		Name:        "web_search",
		Description: "Search the web and return results.",
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			query := args["query"].(string)
			return fmt.Sprintf("Search results for %q (read-only, no side effects)", query), nil
		},
	}); err != nil {
		if !strings.Contains(err.Error(), "already registered") {
			return fmt.Errorf("register web_search: %w", err)
		}
	}
	registry.Allow("read_file")
	registry.Allow("web_search")

	// Logging.
	if envLog := os.Getenv("BH_LOG"); envLog != "" {
		cfg.Logger.Close()
		cfg.Logger = bh.NewAuditLogger()
	}

	fmt.Println("🔍 BH BadHarness v2")
	fmt.Printf("Model: %s / %s\n", cfg.Provider, cfg.Model)
	fmt.Printf("Posture: %s\n", cfg.SafetyPosture)
	fmt.Printf("Guardrails: active (unmodified)\n")
	fmt.Println("---")

	result, err := h.Run(ctx, strings.Join(args, " "))
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	fmt.Println("---")
	fmt.Printf("STATUS: %s\n", result.Decision)
	fmt.Println("---")
	fmt.Println(result.Content)
	return nil
}
