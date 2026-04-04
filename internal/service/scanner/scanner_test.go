package scanner

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestSanitizeMessageHistoryDropsOrphanTool(t *testing.T) {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "prompt"},
		{Role: openai.ChatMessageRoleTool, Content: "orphan", ToolCallID: "call-1"},
	}

	sanitized, stats := sanitizeMessageHistory(messages)

	if len(sanitized) != 1 {
		t.Fatalf("expected 1 message after sanitization, got %d", len(sanitized))
	}
	if sanitized[0].Role != openai.ChatMessageRoleUser {
		t.Fatalf("expected user message to remain, got role %q", sanitized[0].Role)
	}
	if stats.droppedToolMessages != 1 {
		t.Fatalf("expected 1 dropped tool message, got %d", stats.droppedToolMessages)
	}
	if stats.droppedIncompleteRuns != 0 {
		t.Fatalf("expected 0 dropped incomplete rounds, got %d", stats.droppedIncompleteRuns)
	}
}

func TestSanitizeMessageHistoryDropsIncompleteToolRound(t *testing.T) {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "prompt"},
		{
			Role: openai.ChatMessageRoleAssistant,
			ToolCalls: []openai.ToolCall{
				{ID: "call-1", Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"a"}`}},
				{ID: "call-2", Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"b"}`}},
			},
		},
		{Role: openai.ChatMessageRoleTool, Content: "partial", ToolCallID: "call-1"},
		{Role: openai.ChatMessageRoleUser, Content: "next"},
	}

	sanitized, stats := sanitizeMessageHistory(messages)

	if len(sanitized) != 2 {
		t.Fatalf("expected incomplete tool round to be removed, got %d messages", len(sanitized))
	}
	if sanitized[0].Role != openai.ChatMessageRoleUser || sanitized[1].Role != openai.ChatMessageRoleUser {
		t.Fatalf("expected only user messages to remain, got roles %q and %q", sanitized[0].Role, sanitized[1].Role)
	}
	if stats.droppedIncompleteRuns != 1 {
		t.Fatalf("expected 1 dropped incomplete round, got %d", stats.droppedIncompleteRuns)
	}
	if stats.droppedToolMessages != 0 {
		t.Fatalf("expected 0 orphan tool drops, got %d", stats.droppedToolMessages)
	}
}

func TestSanitizeMessageHistoryKeepsCompleteToolRound(t *testing.T) {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "prompt"},
		{
			Role: openai.ChatMessageRoleAssistant,
			ToolCalls: []openai.ToolCall{
				{ID: "call-1", Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"a"}`}},
			},
		},
		{Role: openai.ChatMessageRoleTool, Content: "result", ToolCallID: "call-1"},
		{Role: openai.ChatMessageRoleAssistant, Content: "done"},
	}

	sanitized, stats := sanitizeMessageHistory(messages)

	if len(sanitized) != len(messages) {
		t.Fatalf("expected complete tool round to remain, got %d messages", len(sanitized))
	}
	if stats.changed() {
		t.Fatalf("expected no sanitization changes, got %+v", stats)
	}
}

func TestSelectCompressionWindowAlignsToToolRoundStart(t *testing.T) {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "prompt"},
		{Role: openai.ChatMessageRoleAssistant, Content: "thinking"},
		{
			Role: openai.ChatMessageRoleAssistant,
			ToolCalls: []openai.ToolCall{
				{ID: "call-1", Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"a"}`}},
				{ID: "call-2", Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"b"}`}},
			},
		},
		{Role: openai.ChatMessageRoleTool, Content: "result-a", ToolCallID: "call-1"},
		{Role: openai.ChatMessageRoleTool, Content: "result-b", ToolCallID: "call-2"},
		{Role: openai.ChatMessageRoleAssistant, Content: "done"},
		{Role: openai.ChatMessageRoleUser, Content: "next"},
	}

	selection := selectCompressionWindow(messages, 3)

	if selection.candidateStart != 4 {
		t.Fatalf("expected candidate start 4, got %d", selection.candidateStart)
	}
	if selection.candidateRole != openai.ChatMessageRoleTool {
		t.Fatalf("expected candidate role tool, got %q", selection.candidateRole)
	}
	if selection.adjustedStart != 2 {
		t.Fatalf("expected adjusted start 2, got %d", selection.adjustedStart)
	}
	if selection.usedFullHistory {
		t.Fatal("expected aligned tail to be used without full-history fallback")
	}
	if selection.tailSanitizeStats.changed() {
		t.Fatalf("expected aligned tail to remain valid, got %+v", selection.tailSanitizeStats)
	}
	if len(selection.selectedMessages) != 5 {
		t.Fatalf("expected 5 selected messages, got %d", len(selection.selectedMessages))
	}
	if len(selection.selectedMessages[0].ToolCalls) != 2 {
		t.Fatalf("expected selected window to start with assistant tool calls, got %+v", selection.selectedMessages[0])
	}
	if selection.selectedMessages[1].ToolCallID != "call-1" || selection.selectedMessages[2].ToolCallID != "call-2" {
		t.Fatalf("expected both tool outputs to remain in selected window, got %+v", selection.selectedMessages)
	}
}

func TestSelectCompressionWindowKeepsValidBoundary(t *testing.T) {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "prompt"},
		{
			Role: openai.ChatMessageRoleAssistant,
			ToolCalls: []openai.ToolCall{
				{ID: "call-1", Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"a"}`}},
			},
		},
		{Role: openai.ChatMessageRoleTool, Content: "result-a", ToolCallID: "call-1"},
		{Role: openai.ChatMessageRoleAssistant, Content: "done"},
		{Role: openai.ChatMessageRoleUser, Content: "next"},
	}

	selection := selectCompressionWindow(messages, 2)

	if selection.candidateStart != 3 || selection.adjustedStart != 3 {
		t.Fatalf("expected valid boundary to remain unchanged, got candidate=%d adjusted=%d", selection.candidateStart, selection.adjustedStart)
	}
	if selection.usedFullHistory {
		t.Fatal("expected no full-history fallback on valid boundary")
	}
	if len(selection.selectedMessages) != 2 {
		t.Fatalf("expected 2 selected messages, got %d", len(selection.selectedMessages))
	}
	if selection.selectedMessages[0].Role != openai.ChatMessageRoleAssistant || selection.selectedMessages[1].Role != openai.ChatMessageRoleUser {
		t.Fatalf("expected assistant/user tail, got %+v", selection.selectedMessages)
	}
}

func TestSelectCompressionWindowFallsBackToSanitizedFullHistory(t *testing.T) {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "prompt"},
		{
			Role: openai.ChatMessageRoleAssistant,
			ToolCalls: []openai.ToolCall{
				{ID: "call-1", Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"a"}`}},
			},
		},
		{Role: openai.ChatMessageRoleTool, Content: "mismatch", ToolCallID: "call-2"},
		{Role: openai.ChatMessageRoleAssistant, Content: "done"},
	}

	selection := selectCompressionWindow(messages, 2)

	if !selection.usedFullHistory {
		t.Fatal("expected invalid tail to trigger full-history fallback")
	}
	if !selection.tailSanitizeStats.changed() {
		t.Fatalf("expected tail sanitization to detect invalid tail, got %+v", selection.tailSanitizeStats)
	}
	if len(selection.selectedMessages) != 2 {
		t.Fatalf("expected fallback to sanitized full history with 2 messages, got %d", len(selection.selectedMessages))
	}
	if selection.selectedMessages[0].Role != openai.ChatMessageRoleUser || selection.selectedMessages[1].Role != openai.ChatMessageRoleAssistant {
		t.Fatalf("expected fallback history to keep prompt and final assistant, got %+v", selection.selectedMessages)
	}
}

func TestIsRetryableAIError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "timeout",
			err:       context.DeadlineExceeded,
			retryable: true,
		},
		{
			name:      "protocol 400",
			err:       errors.New("error, status code: 400, status: 400 Bad Request, message: No tool call found for function call output"),
			retryable: false,
		},
		{
			name:      "rate limit 429",
			err:       errors.New("error, status code: 429, status: 429 Too Many Requests"),
			retryable: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isRetryableAIError(tc.err)
			if got != tc.retryable {
				t.Fatalf("expected retryable=%v, got %v", tc.retryable, got)
			}
		})
	}
}

func TestResetConversationMessagesIncludesEvidenceIndex(t *testing.T) {
	messages := resetConversationMessages("prompt", "summary", "- ev-1 | api/user.go | lines 10-20 | 512 bytes")

	if len(messages) != 4 {
		t.Fatalf("expected 4 messages after reset with evidence index, got %d", len(messages))
	}
	if messages[2].Role != openai.ChatMessageRoleUser {
		t.Fatalf("expected evidence index message to be a user message, got %q", messages[2].Role)
	}
	if got := messages[2].Content; got == "" || !containsAll(got, "PRESERVED READ_FILE EVIDENCE INDEX", "ev-1", "api/user.go") {
		t.Fatalf("expected evidence index content in reset message, got %q", got)
	}
	if got := messages[3].Content; !containsAll(got, "get_evidence", "summary above") {
		t.Fatalf("expected continuation guidance to mention get_evidence, got %q", got)
	}
}

func TestAppendChineseNarrativeRulesIncludesLanguageRequirements(t *testing.T) {
	prompt := appendChineseNarrativeRules("BASE PROMPT", "rce")

	if !containsAll(prompt, "LANGUAGE REQUIREMENTS", "Simplified Chinese", "CURRENT STAGE: rce") {
		t.Fatalf("expected Chinese narrative guidance to be appended, got %q", prompt)
	}
}

func TestRepairChineseNarrativeRulesIncludesLanguageRequirements(t *testing.T) {
	prompt := repairChineseNarrativeRules("logic")

	if !containsAll(prompt, "LANGUAGE REQUIREMENTS", "Simplified Chinese", "CURRENT STAGE: logic") {
		t.Fatalf("expected repair guidance to preserve Chinese narrative rules, got %q", prompt)
	}
}

func TestEvidenceStorePreservesReadFileSnippet(t *testing.T) {
	store := newEvidenceStore(64)
	record := store.addReadFileEvidence("internal/service/foo.go", 10, 30, strings.Repeat("x", 80))

	if record.ID == "" {
		t.Fatal("expected evidence ID to be assigned")
	}
	if !record.Truncated {
		t.Fatal("expected oversized evidence payload to be marked truncated")
	}
	if _, ok := store.get(record.ID); !ok {
		t.Fatalf("expected evidence %q to be retrievable", record.ID)
	}

	index := store.compactIndex(5)
	if !containsAll(index, record.ID, "internal/service/foo.go", "lines 10-30") {
		t.Fatalf("expected compact index to include stored evidence, got %q", index)
	}

	payload := formatEvidencePayload(record)
	if !containsAll(payload, "EVIDENCE "+record.ID, "Range: lines 10-30", "Truncated: true") {
		t.Fatalf("expected formatted evidence payload to include metadata, got %q", payload)
	}
}

func TestDisplayToolPathReturnsRelativeProjectPath(t *testing.T) {
	basePath := t.TempDir()
	resolvedPath := filepath.Join(basePath, "internal", "service", "foo.go")

	got := displayToolPath(basePath, resolvedPath)

	if got != "internal/service/foo.go" {
		t.Fatalf("expected project-relative path, got %q", got)
	}
}

func TestDisplayToolPathFallsBackToSafeFileNameForOutsidePath(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "project")
	resolvedPath := filepath.Join(root, "other", "secret.go")

	got := displayToolPath(basePath, resolvedPath)

	if got != "secret.go" {
		t.Fatalf("expected safe filename fallback, got %q", got)
	}
	if strings.Contains(got, "..") || filepath.IsAbs(got) {
		t.Fatalf("expected non-absolute, non-escaping fallback path, got %q", got)
	}
}

func TestCalculateContextBytesIncludesToolCallArgumentsAndName(t *testing.T) {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "prompt"},
		{
			Role: openai.ChatMessageRoleAssistant,
			ToolCalls: []openai.ToolCall{
				{Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"a.go"}`}},
			},
		},
	}

	got := calculateContextBytes(messages)
	want := len("prompt") + len("read_file") + len(`{"path":"a.go"}`)
	if got != want {
		t.Fatalf("expected %d context bytes, got %d", want, got)
	}
}

func containsAll(input string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(input, part) {
			return false
		}
	}
	return true
}
