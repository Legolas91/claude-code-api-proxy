package converter

import (
	"strings"
	"testing"
)

func TestGetToolGuidanceForModel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		contains string // substring expected in the returned guidance
	}{
		{
			name:     "codestral model",
			model:    "codestral-2503",
			contains: "code-focused assistant",
		},
		{
			name:     "codestral case-insensitive",
			model:    "Codestral-2508",
			contains: "code-focused assistant",
		},
		{
			name:     "mistral-medium model",
			model:    "mistral-medium-2508",
			contains: "analyze it carefully",
		},
		{
			name:     "mistral-large model (same tier as medium)",
			model:    "mistral-large-2502",
			contains: "analyze it carefully",
		},
		{
			name:     "magistral model",
			model:    "magistral-medium-2509",
			contains: "analyze it carefully",
		},
		{
			name:     "mistral-small model",
			model:    "mistral-small-2506",
			contains: "Call one tool at a time",
		},
		{
			name:     "mistral-nemo model",
			model:    "mistral-nemo-12b",
			contains: "Call one tool at a time",
		},
		{
			name:     "unknown model falls back to default",
			model:    "gpt-5",
			contains: "tool_calls API",
		},
		{
			name:     "empty model falls back to default",
			model:    "",
			contains: "tool_calls API",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetToolGuidanceForModel(tt.model)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("GetToolGuidanceForModel(%q) = %q, want it to contain %q", tt.model, result, tt.contains)
			}
			if result == "" {
				t.Errorf("GetToolGuidanceForModel(%q) returned empty string", tt.model)
			}
		})
	}
}
