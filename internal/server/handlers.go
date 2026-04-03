package server

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/claude-code-proxy/proxy/internal/cache"
	"github.com/claude-code-proxy/proxy/internal/config"
	"github.com/claude-code-proxy/proxy/internal/converter"
	"github.com/claude-code-proxy/proxy/internal/loop"
	"github.com/claude-code-proxy/proxy/pkg/models"
	"github.com/gofiber/fiber/v2"
)

// redactSensitiveData removes or truncates sensitive fields from request/response bodies
// to prevent API keys, credentials, and large prompts from being logged.
func redactSensitiveData(body []byte) string {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "[invalid JSON]"
	}

	// Redact sensitive authentication fields
	sensitiveFields := []string{"api_key", "x-api-key", "authorization", "bearer", "token"}
	for _, field := range sensitiveFields {
		if _, ok := data[field]; ok {
			data[field] = "[REDACTED]"
		}
	}

	// Truncate long system prompts (keep first 200 chars for debugging)
	if system, ok := data["system"].(string); ok && len(system) > 200 {
		data["system"] = system[:200] + "... [TRUNCATED " + fmt.Sprintf("%d", len(system)-200) + " chars]"
	}

	// Truncate long message arrays (keep structure but limit content)
	if messages, ok := data["messages"].([]interface{}); ok && len(messages) > 3 {
		truncatedMessages := make([]interface{}, 3)
		copy(truncatedMessages, messages[:3])
		data["messages"] = append(truncatedMessages, map[string]string{
			"note": fmt.Sprintf("[%d more messages truncated]", len(messages)-3),
		})
	}

	redacted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "[error redacting data]"
	}
	return string(redacted)
}

// addOpenRouterHeaders adds OpenRouter-specific HTTP headers for better rate limits.
// Sets HTTP-Referer and X-Title headers when configured, which helps with OpenRouter's
// rate limiting and usage tracking.
func addOpenRouterHeaders(req *http.Request, cfg *config.Config) {
	if cfg.OpenRouterAppURL != "" {
		req.Header.Set("HTTP-Referer", cfg.OpenRouterAppURL)
	}
	if cfg.OpenRouterAppName != "" {
		req.Header.Set("X-Title", cfg.OpenRouterAppName)
	}
}

// handleMessages is the main handler for /v1/messages endpoint.
// It parses Claude requests, converts them to OpenAI format, and routes to either
// streaming or non-streaming handlers based on the request's stream parameter.
func handleMessages(c *fiber.Ctx, cfg *config.Config, responseCache cache.Store) error {
	// Debug: Log request (with sensitive data redacted)
	if cfg.Debug {
		fmt.Printf("\n=== CLAUDE REQUEST (redacted) ===\n%s\n==================================\n", redactSensitiveData(c.Body()))
	}

	// Parse Claude request
	var claudeReq models.ClaudeRequest
	if err := c.BodyParser(&claudeReq); err != nil {
		// Log the error with redacted body for debugging
		fmt.Printf("[ERROR] Failed to parse request body: %v\n", err)
		fmt.Printf("[ERROR] Raw body (redacted): %s\n", redactSensitiveData(c.Body()))
		return c.Status(400).JSON(fiber.Map{
			"type": "error",
			"error": fiber.Map{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid request body: %v", err),
			},
		})
	}

	// Validate required fields
	if claudeReq.Model == "" {
		return c.Status(400).JSON(fiber.Map{
			"type": "error",
			"error": fiber.Map{
				"type":    "invalid_request_error",
				"message": "Missing required field: model",
			},
		})
	}

	if len(claudeReq.Messages) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"type": "error",
			"error": fiber.Map{
				"type":    "invalid_request_error",
				"message": "Missing required field: messages (must contain at least 1 message)",
			},
		})
	}

	// Validate field ranges
	if claudeReq.MaxTokens < 0 || claudeReq.MaxTokens > 200000 {
		return c.Status(400).JSON(fiber.Map{
			"type": "error",
			"error": fiber.Map{
				"type":    "invalid_request_error",
				"message": "max_tokens must be between 0 and 200000",
			},
		})
	}

	if claudeReq.Temperature != nil {
		temp := *claudeReq.Temperature
		if temp < 0.0 || temp > 2.0 {
			return c.Status(400).JSON(fiber.Map{
				"type": "error",
				"error": fiber.Map{
					"type":    "invalid_request_error",
					"message": "temperature must be between 0.0 and 2.0",
				},
			})
		}
	}

	// Validate API key (if configured) using constant-time comparison to prevent timing attacks
	if cfg.AnthropicAPIKey != "" {
		apiKey := c.Get("x-api-key")
		if subtle.ConstantTimeCompare([]byte(apiKey), []byte(cfg.AnthropicAPIKey)) != 1 {
			return c.Status(401).JSON(fiber.Map{
				"type": "error",
				"error": fiber.Map{
					"type":    "authentication_error",
					"message": "Invalid API key",
				},
			})
		}
	}

	// Detect and break infinite tool-call retry loops with escalating severity
	if cfg.MaxIdenticalRetries > 0 {
		if level := loop.GetLoopLevel(claudeReq.Messages, cfg.MaxIdenticalRetries, cfg.MaxLoopLevel); level > 0 {
			claudeReq.Messages = loop.InjectLoopBreakerLevel(claudeReq.Messages, level)
			if level >= 3 {
				// Disable tools to force a text-based approach after persistent looping
				claudeReq.Tools = nil
				claudeReq.ToolChoice = nil
			}
			if cfg.Debug {
				suffix := ""
				if level >= 3 {
					suffix = " + disabling tools"
				}
				fmt.Printf("[DEBUG] Retry loop detected (level %d), injecting nudge%s\n", level, suffix)
			}
			if cfg.SimpleLog {
				timestamp := time.Now().Format("15:04:05")
				if level >= 3 {
					fmt.Printf("[%s] [LOOP] Persistent loop (level %d) — disabling tools\n", timestamp, level)
				} else {
					fmt.Printf("[%s] [LOOP] Retry loop detected (level %d) — injecting nudge\n", timestamp, level)
				}
			}
		}
	}

	// Response cache: check for a cached response (non-streaming only)
	isStreaming := claudeReq.Stream != nil && *claudeReq.Stream
	var cacheKey string
	if responseCache != nil && !isStreaming && isCacheable(claudeReq, cfg.CacheMaxTemperature) {
		cacheKey = cache.ComputeKey(claudeReq)
		if cached := responseCache.Get(cacheKey); cached != nil {
			c.Set("X-Cache", "HIT")
			if cfg.SimpleLog {
				timestamp := time.Now().Format("15:04:05")
				fmt.Printf("[%s] [CACHE] HIT key=%s model=%s\n", timestamp, cacheKey[:12], claudeReq.Model)
			}
			return c.JSON(cached)
		}
		c.Set("X-Cache", "MISS")
	}

	// Get provider configuration for this tier (multi-URL routing support)
	tier := converter.GetTierFromModel(claudeReq.Model)
	baseURL, apiKey, _ := cfg.GetProviderForTier(tier)

	// Detect per-tier provider type — route to claude-p if api.anthropic.com without API key
	tierProvider := config.DetectProviderForURL(baseURL, apiKey)
	if tierProvider == config.ProviderClaudeCode {
		if cfg.Debug {
			fmt.Printf("[DEBUG] Tier %s → ProviderClaudeCode (no API key for %s)\n", tier, baseURL)
		}
		if cfg.SimpleLog {
			timestamp := time.Now().Format("15:04:05")
			fmt.Printf("[%s] [ROUTE] %s → claude-p (model=%s)\n", timestamp, tier, claudeReq.Model)
		}
		return handleClaudeCodeMessages(c, claudeReq, cfg, cacheKey, responseCache)
	}

	// Convert Claude request to OpenAI format
	openaiReq, err := converter.ConvertRequest(claudeReq, cfg)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"type": "error",
			"error": fiber.Map{
				"type":    "invalid_request_error",
				"message": err.Error(),
			},
		})
	}

	// Debug: Log converted OpenAI request
	if cfg.Debug {
		openaiReqJSON, _ := json.MarshalIndent(openaiReq, "", "  ")
		fmt.Printf("\n=== OPENAI REQUEST ===\n%s\n===================\n", string(openaiReqJSON))
		if len(claudeReq.Tools) > 0 {
			fmt.Printf("[DEBUG] Request has %d tools\n", len(claudeReq.Tools))
			for i, tool := range openaiReq.Tools {
				fmt.Printf("[DEBUG] Tool %d: %s\n", i, tool.Function.Name)
			}
		}
	}

	// Debug: Check Stream field
	if cfg.Debug {
		if openaiReq.Stream == nil {
			fmt.Printf("[DEBUG] Stream field is nil\n")
		} else {
			fmt.Printf("[DEBUG] Stream field = %v\n", *openaiReq.Stream)
		}
	}

	// Handle streaming vs non-streaming
	if openaiReq.Stream != nil && *openaiReq.Stream {
		return handleStreamingMessages(c, openaiReq, claudeReq.Model, cfg, baseURL, apiKey)
	}

	// Track timing for simple log
	startTime := time.Now()

	// Non-streaming response
	openaiResp, err := callOpenAI(openaiReq, cfg, baseURL, apiKey)
	if err != nil {
		// Propagate the provider's HTTP status code when available
		statusCode := 500
		errorType := "api_error"
		if pe, ok := err.(*providerError); ok {
			statusCode = pe.StatusCode
			if statusCode == 401 || statusCode == 403 {
				errorType = "authentication_error"
			} else if statusCode == 429 {
				errorType = "rate_limit_error"
			} else if statusCode >= 400 && statusCode < 500 {
				errorType = "invalid_request_error"
			}
		}
		return c.Status(statusCode).JSON(fiber.Map{
			"type": "error",
			"error": fiber.Map{
				"type":    errorType,
				"message": fmt.Sprintf("OpenAI API error: %v", err),
			},
		})
	}

	// Debug: Log OpenAI response
	if cfg.Debug {
		openaiRespJSON, _ := json.MarshalIndent(openaiResp, "", "  ")
		fmt.Printf("\n=== OPENAI RESPONSE ===\n%s\n====================\n", string(openaiRespJSON))
		if len(openaiResp.Choices) > 0 {
			choice := openaiResp.Choices[0]
			fmt.Printf("[DEBUG] OpenAI response has %d tool_calls\n", len(choice.Message.ToolCalls))
			for i, tc := range choice.Message.ToolCalls {
				fmt.Printf("[DEBUG] ToolCall %d: ID=%s, Name=%s\n", i, tc.ID, tc.Function.Name)
			}
		}
	}

	// Repair malformed tool call arguments from backends that don't follow OpenAI format strictly
	if cfg.RepairToolCalls && converter.RepairToolCalls(openaiResp) {
		if cfg.Debug {
			fmt.Printf("[DEBUG] Repaired malformed tool call arguments from model %s\n", openaiReq.Model)
		}
		if cfg.SimpleLog {
			timestamp := time.Now().Format("15:04:05")
			fmt.Printf("[%s] [WARN] Repaired malformed tool call arguments (model=%s)\n", timestamp, openaiReq.Model)
		}
	}

	// Convert OpenAI response to Claude format
	claudeResp, err := converter.ConvertResponse(openaiResp, claudeReq.Model)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"type": "error",
			"error": fiber.Map{
				"type":    "api_error",
				"message": fmt.Sprintf("Response conversion error: %v", err),
			},
		})
	}

	// Debug: Log Claude response
	if cfg.Debug {
		claudeRespJSON, _ := json.MarshalIndent(claudeResp, "", "  ")
		fmt.Printf("\n=== CLAUDE RESPONSE ===\n%s\n====================\n\n", string(claudeRespJSON))
		fmt.Printf("[DEBUG] Claude response has %d content blocks\n", len(claudeResp.Content))
		for i, block := range claudeResp.Content {
			fmt.Printf("[DEBUG] Block %d: type=%s", i, block.Type)
			if block.Type == "tool_use" {
				fmt.Printf(", name=%s, id=%s", block.Name, block.ID)
			}
			fmt.Printf("\n")
		}
	}

	// Simple log: one-line summary
	if cfg.SimpleLog {
		duration := time.Since(startTime).Seconds()
		tokensPerSec := 0.0
		if duration > 0 && claudeResp.Usage.OutputTokens > 0 {
			tokensPerSec = float64(claudeResp.Usage.OutputTokens) / duration
		}
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] [REQ] %s model=%s in=%d out=%d tok/s=%.1f\n",
			timestamp,
			cfg.GetBaseURLForModel(openaiReq.Model),
			openaiReq.Model,
			claudeResp.Usage.InputTokens,
			claudeResp.Usage.OutputTokens,
			tokensPerSec)

		// Tool telemetry: log which tools were used
		if len(openaiReq.Tools) > 0 {
			var usedNames []string
			for _, block := range claudeResp.Content {
				if block.Type == "tool_use" {
					usedNames = append(usedNames, block.Name)
				}
			}
			if len(usedNames) > 0 {
				fmt.Printf("[%s] [TOOL] model=%s sent=%d used=%d name=%s\n",
					timestamp,
					openaiReq.Model,
					len(openaiReq.Tools),
					len(usedNames),
					strings.Join(usedNames, ","))
			}
		}
	}

	// Store response in cache
	if cacheKey != "" && responseCache != nil {
		responseCache.Set(cacheKey, claudeResp)
		if cfg.SimpleLog {
			timestamp := time.Now().Format("15:04:05")
			fmt.Printf("[%s] [CACHE] STORE key=%s model=%s entries=%d\n", timestamp, cacheKey[:12], claudeReq.Model, responseCache.Len())
		}
	}

	return c.JSON(claudeResp)
}

// handleStreamingMessages handles streaming SSE responses from the provider.
// It forwards the OpenAI request, receives streaming chunks, and converts them to
// Claude's SSE event format in real-time using streamOpenAIToClaude.
func handleStreamingMessages(c *fiber.Ctx, openaiReq *models.OpenAIRequest, claudeModel string, cfg *config.Config, baseURL, apiKey string) error {
	// Track timing for simple log
	startTime := time.Now()

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		if cfg.Debug {
			fmt.Printf("[DEBUG] StreamWriter: Starting\n")
		}

		if cfg.Debug {
			fmt.Printf("[DEBUG] StreamWriter: Making streaming request to %s\n", baseURL+"/chat/completions")
		}

		// Make streaming request with automatic retry logic
		resp, err := callOpenAIStream(openaiReq, cfg, baseURL, apiKey)
		if err != nil {
			if cfg.Debug {
				fmt.Printf("[DEBUG] StreamWriter: Request failed: %v\n", err)
			}
			writeSSEError(w, fmt.Sprintf("streaming request failed: %v", err))
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if cfg.Debug {
			fmt.Printf("[DEBUG] StreamWriter: Got response, starting streamOpenAIToClaude conversion\n")
		}

		// Stream conversion
		streamOpenAIToClaude(w, resp.Body, openaiReq.Model, claudeModel, cfg, startTime, len(openaiReq.Tools))

		if cfg.Debug {
			fmt.Printf("[DEBUG] StreamWriter: Completed\n")
		}
	})

	return nil
}

// ToolCallState tracks the state of a tool call during streaming
type ToolCallState struct {
	ID          string // Tool call ID from OpenAI
	Name        string // Function name
	ArgsBuffer  string // Accumulated JSON arguments
	JSONSent    bool   // Flag if we sent the JSON delta
	ClaudeIndex int    // The content block index for Claude
	Started     bool   // Flag if content_block_start was sent
}

// thinkingBlockState tracks the state of thinking/reasoning blocks during streaming.
type thinkingBlockState struct {
	index      int
	started    bool
	hasContent bool
}

// emitThinkingDelta sends a thinking block start (on first call) and a thinking_delta event.
// The deltaField is the JSON field name for the thinking text ("text" for reasoning_content, "thinking" for others).
func (s *thinkingBlockState) emitThinkingDelta(w *bufio.Writer, text string, deltaField string) {
	if !s.started {
		contentBlock := map[string]interface{}{"type": "thinking"}
		if deltaField == "thinking" {
			contentBlock["thinking"] = ""
		}
		writeSSEEvent(w, "content_block_start", map[string]interface{}{
			"type":          "content_block_start",
			"index":         s.index,
			"content_block": contentBlock,
		})
		s.started = true
		_ = w.Flush()
	}

	writeSSEEvent(w, "content_block_delta", map[string]interface{}{
		"type":  "content_block_delta",
		"index": s.index,
		"delta": map[string]interface{}{
			"type":     "thinking_delta",
			deltaField: text,
		},
	})
	s.hasContent = true
	_ = w.Flush()
}

// streamOpenAIToClaude converts OpenAI streaming responses to Claude's SSE event format.
//
// It processes the OpenAI SSE stream chunk-by-chunk, generating the proper sequence of
// Claude events: message_start, content_block_start, content_block_delta, content_block_stop,
// message_delta, and message_stop.
//
// Handles:
//   - Thinking blocks from reasoning models (OpenRouter's reasoning_details, OpenAI's reasoning_content)
//   - Text content deltas
//   - Tool call deltas (accumulates JSON arguments across chunks)
//   - Token usage tracking and throughput calculation for simple log mode
//
// The function maintains state to track content block indices, tool call accumulation,
// and ensures proper event ordering for Claude Code compatibility.
func streamOpenAIToClaude(w *bufio.Writer, reader io.Reader, providerModel string, claudeModel string, cfg *config.Config, startTime time.Time, toolsSent int) {
	if cfg.Debug {
		fmt.Printf("[DEBUG] streamOpenAIToClaude: Starting conversion\n")
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // Increase buffer size

	// State variables
	messageID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	textBlockIndex := 1                              // Text block is index 1 (thinking is 0)
	toolBlockCounter := 2                            // Tool calls start at index 2
	currentToolCalls := make(map[int]*ToolCallState)
	finalStopReason := "end_turn"
	usageData := map[string]interface{}{
		"input_tokens":                0,
		"output_tokens":               0,
		"cache_creation_input_tokens": 0,
		"cache_read_input_tokens":     0,
		"cache_creation": map[string]interface{}{
			"ephemeral_5m_input_tokens": 0,
			"ephemeral_1h_input_tokens": 0,
		},
	}

	// Thinking block tracking (to show thinking indicator in Claude Code)
	thinking := &thinkingBlockState{index: 0}
	textBlockStarted := false // Track if we've sent text block_start

	// Send initial SSE events
	writeSSEEvent(w, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         claudeModel,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":                0,
				"output_tokens":               0,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens":     0,
				"cache_creation": map[string]interface{}{
					"ephemeral_5m_input_tokens": 0,
					"ephemeral_1h_input_tokens": 0,
				},
			},
		},
	})

	writeSSEEvent(w, "ping", map[string]interface{}{
		"type": "ping",
	})

	_ = w.Flush()

	// Process streaming chunks
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Check for [DONE] marker
		if strings.Contains(line, "[DONE]") {
			break
		}

		// Parse data line
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataJSON := strings.TrimPrefix(line, "data: ")

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(dataJSON), &chunk); err != nil {
			continue
		}

		// Log every chunk to see what OpenRouter is sending
		if cfg.Debug {
			fmt.Printf("[DEBUG] Raw chunk from OpenRouter: %s\n", dataJSON)
		}

		// Handle usage data
		if usage, ok := chunk["usage"].(map[string]interface{}); ok {
			if cfg.Debug {
				usageJSON, _ := json.Marshal(usage)
				fmt.Printf("[DEBUG] Received usage from OpenAI: %s\n", string(usageJSON))
			}

			// Convert float64 to int for token counts (JSON unmarshals numbers as float64)
			inputTokens := 0
			outputTokens := 0
			if val, ok := usage["prompt_tokens"].(float64); ok {
				inputTokens = int(val)
			}
			if val, ok := usage["completion_tokens"].(float64); ok {
				outputTokens = int(val)
			}

			usageData = map[string]interface{}{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
			}

			// Add cache metrics if present
			if promptTokensDetails, ok := usage["prompt_tokens_details"].(map[string]interface{}); ok {
				if cachedTokens, ok := promptTokensDetails["cached_tokens"].(float64); ok && cachedTokens > 0 {
					usageData["cache_read_input_tokens"] = int(cachedTokens)
				}
			}
			if cfg.Debug {
				usageDataJSON, _ := json.Marshal(usageData)
				fmt.Printf("[DEBUG] Accumulated usageData: %s\n", string(usageDataJSON))
			}
		}

		// Extract delta from choices
		choices, ok := chunk["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}
		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		// Handle reasoning delta (thinking blocks)
		// Support both OpenRouter and OpenAI formats:
		// - OpenRouter: delta.reasoning_details (array)
		// - OpenAI o1/o3: delta.reasoning_content (string)

		// First, check for OpenAI's reasoning_content format (o1/o3 models)
		if reasoningContent, ok := delta["reasoning_content"].(string); ok && reasoningContent != "" {
			thinking.emitThinkingDelta(w, reasoningContent, "text")
		}

		// Then, check for OpenRouter's reasoning_details format
		// Only process reasoning_details if we haven't already processed reasoning field
		if reasoningDetailsRaw, ok := delta["reasoning_details"]; ok && delta["reasoning"] == nil {
			if reasoningDetails, ok := reasoningDetailsRaw.([]interface{}); ok && len(reasoningDetails) > 0 {
				for _, detailRaw := range reasoningDetails {
					if detail, ok := detailRaw.(map[string]interface{}); ok {
						thinkingText := ""
						detailType, _ := detail["type"].(string)

						switch detailType {
						case "reasoning.text":
							thinkingText, _ = detail["text"].(string)
						case "reasoning.summary":
							thinkingText, _ = detail["summary"].(string)
						case "reasoning.encrypted":
							continue
						}

						if thinkingText != "" {
							thinking.emitThinkingDelta(w, thinkingText, "thinking")
						}
					}
				}
			}
		}

		// Handle reasoning field directly (simpler format from some models)
		if reasoning, ok := delta["reasoning"].(string); ok && reasoning != "" {
			thinking.emitThinkingDelta(w, reasoning, "thinking")
		}

		// Handle text delta
		if content, ok := delta["content"].(string); ok && content != "" {
			// Send content_block_start for text block on first text delta
			if !textBlockStarted {
				writeSSEEvent(w, "content_block_start", map[string]interface{}{
					"type":  "content_block_start",
					"index": textBlockIndex,
					"content_block": map[string]interface{}{
						"type": "text",
						"text": "",
					},
				})
				textBlockStarted = true
				_ = w.Flush()
			}

			writeSSEEvent(w, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": textBlockIndex,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": content,
				},
			})
			_ = w.Flush()
		}

		// Handle tool call deltas
		if toolCallsRaw, ok := delta["tool_calls"]; ok {
			// Debug: Log raw tool_calls from provider
			if cfg.Debug {
				toolCallsJSON, _ := json.Marshal(toolCallsRaw)
				fmt.Printf("[DEBUG] Raw tool_calls delta: %s\n", string(toolCallsJSON))
			}

			toolCalls, ok := toolCallsRaw.([]interface{})
			if ok && len(toolCalls) > 0 {
				for _, tcRaw := range toolCalls {
					tcDelta, ok := tcRaw.(map[string]interface{})
					if !ok {
						continue
					}

					// Get tool call index
					tcIndex := 0
					if idx, ok := tcDelta["index"].(float64); ok {
						tcIndex = int(idx)
					}

					// Initialize tool call tracking if not exists
					if _, exists := currentToolCalls[tcIndex]; !exists {
						currentToolCalls[tcIndex] = &ToolCallState{
							ID:          "",
							Name:        "",
							ArgsBuffer:  "",
							JSONSent:    false,
							ClaudeIndex: 0,
							Started:     false,
						}
					}

					toolCall := currentToolCalls[tcIndex]

					// Update tool call ID if provided
					if id, ok := tcDelta["id"].(string); ok {
						toolCall.ID = id
					}

					// Update function name
					if functionData, ok := tcDelta["function"].(map[string]interface{}); ok {
						if name, ok := functionData["name"].(string); ok {
							toolCall.Name = name
						}

						// Start content block when we have complete initial data
						if toolCall.ID != "" && toolCall.Name != "" && !toolCall.Started {
							toolBlockCounter++
							claudeIndex := textBlockIndex + toolBlockCounter
							toolCall.ClaudeIndex = claudeIndex
							toolCall.Started = true

							writeSSEEvent(w, "content_block_start", map[string]interface{}{
								"type":  "content_block_start",
								"index": claudeIndex,
								"content_block": map[string]interface{}{
									"type":  "tool_use",
									"id":    toolCall.ID,
									"name":  toolCall.Name,
									"input": map[string]interface{}{},
								},
							})
							_ = w.Flush()
						}

						// Handle function arguments
						// Type assertion handles nil check, Started flag, and we process even empty strings
						if args, ok := functionData["arguments"].(string); ok && toolCall.Started {
							// Only accumulate if args is not empty
							if args != "" {
								toolCall.ArgsBuffer += args
							}

							// Try to parse complete JSON and send delta when we have valid JSON
							if toolCall.ArgsBuffer != "" {
								var jsonTest interface{}
								if err := json.Unmarshal([]byte(toolCall.ArgsBuffer), &jsonTest); err == nil {
									// If parsing succeeds and we haven't sent this JSON yet
									if !toolCall.JSONSent {
										writeSSEEvent(w, "content_block_delta", map[string]interface{}{
											"type":  "content_block_delta",
											"index": toolCall.ClaudeIndex,
											"delta": map[string]interface{}{
												"type":         "input_json_delta",
												"partial_json": toolCall.ArgsBuffer,
											},
										})
										_ = w.Flush()
										toolCall.JSONSent = true
									}
								}
							}
							// If JSON is incomplete, continue accumulating (no action needed)
						}
					}
				}
			}
		}

		// Handle finish reason
		// NOTE: Don't break here - with stream_options.include_usage, OpenAI sends usage in a chunk AFTER finish_reason
		if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" {
			switch finishReason {
			case "length":
				finalStopReason = "max_tokens"
			case "tool_calls", "function_call":
				finalStopReason = "tool_use"
			case "stop":
				finalStopReason = "end_turn"
			default:
				finalStopReason = "end_turn"
			}
			// Continue processing to capture usage chunk (don't break)
		}
	}

	// Send final SSE events

	// Send content_block_stop for text block if it was started
	if textBlockStarted {
		writeSSEEvent(w, "content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": textBlockIndex,
		})
		_ = w.Flush()
	}

	// Send content_block_stop for each tool call
	for _, toolData := range currentToolCalls {
		// Check both Started AND claude_index is not None
		if toolData.Started && toolData.ClaudeIndex != 0 {
			writeSSEEvent(w, "content_block_stop", map[string]interface{}{
				"type":  "content_block_stop",
				"index": toolData.ClaudeIndex,
			})
			_ = w.Flush()
		}
	}

	// Send content_block_stop for thinking block if it had content
	if thinking.started && thinking.hasContent {
		writeSSEEvent(w, "content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": thinking.index,
		})
		_ = w.Flush()
	}

	// Debug: Check if usage data was received
	if cfg.Debug {
		inputTokens, _ := usageData["input_tokens"].(int)
		outputTokens, _ := usageData["output_tokens"].(int)
		if inputTokens == 0 && outputTokens == 0 {
			fmt.Printf("[DEBUG] OpenRouter streaming: Usage data unavailable (expected limitation of streaming API)\n")
		}
	}

	// Send message_delta with stop_reason and accumulated usage data
	// NOTE: We send the actual accumulated usage to fix the "0 tokens" issue in Claude Code
	if cfg.Debug {
		usageDataJSON, _ := json.Marshal(usageData)
		fmt.Printf("[DEBUG] Sending message_delta with usageData: %s\n", string(usageDataJSON))
	}
	writeSSEEvent(w, "message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   finalStopReason,
			"stop_sequence": nil,
		},
		"usage": usageData,
	})
	_ = w.Flush()

	// Send message_stop
	writeSSEEvent(w, "message_stop", map[string]interface{}{
		"type": "message_stop",
	})
	_ = w.Flush()

	// Simple log: one-line summary
	if cfg.SimpleLog {
		inputTokens := 0
		outputTokens := 0

		// Try to extract tokens from various possible formats
		if val, ok := usageData["input_tokens"].(int); ok {
			inputTokens = val
		} else if val, ok := usageData["input_tokens"].(float64); ok {
			inputTokens = int(val)
		}

		if val, ok := usageData["output_tokens"].(int); ok {
			outputTokens = val
		} else if val, ok := usageData["output_tokens"].(float64); ok {
			outputTokens = int(val)
		}

		// Debug: show what we actually have in usageData
		if cfg.Debug {
			fmt.Printf("[DEBUG] usageData: %+v\n", usageData)
		}

		// Calculate tokens per second
		duration := time.Since(startTime).Seconds()
		tokensPerSec := 0.0
		if duration > 0 && outputTokens > 0 {
			tokensPerSec = float64(outputTokens) / duration
		}

		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] [REQ] %s model=%s in=%d out=%d tok/s=%.1f\n",
			timestamp,
			cfg.GetBaseURLForModel(providerModel),
			providerModel,
			inputTokens,
			outputTokens,
			tokensPerSec)

		// Tool telemetry: log which tools were used during streaming
		if toolsSent > 0 && len(currentToolCalls) > 0 {
			var usedNames []string
			for _, tc := range currentToolCalls {
				if tc.Name != "" {
					usedNames = append(usedNames, tc.Name)
				}
			}
			if len(usedNames) > 0 {
				fmt.Printf("[%s] [TOOL] model=%s sent=%d used=%d name=%s\n",
					timestamp,
					providerModel,
					toolsSent,
					len(usedNames),
					strings.Join(usedNames, ","))
			}
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		writeSSEError(w, fmt.Sprintf("stream read error: %v", err))
	}
}

// writeSSEEvent writes a Server-Sent Event
func writeSSEEvent(w *bufio.Writer, event string, data interface{}) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("[ERROR] writeSSEEvent: failed to marshal %s event: %v\n", event, err)
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", string(dataJSON))
}

// writeSSEError writes an error event
func writeSSEError(w *bufio.Writer, message string) {
	writeSSEEvent(w, "error", map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "api_error",
			"message": message,
		},
	})
	_ = w.Flush()
}

// cacheMaxCompletionTokensSupported caches that a (provider, model) supports max_completion_tokens.
// Called after a successful first request that included max_completion_tokens.
func cacheMaxCompletionTokensSupported(req *models.OpenAIRequest, cfg *config.Config) {
	if req.MaxCompletionTokens > 0 {
		cacheKey := config.CacheKey{
			BaseURL: cfg.GetBaseURLForModel(req.Model),
			Model:   req.Model,
		}
		config.SetModelCapabilities(cacheKey, &config.ModelCapabilities{
			UsesMaxCompletionTokens: true,
		})
		if cfg.Debug {
			fmt.Printf("[DEBUG] Cached: model %s supports max_completion_tokens\n", req.Model)
		}
	}
}

// callOpenAI makes an HTTP request to the OpenAI API with automatic retry logic
// for max_completion_tokens parameter errors. Uses per-model capability caching.
func callOpenAI(req *models.OpenAIRequest, cfg *config.Config, baseURL, apiKey string) (*models.OpenAIResponse, error) {
	resp, err := callOpenAIInternal(req, cfg, baseURL, apiKey)
	if err != nil {
		if isMaxTokensParameterError(err.Error()) {
			if cfg.Debug {
				fmt.Printf("[DEBUG] Detected max_completion_tokens parameter error for model %s, retrying without it\n", req.Model)
			}
			retryReq := prepareRetryWithoutMaxCompletionTokens(req, cfg)
			return callOpenAIInternal(&retryReq, cfg, baseURL, apiKey)
		}

		// Retry with flattened messages when backend returns 400 on tool_result cycles.
		// Some vLLM deployments reject role:"tool" messages even when the model supports tool_use.
		if pe, ok := err.(*providerError); ok && pe.StatusCode == 400 && hasToolResults(req.Messages) {
			if cfg.SimpleLog {
				timestamp := time.Now().Format("15:04:05")
				fmt.Printf("[%s] [WARN] Backend 400 on tool_result — retrying with flattened messages\n", timestamp)
			}
			if cfg.Debug {
				fmt.Printf("[DEBUG] Backend 400 on tool_result for model %s, retrying with flattened messages\n", req.Model)
			}
			flatReq := *req
			flatReq.Messages = flattenToolMessages(req.Messages)
			flatResp, flatErr := callOpenAIInternal(&flatReq, cfg, baseURL, apiKey)
			if flatErr == nil {
				return flatResp, nil
			}
			// Final attempt: also strip tools in case the model schema itself causes issues
			if cfg.Debug {
				fmt.Printf("[DEBUG] Flattened retry also failed (%v), retrying without tools\n", flatErr)
			}
			flatReq.Tools = nil
			flatReq.ToolChoice = nil
			return callOpenAIInternal(&flatReq, cfg, baseURL, apiKey)
		}

		return nil, err
	}

	cacheMaxCompletionTokensSupported(req, cfg)
	return resp, nil
}

// callOpenAIStream makes a streaming HTTP request with retry logic for parameter errors.
// Uses per-model capability caching.
func callOpenAIStream(req *models.OpenAIRequest, cfg *config.Config, baseURL, apiKey string) (*http.Response, error) {
	resp, err := callOpenAIStreamInternal(req, cfg, baseURL, apiKey)
	if err != nil {
		if isMaxTokensParameterError(err.Error()) {
			if cfg.Debug {
				fmt.Printf("[DEBUG] Detected max_completion_tokens parameter error in stream for model %s, retrying without it\n", req.Model)
			}
			retryReq := prepareRetryWithoutMaxCompletionTokens(req, cfg)
			return callOpenAIStreamInternal(&retryReq, cfg, baseURL, apiKey)
		}

		// Retry with flattened messages when backend returns 400 on tool_result cycles.
		if pe, ok := err.(*providerError); ok && pe.StatusCode == 400 && hasToolResults(req.Messages) {
			if cfg.SimpleLog {
				timestamp := time.Now().Format("15:04:05")
				fmt.Printf("[%s] [WARN] Backend 400 on tool_result (stream) — retrying with flattened messages\n", timestamp)
			}
			if cfg.Debug {
				fmt.Printf("[DEBUG] Backend 400 on tool_result in stream for model %s, retrying with flattened messages\n", req.Model)
			}
			flatReq := *req
			flatReq.Messages = flattenToolMessages(req.Messages)
			flatResp, flatErr := callOpenAIStreamInternal(&flatReq, cfg, baseURL, apiKey)
			if flatErr == nil {
				return flatResp, nil
			}
			// Final attempt: also strip tools
			if cfg.Debug {
				fmt.Printf("[DEBUG] Flattened stream retry also failed (%v), retrying without tools\n", flatErr)
			}
			flatReq.Tools = nil
			flatReq.ToolChoice = nil
			return callOpenAIStreamInternal(&flatReq, cfg, baseURL, apiKey)
		}

		return nil, err
	}

	cacheMaxCompletionTokensSupported(req, cfg)
	return resp, nil
}

// makeOpenAIHTTPRequest builds and executes an HTTP request to the OpenAI API.
// It handles JSON marshaling, header setup (auth, OpenRouter), and provider-specific logic.
// The caller is responsible for closing the response body.
func makeOpenAIHTTPRequest(req *models.OpenAIRequest, cfg *config.Config, baseURL, apiKey string, timeout time.Duration) (*http.Response, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := baseURL + "/chat/completions"

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Skip auth for Ollama (localhost)
	isLocalhost := strings.Contains(strings.ToLower(baseURL), "localhost") || strings.Contains(strings.ToLower(baseURL), "127.0.0.1")
	if !isLocalhost {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	// OpenRouter-specific headers
	if cfg.DetectProvider() == config.ProviderOpenRouter {
		addOpenRouterHeaders(httpReq, cfg)
	}

	// Create HTTP client with proxy support (enterprise)
	client := &http.Client{
		Timeout:   timeout,
		Transport: cfg.GetHTTPTransport(),
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &providerError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	return resp, nil
}

// providerError represents an HTTP error from the upstream provider,
// preserving the original status code for proper propagation to the client.
type providerError struct {
	StatusCode int
	Body       string
}

func (e *providerError) Error() string {
	return fmt.Sprintf("OpenAI API returned status %d: %s", e.StatusCode, e.Body)
}

// callOpenAIStreamInternal makes a streaming HTTP request without retry logic
func callOpenAIStreamInternal(req *models.OpenAIRequest, cfg *config.Config, baseURL, apiKey string) (*http.Response, error) {
	return makeOpenAIHTTPRequest(req, cfg, baseURL, apiKey, 300*time.Second)
}

// isMaxTokensParameterError checks if the error message indicates an unsupported
// max_tokens or max_completion_tokens parameter issue.
// Uses broad keyword matching to handle different error message formats across providers.
// No status code checking - relies on message content alone.
func isMaxTokensParameterError(errorMessage string) bool {
	errorLower := strings.ToLower(errorMessage)

	// Check for parameter error indicators (various provider formats)
	hasParamIndicator := strings.Contains(errorLower, "parameter") ||
		strings.Contains(errorLower, "unsupported") ||
		strings.Contains(errorLower, "invalid") ||
		strings.Contains(errorLower, "extra_forbidden") ||
		strings.Contains(errorLower, "not permitted") ||
		strings.Contains(errorLower, "oasvalidation")

	// Check for our specific parameter names
	hasOurParam := strings.Contains(errorLower, "max_tokens") ||
		strings.Contains(errorLower, "max_completion_tokens")

	// Require both indicators to reduce false positives
	return hasParamIndicator && hasOurParam
}

// prepareRetryWithoutMaxCompletionTokens creates a copy of the request with
// max_completion_tokens transferred to max_tokens, and caches the capability.
// Shared by both streaming and non-streaming retry paths.
func prepareRetryWithoutMaxCompletionTokens(req *models.OpenAIRequest, cfg *config.Config) models.OpenAIRequest {
	retryReq := *req
	retryReq.MaxTokens = retryReq.MaxCompletionTokens
	retryReq.MaxCompletionTokens = 0

	if cfg.Debug {
		fmt.Printf("[DEBUG] Retrying with max_tokens=%d (was max_completion_tokens) for model: %s\n",
			retryReq.MaxTokens, req.Model)
	}

	// Cache that this (provider, model) doesn't support max_completion_tokens
	cacheKey := config.CacheKey{
		BaseURL: cfg.GetBaseURLForModel(req.Model),
		Model:   req.Model,
	}
	config.SetModelCapabilities(cacheKey, &config.ModelCapabilities{
		UsesMaxCompletionTokens: false,
	})

	return retryReq
}

// callOpenAIInternal is the internal implementation without retry logic
func callOpenAIInternal(req *models.OpenAIRequest, cfg *config.Config, baseURL, apiKey string) (*models.OpenAIResponse, error) {
	resp, err := makeOpenAIHTTPRequest(req, cfg, baseURL, apiKey, 90*time.Second)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var openaiResp models.OpenAIResponse
	if err := json.Unmarshal(respBody, &openaiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &openaiResp, nil
}

// hasToolResults reports whether any message in the slice has role "tool",
// indicating a multi-turn tool_result cycle that some vLLM backends reject with 400.
func hasToolResults(messages []models.OpenAIMessage) bool {
	for _, m := range messages {
		if m.Role == "tool" {
			return true
		}
	}
	return false
}

// extractToolResultContent converts a tool message Content (string or []interface{})
// to a plain string.
func extractToolResultContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if text, ok := itemMap["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return fmt.Sprintf("%v", content)
}

// flattenToolMessages rewrites tool_calls/tool messages as plain text.
// Used as a fallback when the backend returns 400 on multi-turn tool_result cycles.
// assistant messages with ToolCalls become text descriptions; role:"tool" messages
// become role:"user" messages. All other messages are passed through unchanged.
func flattenToolMessages(messages []models.OpenAIMessage) []models.OpenAIMessage {
	result := make([]models.OpenAIMessage, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				lines := make([]string, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					lines = append(lines, fmt.Sprintf(
						"Calling tool `%s` with: %s",
						tc.Function.Name, tc.Function.Arguments,
					))
				}
				result = append(result, models.OpenAIMessage{
					Role:    "assistant",
					Content: strings.Join(lines, "\n"),
				})
			} else {
				result = append(result, msg)
			}
		case "tool":
			result = append(result, models.OpenAIMessage{
				Role:    "user",
				Content: "[Tool result]: " + extractToolResultContent(msg.Content),
			})
		default:
			result = append(result, msg)
		}
	}
	return result
}

func handleCountTokens(c *fiber.Ctx, cfg *config.Config) error {
	// Approximate token count based on body size (~4 chars per token).
	// This is a rough estimate since exact tokenization depends on the model's tokenizer.
	bodyLen := len(c.Body())
	estimatedTokens := bodyLen / 4
	if estimatedTokens < 1 {
		estimatedTokens = 1
	}

	if cfg.Debug {
		fmt.Printf("[DEBUG] handleCountTokens: body=%d bytes, estimated=%d tokens\n", bodyLen, estimatedTokens)
	}

	return c.JSON(fiber.Map{
		"input_tokens": estimatedTokens,
	})
}

// isCacheable returns true when a request is eligible for response caching.
// Only deterministic requests (temperature <= maxTemp) are cached.
// A nil temperature is treated as 0.0 (fully deterministic).
func isCacheable(req models.ClaudeRequest, maxTemp float64) bool {
	temp := 0.0
	if req.Temperature != nil {
		temp = *req.Temperature
	}
	return temp <= maxTemp
}
