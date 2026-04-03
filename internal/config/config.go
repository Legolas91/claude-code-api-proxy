// Package config handles configuration loading from environment variables and .env files.
//
// It supports multiple config file locations (./.env, ~/.claude/proxy.env, ~/.cc-api-proxy)
// and detects the provider type (OpenRouter, OpenAI, Ollama) based on the OPENAI_BASE_URL.
// The package also handles model overrides for routing Claude model names to alternative providers.
package config

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// ProviderType represents the backend provider type
type ProviderType string

const (
	ProviderOpenRouter ProviderType = "openrouter"
	ProviderOpenAI     ProviderType = "openai"
	ProviderOllama     ProviderType = "ollama"
	ProviderAnthropic  ProviderType = "anthropic" // Direct Anthropic API passthrough (with API key)
	ProviderClaudeCode  ProviderType = "claudecode" // Claude CLI backend (Pro/Max subscription, no API key)
	ProviderUnknown    ProviderType = "unknown"
)

// CacheKey uniquely identifies a (provider, model) combination for capability caching
// Using a struct as map key provides type safety and zero collision risk
type CacheKey struct {
	BaseURL string // Provider base URL (e.g., "https://openrouter.ai/api/v1")
	Model   string // Model name (e.g., "gpt-5", "openai/gpt-5")
}

// ModelCapabilities tracks which parameters a specific model supports.
// Learned dynamically through adaptive retry mechanism.
// Cache is in-memory only (cleared on restart).
type ModelCapabilities struct {
	UsesMaxCompletionTokens bool // Does this model use max_completion_tokens?
}

// Global capability cache ((baseURL, model) -> capabilities)
// Protected by mutex for thread-safe access across concurrent requests
var (
	modelCapabilityCache = make(map[CacheKey]*ModelCapabilities)
	capabilityCacheMutex sync.RWMutex
)

// Config holds all proxy configuration
type Config struct {
	// Required
	OpenAIAPIKey string

	// Optional
	OpenAIBaseURL   string
	AnthropicAPIKey string

	// Model routing (pattern-based if not set)
	OpusModel   string
	SonnetModel string
	HaikuModel  string

	// Per-tier base URLs (optional, fallback to OpenAIBaseURL)
	OpusBaseURL   string
	SonnetBaseURL string
	HaikuBaseURL  string

	// Per-tier API keys (optional, fallback to OpenAIAPIKey)
	OpusAPIKey   string
	SonnetAPIKey string
	HaikuAPIKey  string

	// Server settings
	Host string
	Port string

	// Debug logging
	Debug bool

	// Simple logging - one-line summary per request
	SimpleLog bool

	// Passthrough mode - directly proxy to Anthropic without conversion
	PassthroughMode bool

	// OpenRouter-specific (optional, improves rate limits)
	OpenRouterAppName string
	OpenRouterAppURL  string

	// HTTP Proxy configuration (enterprise support)
	HTTPProxy    string // Override HTTP_PROXY env var
	HTTPSProxy   string // Override HTTPS_PROXY env var
	NoProxy      string // Comma-separated list of hosts to exclude from proxy
	ProxyFromEnv bool   // Use system proxy env vars (default: true)

	// Rate limiting (0 = disabled)
	RateLimitRPM int // Maximum requests per minute across all clients

	// Loop detection: max consecutive identical tool calls before injecting a nudge (0 = disabled)
	MaxIdenticalRetries int
	// Maximum loop escalation level (1=gentle nudge only, 2=strong nudge, 3=disable tools)
	MaxLoopLevel int

	// Tool prompt augmentation (nil=auto-detect by provider, true=always, false=never)
	AugmentToolPrompt *bool
	// Override per-model tool guidance template (empty = use model-based auto-selection)
	ToolPromptTemplate string

	// Repair malformed tool call arguments in non-streaming responses (true=repair, false=pass through)
	RepairToolCalls bool

	// Response cache (in-memory, non-streaming only, opt-in)
	CacheEnabled        bool
	CacheMaxEntries     int
	CacheMaxTemperature float64
}

// Load reads configuration from environment variables
// Tries multiple locations: ./.env, ~/.claude/proxy.env, ~/.cc-api-proxy
func Load() (*Config, error) {
	// Try loading .env files in priority order
	locations := []string{
		".env",
		filepath.Join(os.Getenv("HOME"), ".claude", "proxy.env"),
		filepath.Join(os.Getenv("HOME"), ".cc-api-proxy"),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			// File exists, load it (overload to override existing env vars)
			if err := godotenv.Overload(loc); err == nil {
				fmt.Printf("📁 Loaded config from: %s\n", loc)
				break
			}
		}
	}

	// Build config from environment
	cfg := &Config{
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL:   getEnvOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),

		// Pattern-based routing (optional overrides)
		OpusModel:   os.Getenv("ANTHROPIC_DEFAULT_OPUS_MODEL"),
		SonnetModel: os.Getenv("ANTHROPIC_DEFAULT_SONNET_MODEL"),
		HaikuModel:  os.Getenv("ANTHROPIC_DEFAULT_HAIKU_MODEL"),

		// Per-tier base URLs (optional, fallback to OpenAIBaseURL)
		OpusBaseURL:   os.Getenv("ANTHROPIC_DEFAULT_OPUS_BASE_URL"),
		SonnetBaseURL: os.Getenv("ANTHROPIC_DEFAULT_SONNET_BASE_URL"),
		HaikuBaseURL:  os.Getenv("ANTHROPIC_DEFAULT_HAIKU_BASE_URL"),

		// Per-tier API keys (optional, fallback to OpenAIAPIKey)
		OpusAPIKey:   os.Getenv("ANTHROPIC_DEFAULT_OPUS_API_KEY"),
		SonnetAPIKey: os.Getenv("ANTHROPIC_DEFAULT_SONNET_API_KEY"),
		HaikuAPIKey:  os.Getenv("ANTHROPIC_DEFAULT_HAIKU_API_KEY"),

		// Server settings
		Host: getEnvOrDefault("HOST", "0.0.0.0"),
		Port: getEnvOrDefault("PORT", "8082"),

		// Passthrough mode
		PassthroughMode: getEnvAsBoolOrDefault("PASSTHROUGH_MODE", false),

		// OpenRouter-specific (optional)
		OpenRouterAppName: os.Getenv("OPENROUTER_APP_NAME"),
		OpenRouterAppURL:  os.Getenv("OPENROUTER_APP_URL"),

		// HTTP Proxy configuration (enterprise support)
		HTTPProxy:    os.Getenv("CLAUDE_HTTP_PROXY"),
		HTTPSProxy:   os.Getenv("CLAUDE_HTTPS_PROXY"),
		NoProxy:      os.Getenv("CLAUDE_NO_PROXY"),
		ProxyFromEnv: getEnvAsBoolOrDefault("CLAUDE_PROXY_FROM_ENV", true),

		// Rate limiting (0 = disabled)
		RateLimitRPM: getEnvAsIntOrDefault("RATE_LIMIT_RPM", 0),

		// Loop detection (0 = disabled)
		MaxIdenticalRetries: getEnvAsIntOrDefault("PROXY_MAX_IDENTICAL_RETRIES", 3),
		// Max loop escalation level (1=gentle only, 2=strong nudge, 3=disable tools)
		MaxLoopLevel: getEnvAsIntOrDefault("PROXY_MAX_LOOP_LEVEL", 3),

		// Tool prompt augmentation (nil=auto, true=always, false=never)
		AugmentToolPrompt:  getEnvAsBoolPtrOrNil("PROXY_AUGMENT_TOOL_PROMPT"),
		ToolPromptTemplate: os.Getenv("PROXY_TOOL_PROMPT_TEMPLATE"),

		// Repair malformed tool call arguments (default: enabled)
		RepairToolCalls: getEnvAsBoolOrDefault("PROXY_REPAIR_TOOL_CALLS", true),

		// Response cache (default: disabled)
		CacheEnabled:        getEnvAsBoolOrDefault("PROXY_CACHE_ENABLED", false),
		CacheMaxEntries:     getEnvAsIntOrDefault("PROXY_CACHE_MAX_ENTRIES", 100),
		CacheMaxTemperature: getEnvAsFloatOrDefault("PROXY_CACHE_MAX_TEMPERATURE", 0),
	}

	// Validate required fields
	// Allow missing API key for:
	// - Ollama (localhost endpoints)
	// - Anthropic claude-p mode (api.anthropic.com without key → uses claude CLI subscription)
	// - Per-tier configs where all tiers have their own keys/URLs
	if cfg.OpenAIAPIKey == "" {
		isLocalhost := strings.Contains(cfg.OpenAIBaseURL, "localhost") ||
			strings.Contains(cfg.OpenAIBaseURL, "127.0.0.1")
		isAnthropic := strings.Contains(strings.ToLower(cfg.OpenAIBaseURL), "anthropic.com")
		hasAllTierURLs := cfg.OpusBaseURL != "" && cfg.SonnetBaseURL != "" && cfg.HaikuBaseURL != ""

		if !isLocalhost && !isAnthropic && !hasAllTierURLs {
			return nil, fmt.Errorf("OPENAI_API_KEY is required (unless using localhost/Ollama, anthropic.com/claude-p, or per-tier URLs)")
		}
		if isLocalhost {
			cfg.OpenAIAPIKey = "ollama"
		}
	}

	return cfg, nil
}

// LoadWithDebug loads config and sets debug mode
func LoadWithDebug(debug bool) (*Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	cfg.Debug = debug
	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsBoolPtrOrNil returns a *bool if the env var is set, nil otherwise.
// Used for tri-state configuration (nil = auto-detect, true = force on, false = force off).
func getEnvAsBoolPtrOrNil(key string) *bool {
	if value := os.Getenv(key); value != "" {
		b := value == "true" || value == "1" || value == "yes"
		return &b
	}
	return nil
}

func getEnvAsBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

func getEnvAsIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n >= 0 {
			return n
		}
	}
	return defaultValue
}

func getEnvAsFloatOrDefault(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

// DetectProvider identifies the provider type based on base URL
func (c *Config) DetectProvider() ProviderType {
	baseURL := strings.ToLower(c.OpenAIBaseURL)

	if strings.Contains(baseURL, "openrouter.ai") {
		return ProviderOpenRouter
	}
	if strings.Contains(baseURL, "api.openai.com") {
		return ProviderOpenAI
	}
	if strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") {
		return ProviderOllama
	}
	return ProviderUnknown
}

// IsLocalhost returns true if the base URL points to localhost
func (c *Config) IsLocalhost() bool {
	baseURL := strings.ToLower(c.OpenAIBaseURL)
	return strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1")
}

// DetectProviderForURL identifies the provider type for a specific base URL and API key.
// This enables per-tier provider detection (different tiers can use different providers).
//   - "api.anthropic.com" + API key present → ProviderAnthropic (passthrough)
//   - "api.anthropic.com" + no API key      → ProviderClaudeCode (spawn claude -p)
//   - Other URLs                            → standard detection (OpenRouter/OpenAI/Ollama/Unknown)
func DetectProviderForURL(baseURL, apiKey string) ProviderType {
	u := strings.ToLower(baseURL)

	if strings.Contains(u, "anthropic.com") {
		if apiKey != "" {
			return ProviderAnthropic
		}
		return ProviderClaudeCode
	}
	if strings.Contains(u, "openrouter.ai") {
		return ProviderOpenRouter
	}
	if strings.Contains(u, "api.openai.com") {
		return ProviderOpenAI
	}
	if strings.Contains(u, "localhost") || strings.Contains(u, "127.0.0.1") {
		return ProviderOllama
	}
	return ProviderUnknown
}

// GetProviderForTier returns the provider configuration (baseURL, apiKey, model) for a given tier.
// Falls back to default values (OpenAIBaseURL, OpenAIAPIKey) if tier-specific values are not set.
// This enables optional multi-provider routing where different Claude tiers can use different APIs.
func (c *Config) GetProviderForTier(tier string) (baseURL, apiKey, model string) {
	var hasExplicitURL bool
	switch tier {
	case "opus":
		baseURL = c.OpusBaseURL
		apiKey = c.OpusAPIKey
		model = c.OpusModel
		hasExplicitURL = c.OpusBaseURL != ""
	case "sonnet":
		baseURL = c.SonnetBaseURL
		apiKey = c.SonnetAPIKey
		model = c.SonnetModel
		hasExplicitURL = c.SonnetBaseURL != ""
	case "haiku":
		baseURL = c.HaikuBaseURL
		apiKey = c.HaikuAPIKey
		model = c.HaikuModel
		hasExplicitURL = c.HaikuBaseURL != ""
	}

	// Fallback to default values if tier-specific not set
	if baseURL == "" {
		baseURL = c.OpenAIBaseURL
	}
	// Only fallback apiKey if no explicit URL was configured for this tier.
	// When a tier-specific URL is set without an API key, the absence is intentional
	// (e.g. ProviderClaudeCode: api.anthropic.com without a key spawns claude -p).
	if apiKey == "" && !hasExplicitURL {
		apiKey = c.OpenAIAPIKey
	}

	return baseURL, apiKey, model
}

// GetModelCapabilities retrieves cached capabilities for a (provider, model) combination.
// Returns nil if no capabilities are cached yet (first request for this model).
// Thread-safe with read lock.
func GetModelCapabilities(key CacheKey) *ModelCapabilities {
	capabilityCacheMutex.RLock()
	defer capabilityCacheMutex.RUnlock()
	return modelCapabilityCache[key]
}

// SetModelCapabilities caches the capabilities for a (provider, model) combination.
// This is called after detecting what parameters a specific model supports through adaptive retry.
// Thread-safe with write lock.
func SetModelCapabilities(key CacheKey, capabilities *ModelCapabilities) {
	capabilityCacheMutex.Lock()
	defer capabilityCacheMutex.Unlock()
	modelCapabilityCache[key] = capabilities
}

// GetBaseURLForModel returns the base URL for the given model name.
// It checks if the model matches a tier with a specific base URL configured,
// otherwise falls back to the default OpenAIBaseURL.
func (c *Config) GetBaseURLForModel(modelName string) string {
	if c.OpusModel != "" && modelName == c.OpusModel && c.OpusBaseURL != "" {
		return c.OpusBaseURL
	}
	if c.SonnetModel != "" && modelName == c.SonnetModel && c.SonnetBaseURL != "" {
		return c.SonnetBaseURL
	}
	if c.HaikuModel != "" && modelName == c.HaikuModel && c.HaikuBaseURL != "" {
		return c.HaikuBaseURL
	}
	return c.OpenAIBaseURL
}

// ShouldUseMaxCompletionTokens determines if we should send max_completion_tokens
// based on cached model capabilities learned through adaptive detection.
// No hardcoded model patterns - tries max_completion_tokens for ALL models on first request.
func (c *Config) ShouldUseMaxCompletionTokens(modelName string) bool {
	// Build cache key for this (provider, model) combination
	key := CacheKey{
		BaseURL: c.GetBaseURLForModel(modelName),
		Model:   modelName,
	}

	// Check if we have cached knowledge about this specific model
	caps := GetModelCapabilities(key)
	if caps != nil {
		// Cache hit - use learned capability
		if c.Debug {
			fmt.Printf("[DEBUG] Cache HIT: %s → max_completion_tokens=%v\n",
				modelName, caps.UsesMaxCompletionTokens)
		}
		return caps.UsesMaxCompletionTokens
	}

	// Cache miss - default to max_tokens for maximum compatibility
	// Most providers (including enterprise API gateways) support max_tokens.
	// The retry mechanism in handlers.go handles the opposite case if needed.
	if c.Debug {
		fmt.Printf("[DEBUG] Cache MISS: %s → will auto-detect (try max_tokens first)\n", modelName)
	}
	return false
}

// GetHTTPTransport returns an http.Transport configured with proxy settings.
// Priority order:
// 1. CLAUDE_HTTP_PROXY / CLAUDE_HTTPS_PROXY (custom override)
// 2. HTTP_PROXY / HTTPS_PROXY (system default) if ProxyFromEnv == true
// 3. No proxy if all disabled
func (c *Config) GetHTTPTransport() *http.Transport {
	transport := &http.Transport{
		// Keep default transport settings for performance
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Check if we should use custom proxy or system proxy
	if c.HTTPProxy != "" || c.HTTPSProxy != "" {
		// Custom proxy override
		transport.Proxy = func(req *http.Request) (*url.URL, error) {
			// Check NO_PROXY list first
			// Extract hostname without port for NO_PROXY matching
			hostname := req.URL.Hostname()
			if c.NoProxy != "" && shouldBypassProxy(hostname, c.NoProxy) {
				return nil, nil
			}

			// Use HTTPS_PROXY for https:// URLs, HTTP_PROXY for http://
			var proxyURL string
			if req.URL.Scheme == "https" && c.HTTPSProxy != "" {
				proxyURL = c.HTTPSProxy
			} else if c.HTTPProxy != "" {
				proxyURL = c.HTTPProxy
			}

			if proxyURL == "" {
				return nil, nil
			}

			return url.Parse(proxyURL)
		}
	} else if c.ProxyFromEnv {
		// Use system environment variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
		transport.Proxy = http.ProxyFromEnvironment
	}
	// else: no proxy configured

	return transport
}

// ShouldAugmentToolPrompt returns true if tool-use guidance should be prepended to the system
// prompt. Default behaviour: inject only for unknown providers (enterprise LLM gateways) where
// tool_choice support may be incomplete. Can be overridden with PROXY_AUGMENT_TOOL_PROMPT=true/false.
func (c *Config) ShouldAugmentToolPrompt(provider ProviderType) bool {
	if c.AugmentToolPrompt != nil {
		return *c.AugmentToolPrompt
	}
	// Auto: inject only for unknown providers (e.g. vLLM enterprise gateways)
	return provider == ProviderUnknown
}

// shouldBypassProxy checks if a host should bypass the proxy based on NO_PROXY rules.
// Implements standard NO_PROXY semantics (domain suffix matching, IP matching).
func shouldBypassProxy(host string, noProxy string) bool {
	if noProxy == "" {
		return false
	}

	// Split NO_PROXY into individual patterns
	patterns := strings.Split(noProxy, ",")
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)

		// Exact match
		if pattern == host {
			return true
		}

		// Suffix match (.example.com matches api.example.com)
		if strings.HasPrefix(pattern, ".") && strings.HasSuffix(host, pattern) {
			return true
		}

		// Domain match (example.com matches api.example.com)
		if strings.HasSuffix(host, "."+pattern) {
			return true
		}

		// Wildcard match (*) bypasses all
		if pattern == "*" {
			return true
		}
	}

	return false
}
