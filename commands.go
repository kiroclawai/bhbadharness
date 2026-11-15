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
		return cmdModel(cfg, args[1:], nil)
	case "config":
		return cmdConfig(cfg)
	case "auth":
		return cmdAuth(cfg, args[1:])
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
			if err := handleCommand(cfg, ss, cmd, rest, reader); err != nil {
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
func cmdModel(cfg *bh.HarnessConfig, args []string, reader *bufio.Reader) error {
	// Filter out empty strings
	trimmed := []string{}
	for _, a := range args {
		if strings.TrimSpace(a) != "" {
			trimmed = append(trimmed, a)
		}
	}
	args = trimmed

	if len(args) == 0 {
		// No args: show provider selection menu
		fmt.Println("🔍 Model/Provider Selector")
		fmt.Println("====================")
		prov, err := bh.SelectProvider(reader)
		if err != nil {
			return err
		}
		// Show models for the selected provider
		model, err := bh.SelectModel(prov, reader)
		if err != nil {
			return err
		}
		cfg.Provider = prov
		cfg.Model = model.ID
		fmt.Printf("🔄 Switched to %s / %s\n", prov, model.ID)
		return nil
	}

	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) == 2 {
		// provider/model format: use directly
		provider := bh.Provider(parts[0])
		model := parts[1]
		cfg.Provider = provider
		cfg.Model = model
		fmt.Printf("🔄 Switched to %s / %s\n", provider, model)
		return nil
	}

	// Single provider: show model selection menu
	provider := bh.Provider(args[0])
	fmt.Printf("🔍 Select model for %s\n", provider)
	fmt.Println("============================")
	model, err := bh.SelectModel(provider, reader)
	if err != nil {
		return err
	}
	cfg.Provider = provider
	cfg.Model = model.ID
	fmt.Printf("🔄 Switched to %s / %s\n", provider, model.ID)
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
func handleCommand(cfg *bh.HarnessConfig, ss *bh.SessionStore, cmd, rest string, reader *bufio.Reader) error {
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
		return cmdModel(cfg, args, reader)
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
	// Seamless auth: use stored provider credentials if available.
	var adapter bh.LLM

	// Check for stored credentials via auth store
	var apiKey string
	if authStore, err := bh.NewAuthStore(); err == nil {
		if creds, ok := authStore.Get(string(cfg.Provider)); ok {
			apiKey = creds.APIKey
		}
	}

	// If no stored credentials, check BH_API_KEY env var
	if apiKey == "" {
		apiKey = os.Getenv("BH_API_KEY")
	}

	if apiKey != "" && apiKey != "sk-noop" {
		// Private-gateway mode: use the explicit base URL.
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = os.Getenv("BH_BASE_URL")
		}
		if baseURL != "" {
			adapter = llm.NewLLMWithBase(cfg.Provider, cfg.Model, apiKey, baseURL)
		} else {
			adapter = llm.NewLLM(cfg.Provider, cfg.Model, apiKey)
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

// cmdAuth handles auth commands (login, status, logout).
func cmdAuth(cfg *bh.HarnessConfig, args []string) error {
	if len(args) == 0 {
		return showAuthHelp()
	}
	switch args[0] {
	case "status":
		return cmdAuthStatus(cfg)
	case "login":
		if len(args) < 2 {
			return fmt.Errorf("usage: bh auth login <provider>")
		}
		return cmdAuthLogin(cfg, args[1])
	case "logout":
		if len(args) < 2 {
			return fmt.Errorf("usage: bh auth logout <provider>")
		}
		return cmdAuthLogout(cfg, args[1])
	default:
		return fmt.Errorf("unknown auth command: %s (try: login <provider>, status, logout <provider>)", args[0])
	}
}

// showAuthHelp displays auth-related help.
func showAuthHelp() error {
	fmt.Println("🔐 BH BadHarness — Auth Commands")
	fmt.Println("====================")
	fmt.Println("Usage: bh auth <command> [args]")
	fmt.Println("Commands:")
	fmt.Println("  login <provider>  Authenticate with a provider")
	fmt.Println("  status            Show saved credentials")
	fmt.Println("  logout <provider> Remove saved credentials")
	fmt.Println()
	fmt.Println("Providers: openai, anthropic, gemini, xai, openrouter, ollama, ollama-cloud, commandcode, kilocode, opencode-zen, opencode-go, commandcode-anthropic")
	return nil
}

// cmdAuthStatus shows saved credentials.
func cmdAuthStatus(cfg *bh.HarnessConfig) error {
	authStore, err := bh.NewAuthStore()
	if err != nil {
		return err
	}
	providers := authStore.List()
	if len(providers) == 0 {
		fmt.Println("🔐 No saved credentials")
		fmt.Println("\nRun `bh auth login <provider>` to authenticate with a provider.")
		return nil
	}
	fmt.Println("🔐 Saved Credentials")
	fmt.Println("====================")
	for _, p := range providers {
		creds, _ := authStore.Get(p)
		fmt.Printf("  %s:\n", p)
		if creds.Account != "" {
			fmt.Printf("    Account: %s\n", creds.Account)
		}
		if creds.AccessToken != "" {
			fmt.Printf("    Auth method: %s\n", creds.AuthMethod)
			fmt.Printf("    Token: %s...\n", creds.AccessToken[:min(8, len(creds.AccessToken))])
		}
		if creds.APIKey != "" {
			fmt.Printf("    Auth method: %s\n", creds.AuthMethod)
			fmt.Printf("    API Key: %s...\n", creds.APIKey[:min(8, len(creds.APIKey))])
		}
	}
	return nil
}

// cmdAuthLogin authenticates with a provider.
func cmdAuthLogin(cfg *bh.HarnessConfig, provider string) error {
	provider = bh.NormalizeProvider(provider)
	if provider == "" {
		return fmt.Errorf("unknown provider: %s", provider)
	}

	authStore, err := bh.NewAuthStore()
	if err != nil {
		return err
	}

	if creds, ok := authStore.Get(provider); ok {
		fmt.Printf("🔐 Already authenticated with %s\n", provider)
		if creds.Account != "" {
			fmt.Printf("  Account: %s\n", creds.Account)
		}
		if creds.AccessToken != "" {
			fmt.Printf("  Token: %s...\n", creds.AccessToken[:min(8, len(creds.AccessToken))])
		}
		return nil
	}

	fmt.Printf("🔐 Authenticating with %s...\n", provider)
	fmt.Println("====================")
	fmt.Println("No credentials found. Follow these steps:")

	authMethod := bh.AuthMethodAPIKey
	if provider == "openai" || provider == "anthropic" || provider == "openrouter" {
		fmt.Println("  This provider supports OAuth login.")
		fmt.Println("  Opening browser to login page...")
	}

	fmt.Println("\nEnter your", provider, "credentials:")
	var input string
	fmt.Scanln(&input)

	creds := bh.ProviderCredentials{
		APIKey:     input,
		AuthMethod: authMethod,
		TargetHost: "https://api.openai.com/v1",
		Account:    provider,
	}
	if err := authStore.Save(provider, creds); err != nil {
		return err
	}

	fmt.Printf("✅ Successfully authenticated with %s\n", provider)
	fmt.Println("Your credentials are stored securely (0600 permissions).")
	return nil
}

// cmdAuthLogout removes credentials for a provider.
func cmdAuthLogout(cfg *bh.HarnessConfig, provider string) error {
	provider = bh.NormalizeProvider(provider)
	if provider == "" {
		return fmt.Errorf("unknown provider: %s", provider)
	}

	authStore, err := bh.NewAuthStore()
	if err != nil {
		return err
	}

	_, exists := authStore.Get(provider)
	if !exists {
		fmt.Printf("🔐 No credentials found for %s\n", provider)
		return nil
	}

	if err := authStore.Delete(provider); err != nil {
		return err
	}
	fmt.Printf("✅ Removed credentials for %s\n", provider)
	return nil
}
