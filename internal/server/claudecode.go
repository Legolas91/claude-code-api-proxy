package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/claude-code-proxy/proxy/internal/config"
	"github.com/claude-code-proxy/proxy/pkg/models"
	"github.com/gofiber/fiber/v2"
)

// handleClaudeCodeMessages handles requests routed to the claude -p backend.
// It spawns `claude -p` using the user's Pro/Max subscription instead of
// calling an API endpoint. Supports both streaming and non-streaming modes.
func handleClaudeCodeMessages(c *fiber.Ctx, claudeReq models.ClaudeRequest, cfg *config.Config) error {
	prompt := messagesToPrompt(claudeReq)

	// Build claude CLI arguments — prompt is passed via stdin to avoid ARG_MAX limit.
	// Note: claude CLI does not support --max-tokens; token budget is managed by the subscription.
	args := []string{"-p", "--output-format", "json"}
	if claudeReq.Model != "" {
		args = append(args, "--model", claudeReq.Model)
	}

	if cfg.Debug {
		fmt.Printf("[DEBUG] claude-p: spawning claude %v (prompt: %d bytes via stdin)\n", args, len(prompt))
	}

	isStreaming := claudeReq.Stream != nil && *claudeReq.Stream
	if isStreaming {
		return handleClaudeCodeStreaming(c, claudeReq, cfg, prompt)
	}

	return handleClaudeCodeNonStreaming(c, claudeReq, cfg, args, prompt)
}

// handleClaudeCodeNonStreaming runs `claude -p` and returns the result as a Claude API response.
func handleClaudeCodeNonStreaming(c *fiber.Ctx, claudeReq models.ClaudeRequest, cfg *config.Config, args []string, prompt string) error {
	startTime := time.Now()

	cmd := exec.Command("claude", args...) //nolint:gosec
	cmd.Env = claudeCodeEnv()
	cmd.Stdin = strings.NewReader(prompt)
	output, err := cmd.Output()
	if err != nil {
		errMsg := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			errMsg = string(exitErr.Stderr)
		}
		if cfg.Debug {
			fmt.Printf("[DEBUG] claude-p: command failed: %v stderr=%s\n", err, errMsg)
		}
		return c.Status(500).JSON(fiber.Map{
			"type": "error",
			"error": fiber.Map{
				"type":    "api_error",
				"message": fmt.Sprintf("claude -p failed: %v %s", err, errMsg),
			},
		})
	}

	// Try to parse structured JSON output from claude CLI
	responseText := strings.TrimSpace(string(output))

	// claude --output-format json returns: {"type":"result","result":"...","session_id":"...","cost_usd":...}
	var cliResult struct {
		Type   string `json:"type"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseText), &cliResult); err == nil && cliResult.Result != "" {
		responseText = cliResult.Result
	}

	// Build Claude API response
	stopReason := "end_turn"
	claudeResp := &models.ClaudeResponse{
		ID:   fmt.Sprintf("msg_claudep_%d", time.Now().UnixNano()),
		Type: "message",
		Role: "assistant",
		Content: []models.ContentBlock{
			{
				Type: "text",
				Text: responseText,
			},
		},
		Model:      claudeReq.Model,
		StopReason: &stopReason,
		Usage: models.Usage{
			InputTokens:  estimateTokens(messagesToPrompt(claudeReq)),
			OutputTokens: estimateTokens(responseText),
		},
	}

	if cfg.SimpleLog {
		duration := time.Since(startTime).Seconds()
		tokensPerSec := 0.0
		if duration > 0 && claudeResp.Usage.OutputTokens > 0 {
			tokensPerSec = float64(claudeResp.Usage.OutputTokens) / duration
		}
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] [REQ] claude-p model=%s in=%d out=%d tok/s=%.1f\n",
			timestamp, claudeReq.Model,
			claudeResp.Usage.InputTokens, claudeResp.Usage.OutputTokens, tokensPerSec)
	}

	return c.JSON(claudeResp)
}

// handleClaudeCodeStreaming runs `claude -p` with streaming output and emits Claude SSE events.
func handleClaudeCodeStreaming(c *fiber.Ctx, claudeReq models.ClaudeRequest, cfg *config.Config, prompt string) error {
	startTime := time.Now()

	// Build args for streaming (use stream-json for structured output)
	// Note: claude CLI does not support --max-tokens; token budget is managed by the subscription.
	args := []string{"-p", "--output-format", "stream-json", "--verbose"}
	if claudeReq.Model != "" {
		args = append(args, "--model", claudeReq.Model)
	}

	if cfg.Debug {
		fmt.Printf("[DEBUG] claude-p stream: spawning claude %v (prompt: %d bytes via stdin)\n", args, len(prompt))
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		messageID := fmt.Sprintf("msg_claudep_%d", time.Now().UnixNano())

		// Send message_start
		writeSSEEvent(w, "message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":            messageID,
				"type":          "message",
				"role":          "assistant",
				"model":         claudeReq.Model,
				"content":       []interface{}{},
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]interface{}{
					"input_tokens":  0,
					"output_tokens": 0,
				},
			},
		})
		writeSSEEvent(w, "ping", map[string]interface{}{"type": "ping"})
		_ = w.Flush()

		// Send content_block_start for text
		writeSSEEvent(w, "content_block_start", map[string]interface{}{
			"type":          "content_block_start",
			"index":         0,
			"content_block": map[string]interface{}{"type": "text", "text": ""},
		})
		_ = w.Flush()

		cmd := exec.Command("claude", args...) //nolint:gosec
		cmd.Env = claudeCodeEnv()
		cmd.Stdin = strings.NewReader(prompt)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			writeSSEError(w, fmt.Sprintf("failed to create pipe: %v", err))
			return
		}

		if err := cmd.Start(); err != nil {
			writeSSEError(w, fmt.Sprintf("failed to start claude: %v", err))
			return
		}

		// Read streaming JSON events from claude CLI
		totalOutput := 0
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			// claude --output-format stream-json emits JSON lines
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				// Not JSON — treat as raw text chunk
				streamClaudeCodeTextDelta(w, line, 0)
				totalOutput += len(line)
				continue
			}

			// Handle different event types from claude CLI stream
			eventType, _ := event["type"].(string)
			switch eventType {
			case "assistant":
				// assistant message event — may contain text content
				if message, ok := event["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok && content != "" {
						streamClaudeCodeTextDelta(w, content, 0)
						totalOutput += len(content)
					}
				}
			case "content_block_delta":
				// Already in Claude format — forward delta
				if delta, ok := event["delta"].(map[string]interface{}); ok {
					if text, ok := delta["text"].(string); ok && text != "" {
						streamClaudeCodeTextDelta(w, text, 0)
						totalOutput += len(text)
					}
				}
			case "result":
				// Final result event
				if result, ok := event["result"].(string); ok && result != "" {
					streamClaudeCodeTextDelta(w, result, 0)
					totalOutput += len(result)
				}
			default:
				// Unknown event type — try to extract text content
				if text, ok := event["text"].(string); ok && text != "" {
					streamClaudeCodeTextDelta(w, text, 0)
					totalOutput += len(text)
				}
			}
		}

		// Wait for process to finish
		_ = cmd.Wait()

		// Send content_block_stop
		writeSSEEvent(w, "content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": 0,
		})

		// Send message_delta with stop reason
		outputTokens := estimateTokens(fmt.Sprintf("%d chars", totalOutput))
		writeSSEEvent(w, "message_delta", map[string]interface{}{
			"type": "message_delta",
			"delta": map[string]interface{}{
				"stop_reason":   "end_turn",
				"stop_sequence": nil,
			},
			"usage": map[string]interface{}{
				"output_tokens": outputTokens,
			},
		})

		// Send message_stop
		writeSSEEvent(w, "message_stop", map[string]interface{}{
			"type": "message_stop",
		})
		_ = w.Flush()

		if cfg.SimpleLog {
			duration := time.Since(startTime).Seconds()
			tokensPerSec := 0.0
			if duration > 0 && outputTokens > 0 {
				tokensPerSec = float64(outputTokens) / duration
			}
			timestamp := time.Now().Format("15:04:05")
			fmt.Printf("[%s] [REQ] claude-p model=%s out=%d tok/s=%.1f (stream)\n",
				timestamp, claudeReq.Model, outputTokens, tokensPerSec)
		}
	})

	return nil
}

// streamClaudeCodeTextDelta emits a single text_delta SSE event.
func streamClaudeCodeTextDelta(w *bufio.Writer, text string, index int) {
	writeSSEEvent(w, "content_block_delta", map[string]interface{}{
		"type":  "content_block_delta",
		"index": index,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": text,
		},
	})
	_ = w.Flush()
}

// messagesToPrompt converts Claude API messages into a text prompt for `claude -p`.
// It serializes the conversation history with role markers.
func messagesToPrompt(req models.ClaudeRequest) string {
	var parts []string

	// Include system prompt if present
	systemText := extractSystemForClaudeCode(req.System)
	if systemText != "" {
		parts = append(parts, systemText)
	}

	// Convert each message
	for _, msg := range req.Messages {
		prefix := ""
		switch msg.Role {
		case "user":
			prefix = "Human: "
		case "assistant":
			prefix = "Assistant: "
		}

		switch content := msg.Content.(type) {
		case string:
			parts = append(parts, prefix+content)
		case []interface{}:
			for _, block := range content {
				if blockMap, ok := block.(map[string]interface{}); ok {
					switch blockMap["type"] {
					case "text":
						if text, ok := blockMap["text"].(string); ok {
							parts = append(parts, prefix+text)
						}
					case "tool_use":
						name, _ := blockMap["name"].(string)
						input, _ := json.Marshal(blockMap["input"])
						parts = append(parts, fmt.Sprintf("%s[Tool call: %s(%s)]", prefix, name, string(input)))
					case "tool_result":
						resultContent := ""
						if rc, ok := blockMap["content"].(string); ok {
							resultContent = rc
						}
						parts = append(parts, fmt.Sprintf("[Tool result: %s]", resultContent))
					}
				}
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

// extractSystemForClaudeCode extracts system text from Claude's flexible system parameter.
func extractSystemForClaudeCode(system interface{}) string {
	if system == nil {
		return ""
	}
	if s, ok := system.(string); ok {
		return s
	}
	if arr, ok := system.([]interface{}); ok {
		var parts []string
		for _, block := range arr {
			if m, ok := block.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if text, ok := m["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// estimateTokens provides a rough token count estimate (~4 chars per token).
// Used for usage reporting when actual token counts are unavailable.
func estimateTokens(text string) int {
	tokens := len(text) / 4
	if tokens == 0 && len(text) > 0 {
		tokens = 1
	}
	return tokens
}

// claudeCodeEnv returns a filtered environment for the claude subprocess.
// ANTHROPIC_BASE_URL and ANTHROPIC_API_URL are stripped so that the child
// process connects directly to Anthropic instead of looping back to the proxy.
func claudeCodeEnv() []string {
	skip := map[string]bool{
		"ANTHROPIC_BASE_URL": true,
		"ANTHROPIC_API_URL":  true,
	}
	var env []string
	for _, e := range os.Environ() {
		key := e
		if idx := strings.IndexByte(e, '='); idx >= 0 {
			key = e[:idx]
		}
		if !skip[key] {
			env = append(env, e)
		}
	}
	return env
}
