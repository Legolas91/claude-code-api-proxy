package server

import (
	"strings"
	"testing"

	"github.com/claude-code-proxy/proxy/pkg/models"
)

func TestMessagesToPrompt(t *testing.T) {
	tests := []struct {
		name     string
		req      models.ClaudeRequest
		contains []string
	}{
		{
			name: "simple text message",
			req: models.ClaudeRequest{
				Messages: []models.ClaudeMessage{
					{Role: "user", Content: "Hello world"},
				},
			},
			contains: []string{"Human: Hello world"},
		},
		{
			name: "multi-turn conversation",
			req: models.ClaudeRequest{
				Messages: []models.ClaudeMessage{
					{Role: "user", Content: "What is Go?"},
					{Role: "assistant", Content: "Go is a programming language."},
					{Role: "user", Content: "Thanks!"},
				},
			},
			contains: []string{
				"Human: What is Go?",
				"Assistant: Go is a programming language.",
				"Human: Thanks!",
			},
		},
		{
			name: "with system prompt string",
			req: models.ClaudeRequest{
				System: "You are a helpful assistant.",
				Messages: []models.ClaudeMessage{
					{Role: "user", Content: "Hi"},
				},
			},
			contains: []string{
				"You are a helpful assistant.",
				"Human: Hi",
			},
		},
		{
			name: "with system prompt array",
			req: models.ClaudeRequest{
				System: []interface{}{
					map[string]interface{}{"type": "text", "text": "System instruction 1"},
					map[string]interface{}{"type": "text", "text": "System instruction 2"},
				},
				Messages: []models.ClaudeMessage{
					{Role: "user", Content: "Hi"},
				},
			},
			contains: []string{
				"System instruction 1",
				"System instruction 2",
			},
		},
		{
			name: "with tool_use block",
			req: models.ClaudeRequest{
				Messages: []models.ClaudeMessage{
					{
						Role: "assistant",
						Content: []interface{}{
							map[string]interface{}{
								"type":  "tool_use",
								"name":  "read_file",
								"input": map[string]interface{}{"path": "/tmp/test.txt"},
							},
						},
					},
				},
			},
			contains: []string{"[Tool call: read_file("},
		},
		{
			name: "with tool_result block",
			req: models.ClaudeRequest{
				Messages: []models.ClaudeMessage{
					{
						Role: "user",
						Content: []interface{}{
							map[string]interface{}{
								"type":        "tool_result",
								"tool_use_id": "toolu_123",
								"content":     "file contents here",
							},
						},
					},
				},
			},
			contains: []string{"[Tool result: file contents here]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := messagesToPrompt(tt.req)
			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("messagesToPrompt() result does not contain %q\nGot: %s", substr, result)
				}
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "empty string",
			text:     "",
			expected: 0,
		},
		{
			name:     "short text (less than 4 chars)",
			text:     "Hi",
			expected: 1,
		},
		{
			name:     "normal text",
			text:     "Hello, this is a test of the token estimator function.", // 54 chars
			expected: 13,                                                      // 54/4 = 13
		},
		{
			name:     "exact multiple of 4",
			text:     "12345678", // 8 chars
			expected: 2,         // 8/4 = 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.text)
			if got != tt.expected {
				t.Errorf("estimateTokens(%q) = %d, want %d", tt.text, got, tt.expected)
			}
		})
	}
}

func TestExtractSystemForClaudeCode(t *testing.T) {
	tests := []struct {
		name     string
		system   interface{}
		expected string
	}{
		{
			name:     "nil system",
			system:   nil,
			expected: "",
		},
		{
			name:     "string system",
			system:   "You are helpful.",
			expected: "You are helpful.",
		},
		{
			name: "array system with text blocks",
			system: []interface{}{
				map[string]interface{}{"type": "text", "text": "Part 1"},
				map[string]interface{}{"type": "text", "text": "Part 2"},
			},
			expected: "Part 1\nPart 2",
		},
		{
			name: "array system with non-text blocks ignored",
			system: []interface{}{
				map[string]interface{}{"type": "text", "text": "Only this"},
				map[string]interface{}{"type": "image", "source": "data:..."},
			},
			expected: "Only this",
		},
		{
			name:     "unsupported type returns empty",
			system:   42,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSystemForClaudeCode(tt.system)
			if got != tt.expected {
				t.Errorf("extractSystemForClaudeCode() = %q, want %q", got, tt.expected)
			}
		})
	}
}
