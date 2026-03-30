package converter

import (
	"testing"

	"github.com/claude-code-proxy/proxy/pkg/models"
)

// helper builds an OpenAIResponse with a single tool call
func singleToolCallResponse(id, name, args string) *models.OpenAIResponse {
	return &models.OpenAIResponse{
		Choices: []models.OpenAIChoice{
			{
				Message: models.OpenAIMessage{
					Role: "assistant",
					ToolCalls: []models.OpenAIToolCall{
						{
							ID:   id,
							Type: "function",
							Function: struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							}{Name: name, Arguments: args},
						},
					},
				},
			},
		},
	}
}

func TestRepairToolCalls_NilResponse(t *testing.T) {
	if RepairToolCalls(nil) {
		t.Error("expected false for nil response")
	}
}

func TestRepairToolCalls_NoToolCalls(t *testing.T) {
	resp := &models.OpenAIResponse{
		Choices: []models.OpenAIChoice{
			{Message: models.OpenAIMessage{Role: "assistant", Content: "Hello"}},
		},
	}
	if RepairToolCalls(resp) {
		t.Error("expected false when no tool calls present")
	}
}

func TestRepairToolCalls_CleanArgs_NotRepaired(t *testing.T) {
	resp := singleToolCallResponse("call_01", "read_file", `{"path":"/tmp/foo"}`)
	if RepairToolCalls(resp) {
		t.Error("expected false for clean JSON arguments")
	}
	if resp.Choices[0].Message.ToolCalls[0].Function.Arguments != `{"path":"/tmp/foo"}` {
		t.Error("clean arguments should be unchanged")
	}
}

func TestRepairToolCalls_MarkdownCodeBlock(t *testing.T) {
	resp := singleToolCallResponse("call_01", "read_file", "```json\n{\"path\":\"/tmp/foo\"}\n```")
	if !RepairToolCalls(resp) {
		t.Error("expected true: markdown code block should be repaired")
	}
	got := resp.Choices[0].Message.ToolCalls[0].Function.Arguments
	if got != `{"path":"/tmp/foo"}` {
		t.Errorf("unexpected repaired arguments: %q", got)
	}
}

func TestRepairToolCalls_MarkdownCodeBlockNoLang(t *testing.T) {
	resp := singleToolCallResponse("call_01", "write_file", "```\n{\"path\":\"/tmp/bar\",\"content\":\"hello\"}\n```")
	if !RepairToolCalls(resp) {
		t.Error("expected true: markdown code block (no lang) should be repaired")
	}
	got := resp.Choices[0].Message.ToolCalls[0].Function.Arguments
	if got != `{"path":"/tmp/bar","content":"hello"}` {
		t.Errorf("unexpected repaired arguments: %q", got)
	}
}

func TestRepairToolCalls_XMLArguments(t *testing.T) {
	resp := singleToolCallResponse("call_02", "bash", `<arguments>{"command":"ls -la"}</arguments>`)
	if !RepairToolCalls(resp) {
		t.Error("expected true: XML arguments should be repaired")
	}
	got := resp.Choices[0].Message.ToolCalls[0].Function.Arguments
	if got != `{"command":"ls -la"}` {
		t.Errorf("unexpected repaired arguments: %q", got)
	}
}

func TestRepairToolCalls_MissingID(t *testing.T) {
	resp := singleToolCallResponse("", "read_file", `{"path":"/tmp/foo"}`)
	if !RepairToolCalls(resp) {
		t.Error("expected true: missing ID should be repaired")
	}
	id := resp.Choices[0].Message.ToolCalls[0].ID
	if id == "" {
		t.Error("ID should have been set")
	}
}

func TestRepairToolCalls_WhitespaceOnly(t *testing.T) {
	resp := singleToolCallResponse("call_03", "list", "  {\"path\":\"/\"}\n  ")
	if !RepairToolCalls(resp) {
		t.Error("expected true: whitespace should be trimmed")
	}
	got := resp.Choices[0].Message.ToolCalls[0].Function.Arguments
	if got != `{"path":"/"}` {
		t.Errorf("unexpected repaired arguments: %q", got)
	}
}
