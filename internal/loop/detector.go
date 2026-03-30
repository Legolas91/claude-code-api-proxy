// Package loop detects and breaks infinite tool-call retry loops.
//
// When backend models (e.g. Codestral/Mistral) receive a tool error, they sometimes
// retry the exact same tool call indefinitely. This package detects consecutive
// identical tool_use blocks in the conversation and injects a nudge message
// to force the model to try a different approach.
package loop

import (
	"encoding/json"

	"github.com/claude-code-proxy/proxy/pkg/models"
)

// NudgeMessage is injected at loop level 1 (gentle nudge).
const NudgeMessage = "This approach has failed multiple times with the same error. " +
	"Stop retrying the same command. Try a completely different approach to accomplish the goal."

// StrongNudgeMessage is injected at loop level 2 (persistent loop).
const StrongNudgeMessage = "You have attempted the same failing operation many times. " +
	"STOP. This strategy does not work. You MUST try a completely different approach. " +
	"If you cannot proceed without this tool, explain clearly why it is failing."

// toolSignature uniquely identifies a tool call by name and serialized input.
type toolSignature struct {
	Name  string
	Input string
}

// DetectRetryLoop scans the conversation messages from the tail and returns true
// if the last maxRetries consecutive assistant tool_use calls are identical
// (same tool name + same input). Only consecutive pairs of assistant(tool_use) →
// user(tool_result) are considered.
func DetectRetryLoop(messages []models.ClaudeMessage, maxRetries int) bool {
	if maxRetries < 2 || len(messages) < 2 {
		return false
	}

	// Collect tool signatures from assistant messages, walking backwards
	var signatures []toolSignature
	for i := len(messages) - 1; i >= 0 && len(signatures) < maxRetries; i-- {
		msg := messages[i]

		if msg.Role == "assistant" {
			sig, ok := extractToolSignature(msg)
			if !ok {
				// Assistant message without tool_use — break the chain
				break
			}
			signatures = append(signatures, sig)
		}
		// Skip user messages (tool_result responses) — they are expected between assistant tool_use calls
	}

	if len(signatures) < maxRetries {
		return false
	}

	// Check if all collected signatures are identical
	ref := signatures[0]
	for _, sig := range signatures[1:] {
		if sig.Name != ref.Name || sig.Input != ref.Input {
			return false
		}
	}

	return true
}

// InjectLoopBreaker appends a user nudge message to break the retry loop.
func InjectLoopBreaker(messages []models.ClaudeMessage) []models.ClaudeMessage {
	return append(messages, models.ClaudeMessage{
		Role:    "user",
		Content: NudgeMessage,
	})
}

// CountIdenticalCalls counts the number of consecutive identical tool_use calls at the
// tail of the conversation. Returns 0 if the last assistant message is not a tool_use
// or if fewer than two consecutive identical calls are found.
func CountIdenticalCalls(messages []models.ClaudeMessage) int {
	if len(messages) == 0 {
		return 0
	}

	var ref *toolSignature
	count := 0

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "assistant" {
			sig, ok := extractToolSignature(msg)
			if !ok {
				break
			}
			if ref == nil {
				ref = &sig
				count = 1
			} else if sig.Name == ref.Name && sig.Input == ref.Input {
				count++
			} else {
				break
			}
		}
		// Skip user messages (tool_result responses between retries)
	}
	return count
}

// GetLoopLevel returns the severity level of a detected retry loop:
//
//	0 = no loop (count < maxRetries)
//	1 = gentle nudge (count in [maxRetries, 2*maxRetries))
//	2 = strong nudge (count in [2*maxRetries, 3*maxRetries))
//	3 = disable tools (count >= 3*maxRetries)
//
// maxLoopLevel caps the returned level (e.g. maxLoopLevel=2 prevents tool disabling).
func GetLoopLevel(messages []models.ClaudeMessage, maxRetries, maxLoopLevel int) int {
	if maxRetries < 2 {
		return 0
	}
	count := CountIdenticalCalls(messages)
	if count < maxRetries {
		return 0
	}
	level := count / maxRetries
	if level > 3 {
		level = 3
	}
	if maxLoopLevel > 0 && level > maxLoopLevel {
		level = maxLoopLevel
	}
	return level
}

// InjectLoopBreakerLevel injects an appropriate nudge based on severity:
//
//	level 1 → NudgeMessage (gentle)
//	level 2+ → StrongNudgeMessage (persistent loop)
func InjectLoopBreakerLevel(messages []models.ClaudeMessage, level int) []models.ClaudeMessage {
	msg := NudgeMessage
	if level >= 2 {
		msg = StrongNudgeMessage
	}
	return append(messages, models.ClaudeMessage{
		Role:    "user",
		Content: msg,
	})
}

// extractToolSignature returns the (name, serialized-input) of the first tool_use
// block in an assistant message. Returns false if none found.
func extractToolSignature(msg models.ClaudeMessage) (toolSignature, bool) {
	blocks, ok := msg.Content.([]interface{})
	if !ok {
		return toolSignature{}, false
	}

	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		if blockMap["type"] != "tool_use" {
			continue
		}

		name, _ := blockMap["name"].(string)
		input := blockMap["input"]

		// Serialize input deterministically for comparison
		inputJSON, err := json.Marshal(input)
		if err != nil {
			inputJSON = []byte("{}")
		}

		return toolSignature{
			Name:  name,
			Input: string(inputJSON),
		}, true
	}

	return toolSignature{}, false
}
