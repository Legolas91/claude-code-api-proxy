package loop

import (
	"testing"

	"github.com/claude-code-proxy/proxy/pkg/models"
)

// helper to build an assistant message with a single tool_use block
func assistantToolUse(name string, input map[string]interface{}) models.ClaudeMessage {
	return models.ClaudeMessage{
		Role: "assistant",
		Content: []interface{}{
			map[string]interface{}{
				"type":  "tool_use",
				"id":    "toolu_test",
				"name":  name,
				"input": input,
			},
		},
	}
}

// helper to build a user message with a tool_result block
func userToolResult(toolUseID, content string) models.ClaudeMessage {
	return models.ClaudeMessage{
		Role: "user",
		Content: []interface{}{
			map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": toolUseID,
				"content":     content,
			},
		},
	}
}

func TestDetectRetryLoop_NoLoop(t *testing.T) {
	messages := []models.ClaudeMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "List files"},
	}

	if DetectRetryLoop(messages, 3) {
		t.Error("expected no loop for plain conversation")
	}
}

func TestDetectRetryLoop_ExactlyN(t *testing.T) {
	input := map[string]interface{}{"command": "gh repo view wrongname/repo"}
	messages := []models.ClaudeMessage{
		{Role: "user", Content: "Check this repo"},
		assistantToolUse("Bash", input),
		userToolResult("toolu_1", "exit code 1: not found"),
		assistantToolUse("Bash", input),
		userToolResult("toolu_2", "exit code 1: not found"),
		assistantToolUse("Bash", input),
		userToolResult("toolu_3", "exit code 1: not found"),
	}

	if !DetectRetryLoop(messages, 3) {
		t.Error("expected loop to be detected with 3 identical tool calls")
	}
}

func TestDetectRetryLoop_BelowThreshold(t *testing.T) {
	input := map[string]interface{}{"command": "gh repo view wrongname/repo"}
	messages := []models.ClaudeMessage{
		{Role: "user", Content: "Check this repo"},
		assistantToolUse("Bash", input),
		userToolResult("toolu_1", "exit code 1: not found"),
		assistantToolUse("Bash", input),
		userToolResult("toolu_2", "exit code 1: not found"),
	}

	if DetectRetryLoop(messages, 3) {
		t.Error("expected no loop with only 2 identical calls (threshold=3)")
	}
}

func TestDetectRetryLoop_DifferentInputs(t *testing.T) {
	messages := []models.ClaudeMessage{
		{Role: "user", Content: "Check repos"},
		assistantToolUse("Bash", map[string]interface{}{"command": "gh repo view repo1"}),
		userToolResult("toolu_1", "not found"),
		assistantToolUse("Bash", map[string]interface{}{"command": "gh repo view repo2"}),
		userToolResult("toolu_2", "not found"),
		assistantToolUse("Bash", map[string]interface{}{"command": "gh repo view repo3"}),
		userToolResult("toolu_3", "not found"),
	}

	if DetectRetryLoop(messages, 3) {
		t.Error("expected no loop when inputs differ")
	}
}

func TestDetectRetryLoop_DifferentToolNames(t *testing.T) {
	input := map[string]interface{}{"command": "test"}
	messages := []models.ClaudeMessage{
		{Role: "user", Content: "Try things"},
		assistantToolUse("Bash", input),
		userToolResult("toolu_1", "error"),
		assistantToolUse("Read", input),
		userToolResult("toolu_2", "error"),
		assistantToolUse("Bash", input),
		userToolResult("toolu_3", "error"),
	}

	if DetectRetryLoop(messages, 3) {
		t.Error("expected no loop when tool names differ")
	}
}

func TestDetectRetryLoop_MixedConversation(t *testing.T) {
	// Loop in the middle but not at the tail
	input := map[string]interface{}{"command": "failing-cmd"}
	messages := []models.ClaudeMessage{
		{Role: "user", Content: "Do something"},
		assistantToolUse("Bash", input),
		userToolResult("toolu_1", "error"),
		assistantToolUse("Bash", input),
		userToolResult("toolu_2", "error"),
		assistantToolUse("Bash", input),
		userToolResult("toolu_3", "error"),
		// Model changed approach
		{Role: "assistant", Content: "Let me try something else"},
		{Role: "user", Content: "OK"},
	}

	if DetectRetryLoop(messages, 3) {
		t.Error("expected no loop when tail is not a tool_use pattern")
	}
}

func TestDetectRetryLoop_StringContent(t *testing.T) {
	messages := []models.ClaudeMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}

	if DetectRetryLoop(messages, 3) {
		t.Error("expected no loop for string-only content")
	}
}

func TestDetectRetryLoop_Disabled(t *testing.T) {
	input := map[string]interface{}{"command": "fail"}
	messages := []models.ClaudeMessage{
		assistantToolUse("Bash", input),
		userToolResult("toolu_1", "error"),
		assistantToolUse("Bash", input),
		userToolResult("toolu_2", "error"),
		assistantToolUse("Bash", input),
	}

	// maxRetries=0 should disable detection
	if DetectRetryLoop(messages, 0) {
		t.Error("expected no detection when maxRetries=0")
	}
	// maxRetries=1 should also be disabled (needs at least 2 to compare)
	if DetectRetryLoop(messages, 1) {
		t.Error("expected no detection when maxRetries=1")
	}
}

func TestInjectLoopBreaker(t *testing.T) {
	messages := []models.ClaudeMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}

	result := InjectLoopBreaker(messages)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	nudge := result[2]
	if nudge.Role != "user" {
		t.Errorf("expected nudge role=user, got %s", nudge.Role)
	}

	content, ok := nudge.Content.(string)
	if !ok {
		t.Fatal("expected nudge content to be a string")
	}
	if content != NudgeMessage {
		t.Errorf("unexpected nudge content: %s", content)
	}
}
