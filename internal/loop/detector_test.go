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

func TestCountIdenticalCalls_Empty(t *testing.T) {
	if CountIdenticalCalls(nil) != 0 {
		t.Error("expected 0 for nil messages")
	}
	if CountIdenticalCalls([]models.ClaudeMessage{}) != 0 {
		t.Error("expected 0 for empty messages")
	}
}

func TestCountIdenticalCalls_Three(t *testing.T) {
	input := map[string]interface{}{"command": "failing-cmd"}
	messages := []models.ClaudeMessage{
		{Role: "user", Content: "Do something"},
		assistantToolUse("Bash", input),
		userToolResult("toolu_1", "error"),
		assistantToolUse("Bash", input),
		userToolResult("toolu_2", "error"),
		assistantToolUse("Bash", input),
		userToolResult("toolu_3", "error"),
	}
	if got := CountIdenticalCalls(messages); got != 3 {
		t.Errorf("expected 3 identical calls, got %d", got)
	}
}

func TestCountIdenticalCalls_StopsAtDifferent(t *testing.T) {
	messages := []models.ClaudeMessage{
		assistantToolUse("Bash", map[string]interface{}{"command": "cmd-A"}),
		userToolResult("t1", "error"),
		assistantToolUse("Bash", map[string]interface{}{"command": "cmd-A"}),
		userToolResult("t2", "error"),
		assistantToolUse("Bash", map[string]interface{}{"command": "cmd-B"}), // different
		userToolResult("t3", "error"),
	}
	// Scan from tail: last 2 assistant messages are A then B (different) → count=1 (only the tail)
	// Wait, CountIdenticalCalls scans from the tail. The last assistant message is cmd-B.
	// Then previous assistant is cmd-A (different from cmd-B) → breaks at 1.
	if got := CountIdenticalCalls(messages); got != 1 {
		t.Errorf("expected 1 (only tail matches itself), got %d", got)
	}
}

func TestCountIdenticalCalls_TextTail(t *testing.T) {
	input := map[string]interface{}{"command": "cmd"}
	messages := []models.ClaudeMessage{
		assistantToolUse("Bash", input),
		userToolResult("t1", "error"),
		{Role: "assistant", Content: "Let me try differently"}, // text, no tool_use
	}
	if got := CountIdenticalCalls(messages); got != 0 {
		t.Errorf("expected 0 when tail is text, got %d", got)
	}
}

func TestGetLoopLevel(t *testing.T) {
	input := map[string]interface{}{"command": "cmd"}

	buildMessages := func(n int) []models.ClaudeMessage {
		msgs := []models.ClaudeMessage{{Role: "user", Content: "start"}}
		for i := 0; i < n; i++ {
			msgs = append(msgs, assistantToolUse("Bash", input))
			msgs = append(msgs, userToolResult("t", "error"))
		}
		return msgs
	}

	tests := []struct {
		name         string
		count        int
		maxRetries   int
		maxLoopLevel int
		want         int
	}{
		{"below threshold", 2, 3, 3, 0},
		{"exactly threshold → level 1", 3, 3, 3, 1},
		{"double threshold → level 2", 6, 3, 3, 2},
		{"triple threshold → level 3", 9, 3, 3, 3},
		{"capped by maxLoopLevel=1", 9, 3, 1, 1},
		{"capped by maxLoopLevel=2", 9, 3, 2, 2},
		{"disabled (maxRetries=0)", 9, 0, 3, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := buildMessages(tt.count)
			got := GetLoopLevel(msgs, tt.maxRetries, tt.maxLoopLevel)
			if got != tt.want {
				t.Errorf("GetLoopLevel(count=%d, maxRetries=%d, maxLoopLevel=%d) = %d, want %d",
					tt.count, tt.maxRetries, tt.maxLoopLevel, got, tt.want)
			}
		})
	}
}

func TestInjectLoopBreakerLevel(t *testing.T) {
	base := []models.ClaudeMessage{{Role: "user", Content: "hello"}}

	t.Run("level 1 → gentle nudge", func(t *testing.T) {
		result := InjectLoopBreakerLevel(base, 1)
		if len(result) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(result))
		}
		if result[1].Content.(string) != NudgeMessage {
			t.Errorf("expected NudgeMessage, got %q", result[1].Content)
		}
	})

	t.Run("level 2 → strong nudge", func(t *testing.T) {
		result := InjectLoopBreakerLevel(base, 2)
		if result[1].Content.(string) != StrongNudgeMessage {
			t.Errorf("expected StrongNudgeMessage, got %q", result[1].Content)
		}
	})

	t.Run("level 3 → strong nudge (same as 2)", func(t *testing.T) {
		result := InjectLoopBreakerLevel(base, 3)
		if result[1].Content.(string) != StrongNudgeMessage {
			t.Errorf("expected StrongNudgeMessage, got %q", result[1].Content)
		}
	})
}
