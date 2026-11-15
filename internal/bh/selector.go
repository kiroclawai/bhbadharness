// Package bh model/provider selector: pick a provider + model (opencode/kilocode-style selection).
package bh

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ---- Provider definition -------------------------------------------------

// ProviderDef is the complete provider definition: identity, transport, credentials,
// endpoint, and source. Merged from models.dev + user config.
type ProviderDef struct {
	ID            string   // canonical provider id (openai, anthropic, gemini, xai, ollama, openrouter, ...)
	Name          string   // human-readable label
	Transport     string   // openai_chat | anthropic_messages | codex_responses | bedrock_converse
	APIKeyEnvVars []string // all env vars to check for an API key
	BaseURL       string   // endpoint base URL
	BaseURLEnvVar string   // env var naming the base URL (nil/"" = no per-provider override)
	IsAggregator  bool     // /v1/models returns "vendor/model" slugs; forward them as-is
	AuthType      string   // "api_key" | "oauth" | "none"
	Doc           string   // human-readable description
	Source        string   // "models.dev" | "hermes" | "user-config"
}

// ProviderID returns the canonical id (lowercase, no spaces).
func (p ProviderDef) ProviderID() string {
	return strings.ToLower(strings.ReplaceAll(p.ID, " ", ""))
}

// ---- Model definition ---------------------------------------------------

// ModelDef is a named model with provider, metadata, and local flag.
type ModelDef struct {
	ID          string // provider-specific model id
	Name        string // display name
	Provider    Provider
	ContextLen  int  // context window in tokens; 0 = unknown
	IsLocal     bool // runs on-device (ollama, local GGUF)
	Description string
}

// ByProvider groups models by provider (canonical form).
func ByProvider(models []ModelDef) map[Provider][]ModelDef {
	out := map[Provider][]ModelDef{}
	for _, md := range models {
		out[md.Provider] = append(out[md.Provider], md)
	}
	return out
}

// ---- Provider registry ---------------------------------------------------------

// ProviderRegistry is a global + per-scope provider map with registration,
// per-profile scoped maps, and a lock. Mirrors agent.provider_registry.ProviderRegistry.
type ProviderRegistry struct {
	mu              sync.RWMutex
	providers       map[string]ProviderDef            // global map keyed by canonical id
	scopedProviders map[string]map[string]ProviderDef // scope -> map of provider id -> def
}

// NewProviderRegistry builds an empty registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers:       map[string]ProviderDef{},
		scopedProviders: map[string]map[string]ProviderDef{},
	}
}

// Register adds a provider. Same-name re-registration overwrites (hot reload).
func (r *ProviderRegistry) Register(provider ProviderDef) error {
	if provider.Name == "" {
		return fmt.Errorf("provider name must be non-empty")
	}
	key := strings.ToLower(strings.ReplaceAll(provider.ID, " ", ""))
	if key == "" {
		return fmt.Errorf("provider id must be non-empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[key] = provider
	return nil
}

// RegisterScoped adds a provider under a scope (e.g. a profile). Overwrites.
func (r *ProviderRegistry) RegisterScoped(scope string, provider ProviderDef) error {
	if provider.Name == "" {
		return fmt.Errorf("provider name must be non-empty")
	}
	key := strings.ToLower(strings.ReplaceAll(provider.ID, " ", ""))
	if key == "" {
		return fmt.Errorf("provider id must be non-empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.scopedProviders[scope] == nil {
		r.scopedProviders[scope] = map[string]ProviderDef{}
	}
	r.scopedProviders[scope][key] = provider
	return nil
}

// List returns all registered providers (global + scoped), sorted by name.
func (r *ProviderRegistry) List(scope ...string) []ProviderDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	merged := map[string]ProviderDef{}
	for _, p := range r.providers {
		merged[p.ProviderID()] = p
	}
	for _, s := range scope {
		if m, ok := r.scopedProviders[s]; ok {
			for k, p := range m {
				merged[k] = p
			}
		}
	}
	out := make([]ProviderDef, 0, len(merged))
	for _, p := range merged {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns the provider registered under name (canonical id or alias), or nil.
func (r *ProviderRegistry) Get(name string) (*ProviderDef, bool) {
	if name == "" {
		return nil, false
	}
	key := normalizeProvider(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.providers[key]; ok {
		return &p, true
	}
	// Scoped first, then global.
	for _, m := range r.scopedProviders {
		if p, ok := m[key]; ok {
			return &p, true
		}
	}
	return nil, false
}

// ---- Provider alias resolution --------------------------------------------------

// AliasGroups maps human-friendly / legacy provider names to canonical ids.
// Canonical ids are the values of the Provider enum (openai, anthropic, gemini,
// xai, ollama, openrouter) plus additional providers registerable via
// ProviderDef. Mirrors hermes_cli/providers.py _ALIAS_GROUPS + ALIASES.
var AliasGroups = map[string][]string{
	"openai":               {"openai"},
	"anthropic":            {"anthropic", "claude", "claude-code", "claude-oauth"},
	"xai":                  {"xai", "x-ai", "x.ai", "grok"},
	"openrouter":           {"openrouter", "openrouter-zen", "zen"},
	"kilocode":             {"kilocode", "kilo-code", "kilo-gateway", "kilo"},
	"zai":                  {"zai", "glm", "z-ai", "z.ai", "zhipu"},
	"deepseek":             {"deepseek", "deep-seek"},
	"alibaba":              {"alibaba", "dashscope", "aliyun", "qwen", "alibaba-cloud"},
	"huggingface":          {"huggingface", "hf", "hugging-face", "huggingface-hub"},
	"bedrock":              {"bedrock", "aws", "aws-bedrock", "amazon-bedrock", "amazon"},
	"nvidia":               {"nvidia", "nim", "nvidia-nim", "build-nvidia", "nemotron"},
	"lmstudio":             {"lmstudio", "lm-studio", "lm_studio"},
	"custom":               {"ollama", "local", "vllm", "llamacpp", "llama.cpp", "llama-cpp"},
	"ollama-cloud":         {"ollama-cloud"},
	"gemini":               {"gemini", "google", "google-gemini", "google-ai-studio"},
	"kimi-for-coding":      {"kimi-for-coding", "kimi-coding", "kimi-coding-cn", "kimi", "moonshot"},
	"stepfun":              {"stepfun", "step"},
	"minimax-cn":           {"minimax-cn", "minimax-china", "minimax_cn"},
	"fireworks":            {"fireworks", "fireworks-ai", "fw"},
	"upstage":              {"upstage", "solar"},
	"actual":               {"actual", "actual-computer", "actualcomputer", "aci"},
	"nebius-token-factory": {"nebius-token-factory", "nebius", "nebius-tokenfactory", "nebius-tf", "token-factory", "tokenfactory"},
	"tencent-tokenhub":     {"tencent-tokenhub", "tencent", "tokenhub", "tencent-cloud", "tencentmaas"},
	"tencent-tokenplan":    {"tencent-tokenplan", "tokenplan", "tencent-lkeap"},
	"gmi":                  {"gmi", "gmi-cloud", "gmicloud"},
	"meta":                 {"meta", "meta-ai"},
	"azure":                {"azure", "azure-foundry"},
	"opencode":             {"opencode-zen", "opencode", "opencode_zen", "zen"},
	"opencode-go":          {"opencode-go", "opencode_go", "go", "opencode-go-sub"},
	"openai-codex":         {"openai-codex", "chatgpt", "chatgpt-codex"},
	"ollama-oauth":         {"ollama-oauth", "ollama-oauth"},
	"custom-route":         {"custom-route", "custom-route"},
	// Extended providers (hermes-style)
	"commandcode":           {"commandcode", "commandcode-chat"},
	"commandcode-anthropic": {"commandcode-anthropic", "commandcode-claude"},
	"ai-gateway":            {"ai-gateway"},
	"nous":                  {"nous"},
	"grok-oauth":            {"grok-oauth"},
}

// AliasToCanonical maps alias -> canonical provider id.
var AliasToCanonical = func() map[string]string {
	m := map[string]string{}
	for canon, aliases := range AliasGroups {
		for _, a := range aliases {
			m[strings.ToLower(a)] = canon
		}
	}
	return m
}()

// normalizeProvider resolves aliases and normalises casing to a canonical provider id.
func normalizeProvider(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if canon, ok := AliasToCanonical[key]; ok {
		return canon
	}
	// Built-in providers (direct, no alias) pass through with casing normalised.
	return key
}

// ---- Model registry ---------------------------------------------------------------

// ModelRegistry is a model catalog grouped by provider, with per-provider
// name normalization. Mirrors hermes_cli.models / model_normalize.py.
type ModelRegistry struct {
	models map[Provider][]ModelDef
	mu     sync.RWMutex
}

// NewModelRegistry builds an empty registry.
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models: map[Provider][]ModelDef{},
	}
}

// Register adds a model to the registry.
func (r *ModelRegistry) Register(model ModelDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[model.Provider] = append(r.models[model.Provider], model)
}

// RegisterMany adds several models at once.
func (r *ModelRegistry) RegisterMany(models []ModelDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, md := range models {
		r.models[md.Provider] = append(r.models[md.Provider], md)
	}
}

// ByProvider returns models for a provider (shallow copy).
func (r *ModelRegistry) ByProvider(provider Provider) []ModelDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelDef, len(r.models[provider]))
	copy(out, r.models[provider])
	return out
}

// All returns every model across all providers.
func (r *ModelRegistry) All() []ModelDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]ModelDef{}
	for _, models := range r.models {
		for _, md := range models {
			seen[md.ID] = md
		}
	}
	out := make([]ModelDef, 0, len(seen))
	for _, md := range seen {
		out = append(out, md)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Find returns a model by (provider, modelID).
func (r *ModelRegistry) Find(provider Provider, modelID string) (ModelDef, bool) {
	for _, md := range r.models[provider] {
		if md.ID == modelID {
			return md, true
		}
	}
	return ModelDef{}, false
}

// ---- Model name normalization -------------------------------------------------

// normalizeModelForProvider translates a model name (bare, vendor-prefixed or
// native) into what the target provider's API expects.
func normalizeModelForProvider(modelInput string, targetProvider Provider) string {
	name := strings.TrimSpace(modelInput)
	if name == "" {
		return name
	}
	canonical := strings.ToLower(targetProvider.String())

	// Aggregators (OpenRouter, Nous, AI Gateway, Kilocode): /v1/models returns
	// "vendor/model" slugs; prepend the detected vendor.
	if isAggregator(canonical) {
		return prependVendor(name)
	}

	// OpenCode Zen / Go families: /v1/models returns bare IDs and 401s on
	// vendor-prefixed names; strip ANY leading vendor/.
	if isOpenCodeFamily(canonical) {
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = strings.TrimSpace(name[idx+1:])
		}
		if strings.EqualFold(canonical, "opencode-zen") && strings.HasPrefix(strings.ToLower(name), "claude-") {
			return dotsToHyphens(name)
		}
		return name
	}

	// Dot-to-hyphen providers (e.g. anthropic).
	if dotToHyphen(canonical) {
		stripped := stripMatchingProviderPrefix(name, canonical)
		if strings.Contains(stripped, "/") {
			return stripped
		}
		return dotsToHyphens(stripped)
	}

	// Authoritative-native providers (huggingface): pass through unchanged.
	if authoritiveNative(canonical) {
		return name
	}

	// Matching-prefix-strip providers: strip provider/ only when it matches
	// the target provider; lowercase for specific vendors.
	if matchingPrefixStrip(canonical) {
		result := stripMatchingProviderPrefix(name, canonical)
		if strings.EqualFold(canonical, "xiaomi") {
			return strings.ToLower(result)
		}
		return result
	}

	// DeepSeek retired aliases fold onto current id.
	if canonical == "deepseek" {
		return normalizeForDeepSeek(name)
	}

	// Catalogue-prefix-repair providers (nvidia): repair only when the bare id
	// matches exactly one curated entry.
	if cataloguePrefixRepair(canonical) {
		return repairPrefixFromCatalogue(name, canonical)
	}

	// Unknown / custom providers: pass through as-is.
	return name
}

// ---- Per-provider normalisation helpers (mirroring hermes) -------------------------

var (
	_AGGREGATOR_PROVIDERS        = map[string]bool{"openrouter": true, "nous": true, "ai-gateway": true, "kilocode": true, "grok-oauth": true}
	_DOT_TO_HYPHEN               = map[string]bool{"anthropic": true}
	_STRIP_VENDOR_ONLY           = map[string]bool{"openai-codex": true}
	_STRIP_VENDOR_ONLY_2         = map[string]bool{"openai-codex": true}
	_MATCHING_PREFIX_STRIP       = map[string]bool{"zai": true, "kimi-coding": true, "minimax": true, "minimax-oauth": true, "minimax-cn": true, "alibaba": true, "qwen-oauth": true, "xiaomi": true, "arcee": true, "ollama-cloud": true, "nebius-token-factory": true, "custom": true, "gemini": true, "xai": true, "kimi-coding-cn": true}
	_CATALOGUE_PREFIX_REPAIR     = map[string]bool{"nvidia": true}
	_LOWERCASE_MODEL_PROVIDERS   = map[string]bool{"xiaomi": true}
	_DEEPSEEK_RETIRED_ALIASES    = map[string]string{"deepseek-chat": "deepseek-flash", "deepseek-reasoner": "deepseek-flash"}
	_STRIP_VENDOR_ONLY_PROVIDERS = map[string]bool{"openai-codex": true}
	_AUTHORITATIVE_NATIVE        = map[string]bool{"huggingface": true}
)

func isAggregator(p string) bool          { return _AGGREGATOR_PROVIDERS[p] }
func dotToHyphen(p string) bool           { return _DOT_TO_HYPHEN[p] }
func authoritiveNative(p string) bool     { return _AUTHORITATIVE_NATIVE[p] }
func matchingPrefixStrip(p string) bool   { return _MATCHING_PREFIX_STRIP[p] }
func cataloguePrefixRepair(p string) bool { return _CATALOGUE_PREFIX_REPAIR[p] }

func prependVendor(modelName string) string {
	if strings.Contains(modelName, "/") {
		return modelName
	}
	return detectVendor(modelName) + "/" + modelName
}

func detectVendor(modelName string) string {
	name := strings.TrimSpace(modelName)
	if name == "" {
		return ""
	}
	if idx := strings.Index(name, "/"); idx >= 0 {
		return strings.ToLower(name[:idx])
	}
	lower := strings.ToLower(name)
	firstToken := lower
	if i := strings.Index(lower, "-"); i > 0 {
		firstToken = lower[:i]
	}
	// Known vendor mapping (per hermes _VENDOR_PREFIXES). First try exact match,
	// then prefix match (e.g. "llama3.3" → "meta-llama").
	variants := map[string]string{
		"claude": "anthropic", "gpt": "openai", "o1": "openai", "o3": "openai",
		"o4": "openai", "gemini": "google", "gemma": "google", "deepseek": "deepseek",
		"glm": "z-ai", "kimi": "moonshotai", "minimax": "minimax", "grok": "x-ai",
		"qwen": "qwen", "mimo": "xiaomi", "trinity": "arcee-ai", "nemotron": "nvidia",
		"llama": "meta-llama", "step": "stepfun",
	}
	if vendor, ok := variants[firstToken]; ok {
		return vendor
	}
	// Prefix match for model names like "llama3.3", "gpt-4o", "qwen3-8b"
	for prefix, vendor := range variants {
		if strings.HasPrefix(lower, prefix) {
			return vendor
		}
	}
	return ""
}

func dotsToHyphens(s string) string {
	return strings.ReplaceAll(s, ".", "-")
}

func stripMatchingProviderPrefix(modelName, targetProvider string) string {
	cut := -1
	if i := strings.Index(modelName, "/"); i >= 0 {
		cut = i
	}
	if i := strings.Index(modelName, ":"); i >= 0 && (cut < 0 || i < cut) {
		cut = i
	}
	if cut < 0 {
		return modelName
	}
	prefix := strings.TrimSpace(modelName[:cut])
	remainder := strings.TrimSpace(modelName[cut+1:])
	if prefix == "" || remainder == "" {
		return modelName
	}
	normalizedPrefix := normalizeProvider(prefix)
	if normalizedPrefix == "custom" {
		if strings.ToLower(prefix) == "custom" {
			return remainder
		}
		return modelName
	}
	return remainder
}

func normalizeForDeepSeek(modelName string) string {
	dotted := dotsToHyphens(strings.ToLower(strings.TrimSpace(modelName)))
	if alias, ok := _DEEPSEEK_RETIRED_ALIASES[dotted]; ok {
		return alias
	}
	return dotted
}

// repairPrefixFromCatalogue restores a dropped vendor/ prefix only when the bare id
// matches exactly one curated entry. In this minimal build, no catalogue exists, so
// the function returns the input unchanged (callers treat unknown as a 404).
func repairPrefixFromCatalogue(modelName, provider string) string {
	return modelName
}

// isOpenCodeFamily reports whether provider is an OpenCode Zen/Go family (flat-namespace).
func isOpenCodeFamily(provider string) bool {
	return provider == "opencode-zen" || provider == "opencode-go"
}

// ---- Private gateway support (hermes-style) -------------------------------------

// ProviderBaseConfig holds the endpoint configuration for a provider, including
// the base URL override (private gateway) and the environment variable that
// names it. Mirrors hermes_cli/providers.py HermesOverlay.
type ProviderBaseConfig struct {
	BaseURL       string // default endpoint base URL
	BaseURLEnvVar string // env var naming a user-custom base URL (override)
}

// DefaultProviderBaseConfig returns the canonical base URL for each provider,
// mirroring hermes_cli/providers.py + models.dev. Users can override via the
// BaseURLEnvVar (private gateway mode).
func DefaultProviderBaseConfig() map[Provider]ProviderBaseConfig {
	return map[Provider]ProviderBaseConfig{
		ProviderAnthropic: {
			BaseURL:       "https://api.anthropic.com/v1/messages",
			BaseURLEnvVar: "ANTHROPIC_BASE_URL",
		},
		ProviderGemini: {
			BaseURL:       "https://generativelanguage.googleapis.com/v1beta/openai/",
			BaseURLEnvVar: "GEMINI_BASE_URL",
		},
		ProviderOpenAI: {
			BaseURL:       "https://api.openai.com/v1",
			BaseURLEnvVar: "OPENAI_BASE_URL",
		},
		ProviderxAI: {
			BaseURL:       "https://api.x.ai/v1",
			BaseURLEnvVar: "XAI_BASE_URL",
		},
		ProviderOpenRouter: {
			BaseURL:       "https://openrouter.ai/api/v1",
			BaseURLEnvVar: "OPENROUTER_BASE_URL",
		},
		ProviderOllama: {
			BaseURL:       "http://localhost:11434/api",
			BaseURLEnvVar: "OLLAMA_BASE_URL",
		},
		// Extended providers (opencode/kilocode/commandcode/ollama-cloud families)
		ProviderCommandCode: {
			BaseURL:       "https://api.commandcode.ai/provider/v1",
			BaseURLEnvVar: "COMMANDCODE_BASE_URL",
		},
		ProviderCommandCodeAnthropic: {
			BaseURL:       "https://api.commandcode.ai/provider/v1",
			BaseURLEnvVar: "COMMANDCODE_ANTHROPIC_BASE_URL",
		},
		ProviderKiloCode: {
			BaseURL:       "https://api.kilo.ai/api/gateway",
			BaseURLEnvVar: "KILOCODE_BASE_URL",
		},
		ProviderOpenCodeZen: {
			BaseURL:       "https://opencode.ai/zen/v1",
			BaseURLEnvVar: "OPENCODE_ZEN_BASE_URL",
		},
		ProviderOpenCodeGo: {
			BaseURL:       "https://opencode.ai/zen/go/v1",
			BaseURLEnvVar: "OPENCODE_GO_BASE_URL",
		},
		ProviderOllamaCloud: {
			BaseURL:       "https://ollama.com/v1",
			BaseURLEnvVar: "OLLAMA_CLOUD_BASE_URL",
		},
	}
}

// ---- Private gateway support (hermes-style) -------------------------------------

// UserProviderConfig is the per-user provider configuration loaded from the
// environment (hermes-style config.yaml providers section). It overrides the
// built-in default base URL for the selected provider.
type UserProviderConfig struct {
	BaseURL string // explicit private-gateway endpoint
	KeyEnv  string // env var for the API key (e.g. "MH_API_KEY")
}

// LoadProviderConfig loads per-user provider configuration from environment
// variables, mirroring hermes CLI config.yaml providers section.
// Format (env vars):
//
//	BH_PROVIDER_<NAME>_BASE_URL  - base URL override for <NAME>
//	BH_PROVIDER_<NAME>_KEY_ENV   - env var naming the API key for <NAME>
func LoadProviderConfig() map[string]UserProviderConfig {
	cfg := map[string]UserProviderConfig{}
	// Scan common env var patterns for provider overrides.
	for _, env := range os.Environ() {
		idx := strings.IndexByte(env, '=')
		if idx < 0 {
			continue
		}
		key := env[:idx]
		val := env[idx+1:]
		if strings.HasPrefix(key, "BH_PROVIDER_") && strings.HasSuffix(key, "_BASE_URL") {
			name := strings.TrimSuffix(strings.TrimPrefix(key, "BH_PROVIDER_"), "_BASE_URL")
			cfg[name] = UserProviderConfig{BaseURL: val}
		}
		if strings.HasPrefix(key, "BH_PROVIDER_") && strings.HasSuffix(key, "_KEY_ENV") {
			name := strings.TrimSuffix(strings.TrimPrefix(key, "BH_PROVIDER_"), "_KEY_ENV")
			cfg[name] = UserProviderConfig{KeyEnv: val}
		}
	}
	return cfg
}

// GetBaseURL resolves the effective base URL for a provider, checking for a
// user-configured override first (BH_PROVIDER_<NAME>_BASE_URL), then the
// built-in default (DefaultProviderBaseConfig), then the env var (BaseURLEnvVar).
func GetBaseURL(provider Provider, apiKeyEnvVar string) string {
	if apiKeyEnvVar == "" {
		apiKeyEnvVar = "BH_API_KEY"
	}
	// 1. Check for user-configured base URL via BH_PROVIDER_<NAME>_BASE_URL.
	if cfg := LoadProviderConfig(); cfg != nil {
		if userCfg, ok := cfg[provider.String()]; ok && userCfg.BaseURL != "" {
			return userCfg.BaseURL
		}
	}
	// 2. Check the built-in default base URL (mirrors hermes_cli/providers.py).
	if cfg, ok := DefaultProviderBaseConfig()[provider]; ok {
		// 3. Check for user override via BaseURLEnvVar (private gateway mode).
		if envVal := os.Getenv(cfg.BaseURLEnvVar); envVal != "" {
			return envVal
		}
		return cfg.BaseURL
	}
	// Fallback: OpenAI-compatible endpoint.
	return "https://api.openai.com/v1"
}

// readLine reads a line from stdin using unbuffered reads.
func readLine() (string, error) {
	reader := bufio.NewReaderSize(os.Stdin, 1)
	return reader.ReadString('\n')
}

// ---- Interactive selection menus (opencode/hermes-style pickers) ----------

// SelectProvider shows a numbered provider picker and returns the selected provider.
// Input can be a number (1-n) or a provider name; "q" quits to the parent menu.
// reader provides the input stream shared with the REPL.
func SelectProvider(reader *bufio.Reader) (Provider, error) {
	pr := RegisterBuiltInProviders()
	provs := pr.List()
	if len(provs) == 0 {
		return ProviderOpenAI, nil
	}
	fmt.Println("🔍 Select provider")
	fmt.Println("=================")
	for i, p := range provs {
		fmt.Printf("  %d. %s\n", i+1, p.Name)
	}
	fmt.Println("q. Quit")
	fmt.Print("Enter number (1-" + strconv.Itoa(len(provs)) + ") or name: ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return ProviderOpenAI, err
	}
	line = strings.TrimSpace(line)
	if line == "q" || line == "quit" {
		return ProviderOpenAI, nil
	}
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(provs) {
		return Provider(provs[n-1].ID), nil
	}
	return Provider(normalizeProvider(line)), nil
}

// SelectModel shows a numbered model picker for the given provider.
// "q" or empty input goes back to the parent menu.
// reader provides the input stream shared with the REPL.
func SelectModel(provider Provider, reader *bufio.Reader) (ModelDef, error) {
	pr := RegisterBuiltInProviders()
	provs := pr.List()
	var provDef ProviderDef
	for _, p := range provs {
		if p.ID == string(provider) {
			provDef = p
			break
		}
	}
	if provDef.ID == "" {
		return ModelDef{}, fmt.Errorf("provider not found: %s", provider)
	}
	models := DefaultModels()
	var provModels []ModelDef
	for _, md := range models.byProvider[provider] {
		provModels = append(provModels, ModelDef{
			ID:          md.ID,
			Name:        md.Name,
			Provider:    md.Provider,
			ContextLen:  0,
			IsLocal:     md.IsLocal,
			Description: md.Name,
		})
	}
	if len(provModels) == 0 {
		return ModelDef{}, fmt.Errorf("no models for provider %s", provider)
	}
	fmt.Println("🔍 Select model")
	fmt.Println("=============")
	for i, m := range provModels {
		fmt.Printf("  %d. %s\n", i+1, m.Name)
	}
	fmt.Println("q. Back")
	fmt.Print("Enter number (1-" + strconv.Itoa(len(provModels)) + ") or leave empty: ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return ModelDef{}, err
	}
	line = strings.TrimSpace(line)
	if line == "q" || line == "" {
		return ModelDef{}, nil
	}
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(provModels) {
		return provModels[n-1], nil
	}
	for _, m := range provModels {
		if m.Name == line || m.ID == line {
			return m, nil
		}
	}
	return ModelDef{}, fmt.Errorf("invalid model: %s", line)
}

// RegisterBuiltInProviders registers all built-in providers with a new
// ProviderRegistry. This is called by New() in types.go.
func RegisterBuiltInProviders() *ProviderRegistry {
	pr := NewProviderRegistry()
	pr.Register(ProviderDef{
		ID:            "openai",
		Name:          "OpenAI",
		BaseURL:       "https://api.openai.com/v1",
		BaseURLEnvVar: "OPENAI_BASE_URL",
		APIKeyEnvVars: []string{"OPENAI_BASE_URL"},
	})
	pr.Register(ProviderDef{
		ID:            "anthropic",
		Name:          "Anthropic",
		BaseURL:       "https://api.anthropic.com/v1/messages",
		BaseURLEnvVar: "ANTHROPIC_BASE_URL",
		APIKeyEnvVars: []string{"ANTHROPIC_BASE_URL"},
	})
	pr.Register(ProviderDef{
		ID:            "gemini",
		Name:          "Google Gemini",
		BaseURL:       "https://generativelanguage.googleapis.com/v1beta/openai/",
		BaseURLEnvVar: "GEMINI_BASE_URL",
		APIKeyEnvVars: []string{"GEMINI_BASE_URL"},
	})
	pr.Register(ProviderDef{
		ID:            "xai",
		Name:          "xAI",
		BaseURL:       "https://api.x.ai/v1",
		BaseURLEnvVar: "XAI_BASE_URL",
		APIKeyEnvVars: []string{"XAI_BASE_URL"},
	})
	pr.Register(ProviderDef{
		ID:            "openrouter",
		Name:          "OpenRouter",
		BaseURL:       "https://openrouter.ai/api/v1",
		BaseURLEnvVar: "OPENROUTER_BASE_URL",
		APIKeyEnvVars: []string{"OPENROUTER_BASE_URL"},
	})
	pr.Register(ProviderDef{
		ID:            "ollama",
		Name:          "Ollama",
		BaseURL:       "http://localhost:11434/api",
		BaseURLEnvVar: "OLLAMA_BASE_URL",
		APIKeyEnvVars: []string{"OLLAMA_BASE_URL"},
	})
	// Extended providers
	pr.Register(ProviderDef{
		ID:            "commandcode",
		Name:          "CommandCode",
		BaseURL:       "https://api.commandcode.ai/provider/v1",
		BaseURLEnvVar: "COMMANDCODE_BASE_URL",
		APIKeyEnvVars: []string{"COMMANDCODE_BASE_URL"},
	})
	pr.Register(ProviderDef{
		ID:            "commandcode-anthropic",
		Name:          "CommandCode (Anthropic)",
		BaseURL:       "https://api.commandcode.ai/provider/v1",
		BaseURLEnvVar: "COMMANDCODE_ANTHROPIC_BASE_URL",
		APIKeyEnvVars: []string{"COMMANDCODE_ANTHROPIC_BASE_URL"},
	})
	pr.Register(ProviderDef{
		ID:            "kilocode",
		Name:          "KiloCode",
		BaseURL:       "https://api.kilo.ai/api/gateway",
		BaseURLEnvVar: "KILOCODE_BASE_URL",
		APIKeyEnvVars: []string{"KILOCODE_BASE_URL"},
	})
	pr.Register(ProviderDef{
		ID:            "opencode-zen",
		Name:          "OpenCode Zen",
		BaseURL:       "https://opencode.ai/zen/v1",
		BaseURLEnvVar: "OPENCODE_ZEN_BASE_URL",
		APIKeyEnvVars: []string{"OPENCODE_ZEN_BASE_URL"},
	})
	pr.Register(ProviderDef{
		ID:            "opencode-go",
		Name:          "OpenCode Go",
		BaseURL:       "https://opencode.ai/zen/go/v1",
		BaseURLEnvVar: "OPENCODE_GO_BASE_URL",
		APIKeyEnvVars: []string{"OPENCODE_GO_BASE_URL"},
	})
	pr.Register(ProviderDef{
		ID:            "ollama-cloud",
		Name:          "Ollama Cloud",
		BaseURL:       "https://ollama.com/v1",
		BaseURLEnvVar: "OLLAMA_CLOUD_BASE_URL",
		APIKeyEnvVars: []string{"OLLAMA_CLOUD_BASE_URL"},
	})
	return pr
}
