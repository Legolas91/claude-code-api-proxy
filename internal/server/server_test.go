package server

import (
	"testing"
)

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.2.49", "1.2.49"},
		{"v1.2.49", "1.2.49"},
		{"2.1.49 (Claude Code)", "2.1.49"},
		{"Claude Code 1.5.0", "1.5.0"},
		{"claude-code/1.0.0 node/20.0.0", "1.0.0"},
		{"", ""},
		{"no version here", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractVersion(tt.input)
			if got != tt.expected {
				t.Errorf("extractVersion(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
