package converter

import "strings"

// GetToolGuidanceForModel returns model-specific tool-use instructions to be prepended
// to the system prompt. Helps enterprise LLM gateways (vLLM/Mistral family) that
// sometimes emit tool calls as plain text or markdown instead of API tool_calls.
func GetToolGuidanceForModel(model string) string {
	m := strings.ToLower(model)

	switch {
	case strings.Contains(m, "codestral"):
		return codestralToolGuidance
	case strings.Contains(m, "mistral-medium") || strings.Contains(m, "mistral-large") || strings.Contains(m, "magistral"):
		return mistralMediumToolGuidance
	case strings.Contains(m, "mistral-small") || strings.Contains(m, "mistral-nemo") || strings.Contains(m, "mistral-7b"):
		return mistralSmallToolGuidance
	default:
		return defaultToolGuidance
	}
}

const defaultToolGuidance = `When the user requests an action that requires a tool, you MUST call it using the tool_calls API — never describe a tool call in plain text or markdown code blocks.`

const codestralToolGuidance = `You are a code-focused assistant with access to tools. When a tool is needed, call it directly via the tool_calls API — never output a tool invocation as plain text, JSON code blocks, or markdown. When a tool call fails, read the error carefully before deciding how to proceed.`

const mistralMediumToolGuidance = `When the user requests an action that requires a tool, call the function directly using the tool_calls API. Do not narrate what you are about to do — execute the call. After receiving a tool result, analyze it carefully before deciding next steps.`

const mistralSmallToolGuidance = `Use tools by calling them via the tool_calls API. Never output a tool call as plain text or a code block. Call one tool at a time and wait for its result before proceeding.`
