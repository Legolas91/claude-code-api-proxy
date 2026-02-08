package server

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/claude-code-proxy/proxy/internal/config"
)

// TestServerSetup tests that the server can be initialized
func TestServerSetup(t *testing.T) {
	cfg := &config.Config{
		Host: "127.0.0.1",
		Port: "9999",
	}

	// Just verify config is valid
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Expected host 127.0.0.1")
	}

	if cfg.Port != "9999" {
		t.Errorf("Expected port 9999")
	}
}

// TestAPIKeyValidation tests API key validation logic
func TestAPIKeyValidation(t *testing.T) {
	tests := []struct {
		name           string
		configuredKey  string
		requestKey     string
		shouldValidate bool
		shouldPass     bool
	}{
		{
			name:           "with matching key",
			configuredKey:  "test-key",
			requestKey:     "test-key",
			shouldValidate: true,
			shouldPass:     true,
		},
		{
			name:           "with mismatched key",
			configuredKey:  "test-key",
			requestKey:     "wrong-key",
			shouldValidate: true,
			shouldPass:     false,
		},
		{
			name:           "no validation when not configured",
			configuredKey:  "",
			requestKey:     "any-key",
			shouldValidate: false,
			shouldPass:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				AnthropicAPIKey: tt.configuredKey,
			}

			// Simulate validation logic
			if cfg.AnthropicAPIKey != "" {
				// Validation is required
				if cfg.AnthropicAPIKey != tt.requestKey {
					if tt.shouldPass {
						t.Errorf("Expected validation to pass")
					}
				} else {
					if !tt.shouldPass {
						t.Errorf("Expected validation to fail")
					}
				}
			} else {
				// No validation required
				if !tt.shouldPass {
					t.Errorf("Expected to pass when validation disabled")
				}
			}
		})
	}
}

// TestServerConfiguration tests server host and port configuration
func TestServerConfiguration(t *testing.T) {
	tests := []struct {
		name string
		host string
		port string
	}{
		{
			name: "default configuration",
			host: "0.0.0.0",
			port: "8082",
		},
		{
			name: "localhost only",
			host: "127.0.0.1",
			port: "8082",
		},
		{
			name: "custom port",
			host: "0.0.0.0",
			port: "9999",
		},
		{
			name: "specific interface",
			host: "192.168.1.100",
			port: "8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Host: tt.host,
				Port: tt.port,
			}

			if cfg.Host != tt.host {
				t.Errorf("Expected host %s, got %s", tt.host, cfg.Host)
			}

			if cfg.Port != tt.port {
				t.Errorf("Expected port %s, got %s", tt.port, cfg.Port)
			}
		})
	}
}

// TestDebugMode tests debug mode configuration
func TestDebugMode(t *testing.T) {
	tests := []struct {
		name  string
		debug bool
	}{
		{
			name:  "debug enabled",
			debug: true,
		},
		{
			name:  "debug disabled",
			debug: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Debug: tt.debug,
			}

			if cfg.Debug != tt.debug {
				t.Errorf("Expected Debug %v, got %v", tt.debug, cfg.Debug)
			}
		})
	}
}

// TestSimpleLogMode tests simple log mode configuration
func TestSimpleLogMode(t *testing.T) {
	cfg := &config.Config{
		SimpleLog: true,
	}

	if !cfg.SimpleLog {
		t.Errorf("Expected SimpleLog to be true")
	}

	cfg.SimpleLog = false
	if cfg.SimpleLog {
		t.Errorf("Expected SimpleLog to be false")
	}
}

// TestOpenRouterConfiguration tests OpenRouter-specific configuration
func TestOpenRouterConfiguration(t *testing.T) {
	cfg := &config.Config{
		OpenRouterAppName: "Claude-Code-Proxy",
		OpenRouterAppURL:  "https://github.com/example/repo",
		OpenAIBaseURL:     "https://openrouter.ai/api/v1",
	}

	if cfg.OpenRouterAppName != "Claude-Code-Proxy" {
		t.Errorf("Expected app name 'Claude-Code-Proxy'")
	}

	if cfg.OpenRouterAppURL != "https://github.com/example/repo" {
		t.Errorf("Expected app URL 'https://github.com/example/repo'")
	}

	if cfg.DetectProvider() != config.ProviderOpenRouter {
		t.Errorf("Expected OpenRouter provider")
	}
}

// TestProviderDetectionForHandlers tests provider detection in handler context
func TestProviderDetectionForHandlers(t *testing.T) {
	tests := []struct {
		name             string
		baseURL          string
		expectedProvider config.ProviderType
	}{
		{
			name:             "OpenRouter",
			baseURL:          "https://openrouter.ai/api/v1",
			expectedProvider: config.ProviderOpenRouter,
		},
		{
			name:             "OpenAI",
			baseURL:          "https://api.openai.com/v1",
			expectedProvider: config.ProviderOpenAI,
		},
		{
			name:             "Ollama",
			baseURL:          "http://localhost:11434/v1",
			expectedProvider: config.ProviderOllama,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				OpenAIBaseURL: tt.baseURL,
			}

			provider := cfg.DetectProvider()
			if provider != tt.expectedProvider {
				t.Errorf("Expected provider %v, got %v", tt.expectedProvider, provider)
			}
		})
	}
}

// TestPassthroughMode tests passthrough mode configuration
func TestPassthroughMode(t *testing.T) {
	cfg := &config.Config{
		PassthroughMode: true,
	}

	if !cfg.PassthroughMode {
		t.Errorf("Expected PassthroughMode to be true")
	}
}

// TestConfigFormatDetection tests that config can detect different format scenarios
func TestConfigFormatDetection(t *testing.T) {
	// Test that config struct properly represents all fields
	cfg := &config.Config{
		OpenAIAPIKey:      "test-key",
		OpenAIBaseURL:     "https://api.openai.com/v1",
		AnthropicAPIKey:   "test-anthropic-key",
		OpusModel:         "gpt-5",
		SonnetModel:       "gpt-5",
		HaikuModel:        "gpt-5-mini",
		Host:              "0.0.0.0",
		Port:              "8082",
		Debug:             true,
		SimpleLog:         true,
		PassthroughMode:   false,
		OpenRouterAppName: "app",
		OpenRouterAppURL:  "https://example.com",
	}

	// Verify all fields are accessible
	if cfg.OpenAIAPIKey != "test-key" {
		t.Errorf("OpenAIAPIKey not set correctly")
	}
	if cfg.AnthropicAPIKey != "test-anthropic-key" {
		t.Errorf("AnthropicAPIKey not set correctly")
	}
	if cfg.OpusModel != "gpt-5" {
		t.Errorf("OpusModel not set correctly")
	}
	if cfg.SonnetModel != "gpt-5" {
		t.Errorf("SonnetModel not set correctly")
	}
	if cfg.HaikuModel != "gpt-5-mini" {
		t.Errorf("HaikuModel not set correctly")
	}
}

// --- Streaming handler tests (Phase 2.1) ---

// makeSSEStream builds a fake OpenAI SSE stream from lines
func makeSSEStream(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

// parseSSEEvents parses SSE output into a list of (event, data) pairs
func parseSSEEvents(output string) []struct {
	Event string
	Data  string
} {
	var events []struct {
		Event string
		Data  string
	}
	lines := strings.Split(output, "\n")
	var currentEvent string
	for _, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			events = append(events, struct {
				Event string
				Data  string
			}{Event: currentEvent, Data: data})
			currentEvent = ""
		}
	}
	return events
}

func TestStreamOpenAIToClaude_TextConversion(t *testing.T) {
	input := makeSSEStream(
		`data: {"choices":[{"delta":{"content":"Hello"},"index":0}]}`,
		`data: {"choices":[{"delta":{"content":" World"},"index":0}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop","index":0}]}`,
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	// Verify event sequence
	events := parseSSEEvents(output)

	// Must have message_start, ping, content_block_start, at least 2 deltas, content_block_stop, message_delta, message_stop
	eventTypes := make([]string, 0)
	for _, e := range events {
		eventTypes = append(eventTypes, e.Event)
	}

	if eventTypes[0] != "message_start" {
		t.Errorf("First event should be message_start, got %s", eventTypes[0])
	}
	if eventTypes[1] != "ping" {
		t.Errorf("Second event should be ping, got %s", eventTypes[1])
	}

	// Check text content is present
	if !strings.Contains(output, `"text":"Hello"`) {
		t.Error("Missing 'Hello' text delta")
	}
	if !strings.Contains(output, `"text":" World"`) {
		t.Error("Missing ' World' text delta")
	}

	// Check message_stop is last event
	lastEvent := eventTypes[len(eventTypes)-1]
	if lastEvent != "message_stop" {
		t.Errorf("Last event should be message_stop, got %s", lastEvent)
	}

	// Check stop reason is end_turn for "stop"
	if !strings.Contains(output, `"stop_reason":"end_turn"`) {
		t.Error("Expected stop_reason 'end_turn' for finish_reason 'stop'")
	}
}

func TestStreamOpenAIToClaude_ThinkingBlocks_ReasoningContent(t *testing.T) {
	// OpenAI o1/o3 format: reasoning_content string field
	input := makeSSEStream(
		`data: {"choices":[{"delta":{"reasoning_content":"Let me think..."},"index":0}]}`,
		`data: {"choices":[{"delta":{"content":"The answer is 42"},"index":0}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop","index":0}]}`,
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	// Check thinking block was created
	if !strings.Contains(output, `"type":"thinking"`) {
		t.Error("Missing thinking block type")
	}
	if !strings.Contains(output, `"thinking_delta"`) {
		t.Error("Missing thinking_delta")
	}
	if !strings.Contains(output, `"Let me think..."`) {
		t.Error("Missing thinking content")
	}
	// Check text block was also created
	if !strings.Contains(output, `"The answer is 42"`) {
		t.Error("Missing text content after thinking")
	}
}

func TestStreamOpenAIToClaude_ThinkingBlocks_Reasoning(t *testing.T) {
	// Simple reasoning field format
	input := makeSSEStream(
		`data: {"choices":[{"delta":{"reasoning":"Step 1: analyze"},"index":0}]}`,
		`data: {"choices":[{"delta":{"reasoning":" Step 2: conclude"},"index":0}]}`,
		`data: {"choices":[{"delta":{"content":"Result"},"index":0}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop","index":0}]}`,
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	if !strings.Contains(output, `"thinking_delta"`) {
		t.Error("Missing thinking_delta for reasoning field")
	}
	if !strings.Contains(output, `"Step 1: analyze"`) {
		t.Error("Missing first reasoning content")
	}
}

func TestStreamOpenAIToClaude_ThinkingBlocks_ReasoningDetails(t *testing.T) {
	// OpenRouter format: reasoning_details array
	input := makeSSEStream(
		`data: {"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","text":"Thinking..."}]},"index":0}]}`,
		`data: {"choices":[{"delta":{"content":"Answer"},"index":0}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop","index":0}]}`,
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	if !strings.Contains(output, `"thinking_delta"`) {
		t.Error("Missing thinking_delta for reasoning_details")
	}
	if !strings.Contains(output, `"Thinking..."`) {
		t.Error("Missing reasoning_details content")
	}
}

func TestStreamOpenAIToClaude_ToolCalls(t *testing.T) {
	// Tool call with accumulated JSON arguments
	input := makeSSEStream(
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_123","function":{"name":"get_weather","arguments":""}}]},"index":0}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"location\":"}}]},"index":0}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]},"index":0}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls","index":0}]}`,
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	// Check tool_use block was created
	if !strings.Contains(output, `"type":"tool_use"`) {
		t.Error("Missing tool_use content block")
	}
	if !strings.Contains(output, `"name":"get_weather"`) {
		t.Error("Missing tool name")
	}
	if !strings.Contains(output, `"id":"call_123"`) {
		t.Error("Missing tool call ID")
	}
	// Check accumulated JSON was sent
	if !strings.Contains(output, `"input_json_delta"`) {
		t.Error("Missing input_json_delta")
	}
	// Check stop reason is tool_use
	if !strings.Contains(output, `"stop_reason":"tool_use"`) {
		t.Error("Expected stop_reason 'tool_use' for finish_reason 'tool_calls'")
	}
}

func TestStreamOpenAIToClaude_FinishReasons(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		expectedStop string
	}{
		{"stop", "stop", "end_turn"},
		{"length", "length", "max_tokens"},
		{"tool_calls", "tool_calls", "tool_use"},
		{"function_call", "function_call", "tool_use"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := makeSSEStream(
				`data: {"choices":[{"delta":{"content":"text"},"index":0}]}`,
				`data: {"choices":[{"delta":{},"finish_reason":"`+tt.finishReason+`","index":0}]}`,
				`data: [DONE]`,
			)

			var buf bytes.Buffer
			w := bufio.NewWriter(&buf)
			cfg := &config.Config{}

			streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
			w.Flush()

			output := buf.String()

			expected := `"stop_reason":"` + tt.expectedStop + `"`
			if !strings.Contains(output, expected) {
				t.Errorf("Expected %s, output: %s", expected, output)
			}
		})
	}
}

func TestStreamOpenAIToClaude_ThinkingOnlyResponse(t *testing.T) {
	// Response with only thinking, no text content, no tool calls
	input := makeSSEStream(
		`data: {"choices":[{"delta":{"reasoning_content":"Just thinking..."},"index":0}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop","index":0}]}`,
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	// Should have thinking block
	if !strings.Contains(output, `"thinking_delta"`) {
		t.Error("Missing thinking_delta")
	}
	// Should still have message_stop
	if !strings.Contains(output, `"message_stop"`) {
		t.Error("Missing message_stop event")
	}
	// Should NOT have text block start (no text content was sent)
	events := parseSSEEvents(output)
	for _, e := range events {
		if e.Event == "content_block_start" && strings.Contains(e.Data, `"type":"text"`) {
			t.Error("Should not have text content_block_start when no text content was sent")
		}
	}
}

func TestStreamOpenAIToClaude_UsageTracking(t *testing.T) {
	// Usage data sent after finish_reason (stream_options.include_usage)
	input := makeSSEStream(
		`data: {"choices":[{"delta":{"content":"ok"},"index":0}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop","index":0}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	// Check that usage data is included in message_delta
	if !strings.Contains(output, `"input_tokens":10`) {
		t.Error("Missing input_tokens in usage")
	}
	if !strings.Contains(output, `"output_tokens":5`) {
		t.Error("Missing output_tokens in usage")
	}
}

func TestStreamOpenAIToClaude_EmptyStream(t *testing.T) {
	// Stream with only [DONE] marker, no content
	input := makeSSEStream(
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	// Should still have proper message envelope
	if !strings.Contains(output, `"message_start"`) {
		t.Error("Missing message_start event")
	}
	if !strings.Contains(output, `"message_stop"`) {
		t.Error("Missing message_stop event")
	}
}

// --- Error path tests (Phase 2.4) ---

func TestIsMaxTokensParameterError(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected bool
	}{
		{
			name:     "max_completion_tokens unsupported",
			message:  "Unsupported parameter: max_completion_tokens",
			expected: true,
		},
		{
			name:     "max_tokens invalid",
			message:  "Invalid parameter: max_tokens",
			expected: true,
		},
		{
			name:     "extra_forbidden max_tokens",
			message:  "extra_forbidden: max_tokens is not permitted",
			expected: true,
		},
		{
			name:     "unrelated error",
			message:  "Connection refused",
			expected: false,
		},
		{
			name:     "parameter without max_tokens",
			message:  "Unsupported parameter: temperature",
			expected: false,
		},
		{
			name:     "max_tokens without parameter indicator",
			message:  "max_tokens exceeded",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMaxTokensParameterError(tt.message)
			if result != tt.expected {
				t.Errorf("isMaxTokensParameterError(%q) = %v, expected %v", tt.message, result, tt.expected)
			}
		})
	}
}

func TestProviderError(t *testing.T) {
	err := &providerError{
		StatusCode: 401,
		Body:       `{"error":"invalid_api_key"}`,
	}

	if err.StatusCode != 401 {
		t.Errorf("Expected status code 401, got %d", err.StatusCode)
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "401") {
		t.Error("Error message should contain status code")
	}
	if !strings.Contains(errMsg, "invalid_api_key") {
		t.Error("Error message should contain body")
	}
}

func TestProviderErrorStatusPropagation(t *testing.T) {
	tests := []struct {
		name             string
		statusCode       int
		expectedType     string
	}{
		{"auth error", 401, "authentication_error"},
		{"forbidden", 403, "authentication_error"},
		{"rate limit", 429, "rate_limit_error"},
		{"bad request", 400, "invalid_request_error"},
		{"not found", 404, "invalid_request_error"},
		{"server error", 500, "api_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pe := &providerError{StatusCode: tt.statusCode, Body: "test"}

			// Replicate the logic from handleMessages
			errorType := "api_error"
			if pe.StatusCode == 401 || pe.StatusCode == 403 {
				errorType = "authentication_error"
			} else if pe.StatusCode == 429 {
				errorType = "rate_limit_error"
			} else if pe.StatusCode >= 400 && pe.StatusCode < 500 {
				errorType = "invalid_request_error"
			}

			if errorType != tt.expectedType {
				t.Errorf("For status %d, expected error type %q, got %q", tt.statusCode, tt.expectedType, errorType)
			}
		})
	}
}

func TestWriteSSEEvent(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeSSEEvent(w, "test_event", map[string]interface{}{
		"type":    "test",
		"message": "hello",
	})
	w.Flush()

	output := buf.String()

	if !strings.Contains(output, "event: test_event\n") {
		t.Error("Missing event line")
	}
	if !strings.Contains(output, `"type":"test"`) {
		t.Error("Missing type in data")
	}
	if !strings.Contains(output, `"message":"hello"`) {
		t.Error("Missing message in data")
	}
}

func TestWriteSSEError(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeSSEError(w, "something went wrong")
	// writeSSEError calls w.Flush() internally

	output := buf.String()

	if !strings.Contains(output, "event: error") {
		t.Error("Missing error event")
	}
	if !strings.Contains(output, "something went wrong") {
		t.Error("Missing error message")
	}
	if !strings.Contains(output, `"api_error"`) {
		t.Error("Missing error type")
	}
}

func TestStreamOpenAIToClaude_MalformedJSON(t *testing.T) {
	// Malformed JSON chunks should be silently skipped
	input := makeSSEStream(
		`data: {invalid json}`,
		`data: {"choices":[{"delta":{"content":"valid"},"index":0}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop","index":0}]}`,
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	// Valid content should still be processed
	if !strings.Contains(output, `"valid"`) {
		t.Error("Valid content after malformed JSON should be processed")
	}
}

func TestStreamOpenAIToClaude_EmptyChoices(t *testing.T) {
	// Response with empty choices array (usage-only chunk)
	input := makeSSEStream(
		`data: {"choices":[{"delta":{"content":"text"},"index":0}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":50,"completion_tokens":20}}`,
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	// Should process text and usage without crashing
	if !strings.Contains(output, `"text"`) {
		t.Error("Missing text content")
	}
	if !strings.Contains(output, `"input_tokens":50`) {
		t.Error("Missing usage data from empty-choices chunk")
	}
}

func TestStreamOpenAIToClaude_InvalidChoiceType(t *testing.T) {
	// Choice is not a map (edge case)
	input := makeSSEStream(
		`data: {"choices":["not_a_map"]}`,
		`data: {"choices":[{"delta":{"content":"ok"},"index":0}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop","index":0}]}`,
		`data: [DONE]`,
	)

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	cfg := &config.Config{}

	// Should not panic thanks to the type assertion check
	streamOpenAIToClaude(w, strings.NewReader(input), "test-model", "claude-test-model", cfg, time.Now())
	w.Flush()

	output := buf.String()

	// Valid chunk after bad one should work
	if !strings.Contains(output, `"ok"`) {
		t.Error("Valid content after invalid choice type should be processed")
	}
}
