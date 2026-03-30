package converter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/claude-code-proxy/proxy/pkg/models"
)

var (
	// Matches arguments wrapped in a markdown code block: ```json\n{...}\n```
	reMDCodeBlock = regexp.MustCompile("(?s)^\\s*```(?:json)?\\s*\\n?(\\{[^`]*\\})\\s*```\\s*$")
	// Matches arguments wrapped in an XML <arguments> tag
	reXMLArguments = regexp.MustCompile(`(?s)<arguments>\s*(\{.*?\})\s*</arguments>`)
)

// RepairToolCalls fixes common malformed tool call patterns in non-streaming OpenAI responses.
// Returns true if any repairs were made.
//
// Handles:
//   - Arguments wrapped in markdown code blocks (```json {...} ```)
//   - Arguments wrapped in XML <arguments> tags
//   - Missing or empty tool call IDs
//   - Extraneous whitespace around JSON
//
// Only applied on the non-streaming path; streaming responses cannot be repaired after the fact.
func RepairToolCalls(resp *models.OpenAIResponse) bool {
	if resp == nil || len(resp.Choices) == 0 {
		return false
	}
	repaired := false
	for i := range resp.Choices {
		msg := &resp.Choices[i].Message
		for j := range msg.ToolCalls {
			tc := &msg.ToolCalls[j]

			// Fix missing ID
			if tc.ID == "" {
				tc.ID = fmt.Sprintf("call_%d%d", i, j)
				repaired = true
			}

			args := tc.Function.Arguments

			// Fix: arguments wrapped in markdown code block
			if m := reMDCodeBlock.FindStringSubmatch(args); m != nil {
				tc.Function.Arguments = strings.TrimSpace(m[1])
				repaired = true
				continue
			}

			// Fix: arguments wrapped in XML <arguments> tag
			if m := reXMLArguments.FindStringSubmatch(args); m != nil {
				tc.Function.Arguments = strings.TrimSpace(m[1])
				repaired = true
				continue
			}

			// Fix: extraneous whitespace around JSON
			trimmed := strings.TrimSpace(args)
			if trimmed != args {
				tc.Function.Arguments = trimmed
				repaired = true
			}
		}
	}
	return repaired
}
