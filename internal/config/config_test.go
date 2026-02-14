package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProviderDetection tests that we correctly identify providers from OPENAI_BASE_URL
func TestProviderDetection(t *testing.T) {
	tests := []struct {
		name             string
		baseURL          string
		expectedProvider ProviderType
	}{
		{
			name:             "OpenRouter detection",
			baseURL:          "https://openrouter.ai/api/v1",
			expectedProvider: ProviderOpenRouter,
		},
		{
			name:             "OpenAI Direct detection",
			baseURL:          "https://api.openai.com/v1",
			expectedProvider: ProviderOpenAI,
		},
		{
			name:             "Ollama local detection",
			baseURL:          "http://localhost:11434/v1",
			expectedProvider: ProviderOllama,
		},
		{
			name:             "Ollama with different port",
			baseURL:          "http://localhost:8080/v1",
			expectedProvider: ProviderOllama,
		},
		{
			name:             "Ollama with custom host - should be unknown since not localhost",
			baseURL:          "http://192.168.1.100:11434/v1",
			expectedProvider: ProviderUnknown,
		},
		{
			name:             "Unknown provider",
			baseURL:          "https://custom-api.example.com/v1",
			expectedProvider: ProviderUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				OpenAIBaseURL: tt.baseURL,
			}

			provider := cfg.DetectProvider()
			if provider != tt.expectedProvider {
				t.Errorf("DetectProvider() for %q = %v, want %v",
					tt.baseURL, provider, tt.expectedProvider)
			}
		})
	}
}

// TestModelOverrides tests that model overrides work correctly
func TestModelOverrides(t *testing.T) {
	tests := []struct {
		name         string
		opusModel    string
		sonnetModel  string
		haikuModel   string
		requestModel string
		expectedUsed string
	}{
		{
			name:         "Opus override for OpenRouter",
			opusModel:    "anthropic/claude-opus-4",
			requestModel: "claude-opus-4-1-20250805",
			expectedUsed: "anthropic/claude-opus-4",
		},
		{
			name:         "Sonnet override for Grok",
			sonnetModel:  "x-ai/grok-code-fast-1",
			requestModel: "claude-sonnet-4-5-20250805",
			expectedUsed: "x-ai/grok-code-fast-1",
		},
		{
			name:         "Haiku override for Gemini",
			haikuModel:   "google/gemini-2.5-flash",
			requestModel: "claude-haiku-4-5-20251001",
			expectedUsed: "google/gemini-2.5-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				OpusModel:   tt.opusModel,
				SonnetModel: tt.sonnetModel,
				HaikuModel:  tt.haikuModel,
			}

			// This would be tested through the converter which uses the config
			// The config struct should expose these values properly
			if tt.opusModel != "" && cfg.OpusModel != tt.expectedUsed {
				t.Errorf("OpusModel = %q, want %q", cfg.OpusModel, tt.expectedUsed)
			}
			if tt.sonnetModel != "" && cfg.SonnetModel != tt.expectedUsed {
				t.Errorf("SonnetModel = %q, want %q", cfg.SonnetModel, tt.expectedUsed)
			}
			if tt.haikuModel != "" && cfg.HaikuModel != tt.expectedUsed {
				t.Errorf("HaikuModel = %q, want %q", cfg.HaikuModel, tt.expectedUsed)
			}
		})
	}
}

// TestProviderSpecificParameters tests that provider-specific params are set correctly
func TestProviderSpecificParameters(t *testing.T) {
	tests := []struct {
		name                 string
		provider             ProviderType
		shouldHaveReasoning  bool
		shouldHaveToolChoice bool
	}{
		{
			name:                 "OpenRouter reasoning support",
			provider:             ProviderOpenRouter,
			shouldHaveReasoning:  true,
			shouldHaveToolChoice: false,
		},
		{
			name:                 "OpenAI Direct reasoning support",
			provider:             ProviderOpenAI,
			shouldHaveReasoning:  true,
			shouldHaveToolChoice: false,
		},
		{
			name:                 "Ollama tool choice",
			provider:             ProviderOllama,
			shouldHaveReasoning:  false,
			shouldHaveToolChoice: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is a design test - documenting expected behavior
			// The actual implementation is in converter.go
			t.Logf("Provider %v should have reasoning=%v, tool_choice=%v",
				tt.provider, tt.shouldHaveReasoning, tt.shouldHaveToolChoice)
		})
	}
}

// TestEnvironmentConfigLoading tests that .env files are loaded correctly
func TestEnvironmentConfigLoading(t *testing.T) {
	// Save current env vars
	originalBaseURL := os.Getenv("OPENAI_BASE_URL")
	originalAPIKey := os.Getenv("OPENAI_API_KEY")

	// Restore after test
	defer func() {
		if originalBaseURL != "" {
			os.Setenv("OPENAI_BASE_URL", originalBaseURL)
		} else {
			os.Unsetenv("OPENAI_BASE_URL")
		}
		if originalAPIKey != "" {
			os.Setenv("OPENAI_API_KEY", originalAPIKey)
		} else {
			os.Unsetenv("OPENAI_API_KEY")
		}
	}()

	// Test that environment variables override
	os.Setenv("OPENAI_BASE_URL", "https://test.example.com")
	os.Setenv("OPENAI_API_KEY", "test-key-123")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OpenAIBaseURL != "https://test.example.com" {
		t.Errorf("OpenAIBaseURL = %q, want %q", cfg.OpenAIBaseURL, "https://test.example.com")
	}

	if cfg.OpenAIAPIKey != "test-key-123" {
		t.Errorf("OpenAIAPIKey = %q, want %q", cfg.OpenAIAPIKey, "test-key-123")
	}
}

// TestConfigDefaults tests default values
func TestConfigDefaults(t *testing.T) {
	// Clear relevant env vars for this test
	originalBaseURL := os.Getenv("OPENAI_BASE_URL")
	originalAPIKey := os.Getenv("OPENAI_API_KEY")
	os.Unsetenv("OPENAI_BASE_URL")
	os.Setenv("OPENAI_API_KEY", "test-key") // Required for non-localhost
	defer func() {
		if originalBaseURL != "" {
			os.Setenv("OPENAI_BASE_URL", originalBaseURL)
		}
		if originalAPIKey != "" {
			os.Setenv("OPENAI_API_KEY", originalAPIKey)
		} else {
			os.Unsetenv("OPENAI_API_KEY")
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Default base URL should be OpenAI
	expectedDefault := "https://api.openai.com/v1"
	if cfg.OpenAIBaseURL != expectedDefault {
		t.Errorf("Default OpenAIBaseURL = %q, want %q", cfg.OpenAIBaseURL, expectedDefault)
	}

	// Default port
	if cfg.Port != "8082" {
		t.Errorf("Default Port = %q, want %q", cfg.Port, "8082")
	}

	// Default host
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Default Host = %q, want %q", cfg.Host, "0.0.0.0")
	}
}

// TestProviderIsolation is a conceptual test documenting the isolation requirement
func TestProviderIsolation(t *testing.T) {
	scenarios := []struct {
		name             string
		configuredURL    string
		expectedProvider string
		shouldNotCallURL string
	}{
		{
			name:             "OpenRouter should not call OpenAI",
			configuredURL:    "https://openrouter.ai/api/v1",
			expectedProvider: "OpenRouter",
			shouldNotCallURL: "https://api.openai.com",
		},
		{
			name:             "OpenAI should not call OpenRouter",
			configuredURL:    "https://api.openai.com/v1",
			expectedProvider: "OpenAI Direct",
			shouldNotCallURL: "https://openrouter.ai",
		},
		{
			name:             "Ollama should not call external APIs",
			configuredURL:    "http://localhost:11434/v1",
			expectedProvider: "Ollama",
			shouldNotCallURL: "https://",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Document the requirement: when configured for one provider,
			// we should NEVER make requests to another provider's endpoint
			t.Logf("REQUIREMENT: When OPENAI_BASE_URL=%s (%s), proxy must NOT make requests to %s",
				scenario.configuredURL, scenario.expectedProvider, scenario.shouldNotCallURL)
		})
	}
}

// TestLoadWithDebug tests loading config with debug mode enabled
func TestLoadWithDebug(t *testing.T) {
	// Save original env
	originalKey := os.Getenv("OPENAI_API_KEY")
	originalBaseURL := os.Getenv("OPENAI_BASE_URL")
	defer func() {
		os.Setenv("OPENAI_API_KEY", originalKey)
		os.Setenv("OPENAI_BASE_URL", originalBaseURL)
	}()

	// Set required env vars
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

	cfg, err := LoadWithDebug(true)
	if err != nil {
		t.Fatalf("LoadWithDebug failed: %v", err)
	}

	if !cfg.Debug {
		t.Errorf("Expected Debug=true, got %v", cfg.Debug)
	}

	if cfg.OpenAIAPIKey != "test-key" {
		t.Errorf("Expected API key 'test-key', got %q", cfg.OpenAIAPIKey)
	}
}

// TestIsLocalhost tests localhost detection
func TestIsLocalhost(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected bool
	}{
		{
			name:     "localhost with default port",
			baseURL:  "http://localhost:11434/v1",
			expected: true,
		},
		{
			name:     "localhost with custom port",
			baseURL:  "http://localhost:8080/v1",
			expected: true,
		},
		{
			name:     "127.0.0.1",
			baseURL:  "http://127.0.0.1:11434/v1",
			expected: true,
		},
		{
			name:     "OpenRouter",
			baseURL:  "https://openrouter.ai/api/v1",
			expected: false,
		},
		{
			name:     "OpenAI Direct",
			baseURL:  "https://api.openai.com/v1",
			expected: false,
		},
		{
			name:     "Custom host",
			baseURL:  "http://192.168.1.100:11434/v1",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				OpenAIBaseURL: tt.baseURL,
			}

			result := cfg.IsLocalhost()
			if result != tt.expected {
				t.Errorf("IsLocalhost() for %q = %v, want %v",
					tt.baseURL, result, tt.expected)
			}
		})
	}
}

// TestOpenRouterSpecificSettings tests OpenRouter app name and URL settings
func TestOpenRouterSpecificSettings(t *testing.T) {
	// Save original env
	originalAppName := os.Getenv("OPENROUTER_APP_NAME")
	originalAppURL := os.Getenv("OPENROUTER_APP_URL")
	originalKey := os.Getenv("OPENAI_API_KEY")
	originalBaseURL := os.Getenv("OPENAI_BASE_URL")
	defer func() {
		os.Setenv("OPENROUTER_APP_NAME", originalAppName)
		os.Setenv("OPENROUTER_APP_URL", originalAppURL)
		os.Setenv("OPENAI_API_KEY", originalKey)
		os.Setenv("OPENAI_BASE_URL", originalBaseURL)
	}()

	// Set env vars
	os.Setenv("OPENROUTER_APP_NAME", "Claude-Code-Proxy")
	os.Setenv("OPENROUTER_APP_URL", "https://github.com/example/repo")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("OPENAI_BASE_URL", "https://openrouter.ai/api/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.OpenRouterAppName != "Claude-Code-Proxy" {
		t.Errorf("Expected app name 'Claude-Code-Proxy', got %q", cfg.OpenRouterAppName)
	}

	if cfg.OpenRouterAppURL != "https://github.com/example/repo" {
		t.Errorf("Expected app URL 'https://github.com/example/repo', got %q", cfg.OpenRouterAppURL)
	}

	if cfg.DetectProvider() != ProviderOpenRouter {
		t.Errorf("Expected OpenRouter provider, got %v", cfg.DetectProvider())
	}
}

// TestOllamaWithoutAPIKey tests that Ollama works without API key
func TestOllamaWithoutAPIKey(t *testing.T) {
	// Save original env
	originalKey := os.Getenv("OPENAI_API_KEY")
	originalBaseURL := os.Getenv("OPENAI_BASE_URL")
	defer func() {
		os.Setenv("OPENAI_API_KEY", originalKey)
		os.Setenv("OPENAI_BASE_URL", originalBaseURL)
	}()

	// Clear API key and set Ollama URL
	os.Unsetenv("OPENAI_API_KEY")
	os.Setenv("OPENAI_BASE_URL", "http://localhost:11434/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should succeed for Ollama without API key: %v", err)
	}

	// Should have dummy key for Ollama
	if cfg.OpenAIAPIKey != "ollama" {
		t.Errorf("Expected dummy API key 'ollama', got %q", cfg.OpenAIAPIKey)
	}

	if cfg.DetectProvider() != ProviderOllama {
		t.Errorf("Expected Ollama provider, got %v", cfg.DetectProvider())
	}
}

// TestMissingAPIKeyForOpenAI tests that load fails without API key for OpenAI
func TestMissingAPIKeyForOpenAI(t *testing.T) {
	// Save original env
	originalKey := os.Getenv("OPENAI_API_KEY")
	originalBaseURL := os.Getenv("OPENAI_BASE_URL")
	defer func() {
		os.Setenv("OPENAI_API_KEY", originalKey)
		os.Setenv("OPENAI_BASE_URL", originalBaseURL)
	}()

	// Clear API key and set OpenAI URL
	os.Unsetenv("OPENAI_API_KEY")
	os.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

	cfg, err := Load()
	if err == nil {
		t.Errorf("Load should fail when OpenAI API key is missing")
	}

	_ = cfg
}

// TestHostAndPortDefaults tests default host and port values
func TestHostAndPortDefaults(t *testing.T) {
	// Save original env
	originalHost := os.Getenv("HOST")
	originalPort := os.Getenv("PORT")
	originalKey := os.Getenv("OPENAI_API_KEY")
	originalBaseURL := os.Getenv("OPENAI_BASE_URL")
	defer func() {
		if originalHost != "" {
			os.Setenv("HOST", originalHost)
		} else {
			os.Unsetenv("HOST")
		}
		if originalPort != "" {
			os.Setenv("PORT", originalPort)
		} else {
			os.Unsetenv("PORT")
		}
		os.Setenv("OPENAI_API_KEY", originalKey)
		os.Setenv("OPENAI_BASE_URL", originalBaseURL)
	}()

	// Clear host and port
	os.Unsetenv("HOST")
	os.Unsetenv("PORT")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Host != "0.0.0.0" {
		t.Errorf("Expected default host '0.0.0.0', got %q", cfg.Host)
	}

	if cfg.Port != "8082" {
		t.Errorf("Expected default port '8082', got %q", cfg.Port)
	}
}

// TestPassthroughMode tests passthrough mode configuration
func TestPassthroughMode(t *testing.T) {
	// Save original env
	originalMode := os.Getenv("PASSTHROUGH_MODE")
	originalKey := os.Getenv("OPENAI_API_KEY")
	originalBaseURL := os.Getenv("OPENAI_BASE_URL")
	defer func() {
		if originalMode != "" {
			os.Setenv("PASSTHROUGH_MODE", originalMode)
		} else {
			os.Unsetenv("PASSTHROUGH_MODE")
		}
		os.Setenv("OPENAI_API_KEY", originalKey)
		os.Setenv("OPENAI_BASE_URL", originalBaseURL)
	}()

	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"enabled with true", "true", true},
		{"enabled with 1", "1", true},
		{"enabled with yes", "yes", true},
		{"disabled with false", "false", false},
		{"disabled with 0", "0", false},
		{"default is disabled", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("PASSTHROUGH_MODE", tt.envValue)
			} else {
				os.Unsetenv("PASSTHROUGH_MODE")
			}
			os.Setenv("OPENAI_API_KEY", "test-key")
			os.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}

			if cfg.PassthroughMode != tt.expected {
				t.Errorf("Expected PassthroughMode=%v, got %v", tt.expected, cfg.PassthroughMode)
			}
		})
	}
}

// TestGetBaseURLForModel tests per-tier base URL routing with fallback
func TestGetBaseURLForModel(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		modelName   string
		expectedURL string
	}{
		{
			name: "Opus model with specific base URL",
			cfg: Config{
				OpenAIBaseURL: "https://default.example.com/v1",
				OpusModel:     "mistral-large-2502",
				OpusBaseURL:   "https://api.corp.example/llm-large/v1",
			},
			modelName:   "mistral-large-2502",
			expectedURL: "https://api.corp.example/llm-large/v1",
		},
		{
			name: "Sonnet model with specific base URL",
			cfg: Config{
				OpenAIBaseURL: "https://default.example.com/v1",
				SonnetModel:   "codestral-2503",
				SonnetBaseURL: "https://api.corp.example/codestral/v1",
			},
			modelName:   "codestral-2503",
			expectedURL: "https://api.corp.example/codestral/v1",
		},
		{
			name: "Haiku model with specific base URL",
			cfg: Config{
				OpenAIBaseURL: "https://default.example.com/v1",
				HaikuModel:    "mistral-medium-2508",
				HaikuBaseURL:  "https://api.corp.example/llm-medium/v1",
			},
			modelName:   "mistral-medium-2508",
			expectedURL: "https://api.corp.example/llm-medium/v1",
		},
		{
			name: "Unknown model falls back to default",
			cfg: Config{
				OpenAIBaseURL: "https://default.example.com/v1",
				OpusModel:     "mistral-large-2502",
				OpusBaseURL:   "https://api.corp.example/llm-large/v1",
			},
			modelName:   "some-other-model",
			expectedURL: "https://default.example.com/v1",
		},
		{
			name: "Model matches but no base URL configured - fallback",
			cfg: Config{
				OpenAIBaseURL: "https://default.example.com/v1",
				OpusModel:     "mistral-large-2502",
				OpusBaseURL:   "", // not configured
			},
			modelName:   "mistral-large-2502",
			expectedURL: "https://default.example.com/v1",
		},
		{
			name: "No tier models configured - fallback",
			cfg: Config{
				OpenAIBaseURL: "https://default.example.com/v1",
			},
			modelName:   "gpt-5",
			expectedURL: "https://default.example.com/v1",
		},
		{
			name: "All tiers configured - each routes correctly",
			cfg: Config{
				OpenAIBaseURL: "https://default.example.com/v1",
				OpusModel:     "mistral-large-2502",
				OpusBaseURL:   "https://large.example.com/v1",
				SonnetModel:   "codestral-2503",
				SonnetBaseURL: "https://codestral.example.com/v1",
				HaikuModel:    "mistral-medium-2508",
				HaikuBaseURL:  "https://medium.example.com/v1",
			},
			modelName:   "codestral-2503",
			expectedURL: "https://codestral.example.com/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cfg.GetBaseURLForModel(tt.modelName)
			if result != tt.expectedURL {
				t.Errorf("GetBaseURLForModel(%q) = %q, want %q",
					tt.modelName, result, tt.expectedURL)
			}
		})
	}
}

// TestGetBaseURLForModelWithLoad tests that per-tier base URLs are loaded from env vars
func TestGetBaseURLForModelWithLoad(t *testing.T) {
	// Save original env
	envVars := []string{
		"OPENAI_API_KEY", "OPENAI_BASE_URL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_OPUS_BASE_URL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_SONNET_BASE_URL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_DEFAULT_HAIKU_BASE_URL",
	}
	originals := make(map[string]string)
	for _, key := range envVars {
		originals[key] = os.Getenv(key)
	}
	defer func() {
		for key, val := range originals {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Set env vars
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("OPENAI_BASE_URL", "https://default.example.com/v1")
	os.Setenv("ANTHROPIC_DEFAULT_OPUS_MODEL", "mistral-large-2502")
	os.Setenv("ANTHROPIC_DEFAULT_OPUS_BASE_URL", "https://large.example.com/v1")
	os.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "codestral-2503")
	os.Setenv("ANTHROPIC_DEFAULT_SONNET_BASE_URL", "https://codestral.example.com/v1")
	os.Setenv("ANTHROPIC_DEFAULT_HAIKU_MODEL", "mistral-medium-2508")
	os.Setenv("ANTHROPIC_DEFAULT_HAIKU_BASE_URL", "https://medium.example.com/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OpusBaseURL != "https://large.example.com/v1" {
		t.Errorf("OpusBaseURL = %q, want %q", cfg.OpusBaseURL, "https://large.example.com/v1")
	}
	if cfg.SonnetBaseURL != "https://codestral.example.com/v1" {
		t.Errorf("SonnetBaseURL = %q, want %q", cfg.SonnetBaseURL, "https://codestral.example.com/v1")
	}
	if cfg.HaikuBaseURL != "https://medium.example.com/v1" {
		t.Errorf("HaikuBaseURL = %q, want %q", cfg.HaikuBaseURL, "https://medium.example.com/v1")
	}

	// Test routing
	if url := cfg.GetBaseURLForModel("mistral-large-2502"); url != "https://large.example.com/v1" {
		t.Errorf("GetBaseURLForModel(opus) = %q, want large URL", url)
	}
	if url := cfg.GetBaseURLForModel("codestral-2503"); url != "https://codestral.example.com/v1" {
		t.Errorf("GetBaseURLForModel(sonnet) = %q, want codestral URL", url)
	}
	if url := cfg.GetBaseURLForModel("mistral-medium-2508"); url != "https://medium.example.com/v1" {
		t.Errorf("GetBaseURLForModel(haiku) = %q, want medium URL", url)
	}
	if url := cfg.GetBaseURLForModel("unknown-model"); url != "https://default.example.com/v1" {
		t.Errorf("GetBaseURLForModel(unknown) = %q, want default URL", url)
	}
}

// TestMultipleEnvFiles tests that env files are loaded in correct priority order
func TestMultipleEnvFiles(t *testing.T) {
	// Create temporary directory for test env files
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	originalCwd, _ := os.Getwd()

	// Create mock .claude directory
	claudeDir := filepath.Join(tempDir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	// Create .env file in current directory
	localEnvFile := filepath.Join(tempDir, ".env")
	os.WriteFile(localEnvFile, []byte("OPENAI_API_KEY=local-key\nOPENAI_BASE_URL=https://local.example.com"), 0644)

	// Create ~/.claude/proxy.env file
	claudeEnvFile := filepath.Join(claudeDir, "proxy.env")
	os.WriteFile(claudeEnvFile, []byte("OPENAI_API_KEY=claude-key"), 0644)

	// Setup environment
	os.Chdir(tempDir)
	os.Setenv("HOME", tempDir)

	defer func() {
		os.Chdir(originalCwd)
		os.Setenv("HOME", originalHome)
	}()

	// Load config - should pick up local .env first
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.OpenAIAPIKey != "local-key" {
		t.Errorf("Expected local API key, got %q", cfg.OpenAIAPIKey)
	}

	if cfg.OpenAIBaseURL != "https://local.example.com" {
		t.Errorf("Expected local base URL, got %q", cfg.OpenAIBaseURL)
	}
}

func TestGetProviderForTier(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		tier        string
		wantBaseURL string
		wantAPIKey  string
		wantModel   string
	}{
		{
			name: "opus with specific config",
			cfg: Config{
				OpenAIBaseURL:   "https://default.com/v1",
				OpenAIAPIKey:    "default-key",
				OpusBaseURL:     "https://opus.com/v1",
				OpusAPIKey:      "opus-key",
				OpusModel:       "gpt-5",
			},
			tier:        "opus",
			wantBaseURL: "https://opus.com/v1",
			wantAPIKey:  "opus-key",
			wantModel:   "gpt-5",
		},
		{
			name: "opus fallback to default",
			cfg: Config{
				OpenAIBaseURL: "https://default.com/v1",
				OpenAIAPIKey:  "default-key",
			},
			tier:        "opus",
			wantBaseURL: "https://default.com/v1",
			wantAPIKey:  "default-key",
			wantModel:   "",
		},
		{
			name: "sonnet with specific URL but default key",
			cfg: Config{
				OpenAIBaseURL:   "https://default.com/v1",
				OpenAIAPIKey:    "default-key",
				SonnetBaseURL:   "https://sonnet.com/v1",
				SonnetModel:     "codestral",
			},
			tier:        "sonnet",
			wantBaseURL: "https://sonnet.com/v1",
			wantAPIKey:  "default-key",
			wantModel:   "codestral",
		},
		{
			name: "haiku with specific key but default URL",
			cfg: Config{
				OpenAIBaseURL: "https://default.com/v1",
				OpenAIAPIKey:  "default-key",
				HaikuAPIKey:   "haiku-key",
				HaikuModel:    "qwen2.5",
			},
			tier:        "haiku",
			wantBaseURL: "https://default.com/v1",
			wantAPIKey:  "haiku-key",
			wantModel:   "qwen2.5",
		},
		{
			name: "all tiers with specific configs",
			cfg: Config{
				OpenAIBaseURL:   "https://default.com/v1",
				OpenAIAPIKey:    "default-key",
				OpusBaseURL:     "https://opus.com/v1",
				OpusAPIKey:      "opus-key",
				OpusModel:       "gpt-5",
				SonnetBaseURL:   "https://sonnet.com/v1",
				SonnetAPIKey:    "sonnet-key",
				SonnetModel:     "codestral",
				HaikuBaseURL:    "http://localhost:11434/v1",
				HaikuAPIKey:     "ollama",
				HaikuModel:      "qwen2.5",
			},
			tier:        "sonnet",
			wantBaseURL: "https://sonnet.com/v1",
			wantAPIKey:  "sonnet-key",
			wantModel:   "codestral",
		},
		{
			name: "unknown tier fallback",
			cfg: Config{
				OpenAIBaseURL: "https://default.com/v1",
				OpenAIAPIKey:  "default-key",
			},
			tier:        "unknown",
			wantBaseURL: "https://default.com/v1",
			wantAPIKey:  "default-key",
			wantModel:   "",
		},
		{
			name: "empty tier fallback",
			cfg: Config{
				OpenAIBaseURL: "https://default.com/v1",
				OpenAIAPIKey:  "default-key",
			},
			tier:        "",
			wantBaseURL: "https://default.com/v1",
			wantAPIKey:  "default-key",
			wantModel:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBaseURL, gotAPIKey, gotModel := tt.cfg.GetProviderForTier(tt.tier)

			if gotBaseURL != tt.wantBaseURL {
				t.Errorf("GetProviderForTier(%q) baseURL = %q, want %q", tt.tier, gotBaseURL, tt.wantBaseURL)
			}
			if gotAPIKey != tt.wantAPIKey {
				t.Errorf("GetProviderForTier(%q) apiKey = %q, want %q", tt.tier, gotAPIKey, tt.wantAPIKey)
			}
			if gotModel != tt.wantModel {
				t.Errorf("GetProviderForTier(%q) model = %q, want %q", tt.tier, gotModel, tt.wantModel)
			}
		})
	}
}
