package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codescan/internal/config"
	"codescan/internal/model"

	"github.com/sashabaranov/go-openai"
)

func useTestScannerConfig(t *testing.T) {
	t.Helper()
	config.Scanner, _ = config.NormalizeScannerConfig(config.ScannerConfig{
		ContextCompression: config.ContextCompressionConfig{
			Enabled:                true,
			SoftLimitBytes:         2048,
			HardLimitBytes:         4096,
			SummaryWindowMessages:  8,
			MicrocompactKeepRecent: 1,
			ArtifactMaxBytes:       512,
			CompactMinTailMessages: 2,
			SessionMemoryEnabled:   true,
		},
		SessionMemory: config.SessionMemoryConfig{
			Enabled:                true,
			MinGrowthBytes:         1,
			MinToolCalls:           1,
			MaxUpdateBytes:         2048,
			RequestTimeoutSeconds:  60,
			MaxRetries:             3,
			RetryBackoffSeconds:    2,
			FailureCooldownSeconds: 300,
		},
	})
}

type testHTTPDoer func(req *http.Request) (*http.Response, error)

func (d testHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	return d(req)
}

func newTestAIClient(doer openai.HTTPDoer) *openai.Client {
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = "http://example.com/v1"
	cfg.HTTPClient = doer
	return openai.NewClientWithConfig(cfg)
}

func newChatCompletionResponse(t *testing.T, content string) *http.Response {
	t.Helper()

	payload := openai.ChatCompletionResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "gpt-4o-mini",
		Choices: []openai.ChatCompletionChoice{
			{
				Index: 0,
				Message: openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: content,
				},
				FinishReason: openai.FinishReasonStop,
			},
		},
		Usage: openai.Usage{
			PromptTokens:     1,
			CompletionTokens: 1,
			TotalTokens:      2,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal chat completion response: %v", err)
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}
}

func newTestTask(t *testing.T) *model.Task {
	t.Helper()
	base := t.TempDir()
	task := &model.Task{
		ID:       "task-test",
		BasePath: base,
	}
	if err := os.MkdirAll(task.RuntimeRootPath(), 0o755); err != nil {
		t.Fatalf("create runtime root: %v", err)
	}
	return task
}

func TestPreservedReadFileArtifactLogUsesRelativePath(t *testing.T) {
	useTestScannerConfig(t)
	task := newTestTask(t)
	session, err := newScanSession(task, "auth", "prompt", true)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	resolvedPath := filepath.Join(task.BasePath, "internal", "service", "scanner.go")
	record, err := session.createArtifact(
		"read_file",
		"read_file",
		displayToolPath(task.BasePath, resolvedPath),
		12,
		34,
		"package scanner",
	)
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	logLine := fmt.Sprintf("Preserved read_file artifact %s for %s (%s).", record.ID, record.Path, formatLineRange(record.StartLine, record.EndLine))

	if !strings.Contains(logLine, "internal/service/scanner.go") {
		t.Fatalf("expected log line to include relative path, got %q", logLine)
	}
	if strings.Contains(logLine, task.BasePath) || strings.Contains(logLine, filepath.ToSlash(task.BasePath)) {
		t.Fatalf("expected log line to avoid absolute base path, got %q", logLine)
	}
}

func TestSelectResumableRuntimeStageChoosesNewestResumableState(t *testing.T) {
	task := newTestTask(t)

	writeState := func(stage, status string, updatedAt time.Time) {
		stageDir := task.StageRuntimePath(stage)
		if err := os.MkdirAll(stageDir, 0o755); err != nil {
			t.Fatalf("create stage dir: %v", err)
		}
		state := runtimeState{
			Version:   1,
			TaskID:    task.ID,
			Stage:     stage,
			Status:    status,
			UpdatedAt: updatedAt,
		}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("marshal state: %v", err)
		}
		if err := os.WriteFile(filepath.Join(stageDir, runtimeStateFile), data, 0o644); err != nil {
			t.Fatalf("write state: %v", err)
		}
	}

	writeState("init", runtimeStatusPaused, time.Now().Add(-2*time.Minute))
	writeState("auth", runtimeStatusRunning, time.Now())
	writeState("xss", runtimeStatusFailed, time.Now().Add(2*time.Minute))

	stage, err := selectResumableRuntimeStage(task)
	if err != nil {
		t.Fatalf("select resumable stage: %v", err)
	}
	if stage != "xss" {
		t.Fatalf("expected xss to win, got %q", stage)
	}
}

func TestActiveMessagesIncludePreservedSegmentAfterBoundary(t *testing.T) {
	useTestScannerConfig(t)
	task := newTestTask(t)
	session, err := newScanSession(task, "auth", "prompt", true)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	if err := session.appendSynthetic(openai.ChatMessageRoleAssistant, runtimeKindNormal, "old-a", nil); err != nil {
		t.Fatalf("append old-a: %v", err)
	}
	oldA := session.transcript[len(session.transcript)-1]
	if err := session.appendSynthetic(openai.ChatMessageRoleUser, runtimeKindNormal, "old-b", nil); err != nil {
		t.Fatalf("append old-b: %v", err)
	}
	oldB := session.transcript[len(session.transcript)-1]
	if err := session.appendSynthetic(openai.ChatMessageRoleSystem, runtimeKindCompactBoundary, "", &compactBoundary{
		Source: "test",
		HeadID: oldA.ID,
		TailID: oldB.ID,
	}); err != nil {
		t.Fatalf("append boundary: %v", err)
	}
	if err := session.appendSynthetic(openai.ChatMessageRoleUser, runtimeKindCompactSummary, "summary", nil); err != nil {
		t.Fatalf("append summary: %v", err)
	}

	active := session.activeMessages()
	if len(active) != 3 {
		t.Fatalf("expected preserved segment + summary, got %d messages", len(active))
	}
	if active[0].Content != "old-a" || active[1].Content != "old-b" || active[2].Content != "summary" {
		t.Fatalf("unexpected active message sequence: %+v", active)
	}
}

func TestTruncateLastToolMessageStoresArtifact(t *testing.T) {
	useTestScannerConfig(t)
	task := newTestTask(t)
	session, err := newScanSession(task, "auth", "prompt", true)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	if err := session.appendToolMessage("call-1", "grep_files", strings.Repeat("x", 120), ""); err != nil {
		t.Fatalf("append tool message: %v", err)
	}

	originalBytes, updatedBytes, changed, err := session.truncateLastToolMessage(40)
	if err != nil {
		t.Fatalf("truncate last tool message: %v", err)
	}
	if !changed {
		t.Fatal("expected truncateLastToolMessage to report a change")
	}
	if originalBytes <= updatedBytes {
		t.Fatalf("expected updated bytes to be smaller, got original=%d updated=%d", originalBytes, updatedBytes)
	}
	last := session.transcript[len(session.transcript)-1]
	if last.ArtifactID == "" {
		t.Fatal("expected truncated tool message to reference an artifact")
	}
	if _, ok := session.loadArtifact(last.ArtifactID); !ok {
		t.Fatalf("expected artifact %q to be retrievable", last.ArtifactID)
	}
}

func TestApplyMicrocompactClearsOlderToolResultsOnly(t *testing.T) {
	useTestScannerConfig(t)
	task := newTestTask(t)
	session, err := newScanSession(task, "auth", "prompt", true)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	oldRound := openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleAssistant,
		ToolCalls: []openai.ToolCall{
			{ID: "call-1", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"a.go"}`}},
		},
	}
	if err := session.appendChatMessage(oldRound); err != nil {
		t.Fatalf("append old assistant round: %v", err)
	}
	if err := session.appendToolMessage("call-1", "read_file", "older result", ""); err != nil {
		t.Fatalf("append old tool result: %v", err)
	}
	oldToolID := session.transcript[len(session.transcript)-1].ID

	newRound := openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleAssistant,
		ToolCalls: []openai.ToolCall{
			{ID: "call-2", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "grep_files", Arguments: `{"pattern":"TODO"}`}},
		},
	}
	if err := session.appendChatMessage(newRound); err != nil {
		t.Fatalf("append new assistant round: %v", err)
	}
	if err := session.appendToolMessage("call-2", "grep_files", "new result", ""); err != nil {
		t.Fatalf("append new tool result: %v", err)
	}
	newToolID := session.transcript[len(session.transcript)-1].ID

	entries, err := session.applyMicrocompact(session.buildChatEntries(), nil)
	if err != nil {
		t.Fatalf("apply microcompact: %v", err)
	}
	if _, ok := session.state.Microcompact.ClearedMessages[oldToolID]; !ok {
		t.Fatal("expected older tool result to be microcompacted")
	}
	if _, ok := session.state.Microcompact.ClearedMessages[newToolID]; ok {
		t.Fatal("expected most recent tool round to remain intact")
	}
	foundPlaceholder := false
	for _, entry := range entries {
		if entry.Message.ID == oldToolID && strings.Contains(entry.Chat.Content, "get_artifact") {
			foundPlaceholder = true
		}
	}
	if !foundPlaceholder {
		t.Fatal("expected returned chat entries to contain an artifact recovery placeholder")
	}
}

func TestTrySessionMemoryCompactionKeepsOnlyMessagesAfterMemoryCursor(t *testing.T) {
	useTestScannerConfig(t)
	task := newTestTask(t)
	session, err := newScanSession(task, "auth", "prompt", true)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	if err := session.appendSynthetic(openai.ChatMessageRoleAssistant, runtimeKindNormal, "before-memory", nil); err != nil {
		t.Fatalf("append before-memory: %v", err)
	}
	beforeMemoryID := session.transcript[len(session.transcript)-1].ID
	if err := session.appendSynthetic(openai.ChatMessageRoleUser, runtimeKindNormal, "after-memory", nil); err != nil {
		t.Fatalf("append after-memory: %v", err)
	}
	if err := session.writeMemory("# Memory\nKnown context"); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	session.state.LastMemoryMessageID = beforeMemoryID
	if err := session.saveState(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	entries := session.buildChatEntries()[1:]
	summary, kept, ok := session.trySessionMemoryCompaction(entries)
	if !ok {
		t.Fatal("expected session memory compaction to activate")
	}
	if !strings.Contains(summary, "SESSION MEMORY SNAPSHOT") {
		t.Fatalf("expected memory-based summary, got %q", summary)
	}
	if len(kept) != 1 || kept[0].Chat.Content != "after-memory" {
		t.Fatalf("expected only post-memory messages to remain, got %+v", kept)
	}
}

func TestMaybeUpdateSessionMemorySendsOnlyDeltaAndClearsFailureState(t *testing.T) {
	useTestScannerConfig(t)
	config.AI = config.AIConfig{Model: "gpt-4o-mini"}
	config.Scanner.SessionMemory.MaxRetries = 1
	config.Scanner.SessionMemory.RetryBackoffSeconds = 0
	config.Scanner.SessionMemory.FailureCooldownSeconds = 60

	task := newTestTask(t)
	session, err := newScanSession(task, "auth", "prompt", true)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	if err := session.appendSynthetic(openai.ChatMessageRoleAssistant, runtimeKindNormal, "before-memory", nil); err != nil {
		t.Fatalf("append before-memory: %v", err)
	}
	beforeMemoryID := session.transcript[len(session.transcript)-1].ID
	if err := session.writeMemory("# Memory\nKnown context"); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	session.state.LastMemoryMessageID = beforeMemoryID
	session.state.ConsecutiveMemoryFailures = 2
	session.state.LastMemoryFailureAt = time.Now().Add(-2 * time.Minute)
	if err := session.saveState(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	toolRound := openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleAssistant,
		ToolCalls: []openai.ToolCall{
			{ID: "call-1", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"delta.go"}`}},
		},
	}
	if err := session.appendChatMessage(toolRound); err != nil {
		t.Fatalf("append tool round: %v", err)
	}
	if err := session.appendToolMessage("call-1", "read_file", "delta result", ""); err != nil {
		t.Fatalf("append tool message: %v", err)
	}
	lastDeltaID := session.transcript[len(session.transcript)-1].ID

	var requestContent string
	client := newTestAIClient(testHTTPDoer(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		if len(payload.Messages) != 1 {
			t.Fatalf("expected one message in memory update request, got %d", len(payload.Messages))
		}
		requestContent = payload.Messages[0].Content
		return newChatCompletionResponse(t, "# Updated Memory"), nil
	}))

	session.maybeUpdateSessionMemory(context.Background(), client, nil)

	if strings.Contains(requestContent, "before-memory") {
		t.Fatalf("expected session memory request to omit pre-cursor content, got %q", requestContent)
	}
	if !strings.Contains(requestContent, "delta result") || !strings.Contains(requestContent, `read_file {"path":"delta.go"}`) {
		t.Fatalf("expected delta content in memory update request, got %q", requestContent)
	}
	if session.state.LastMemoryMessageID != lastDeltaID {
		t.Fatalf("expected cursor to advance to %q, got %q", lastDeltaID, session.state.LastMemoryMessageID)
	}
	if session.state.ConsecutiveMemoryFailures != 0 {
		t.Fatalf("expected failure count reset, got %d", session.state.ConsecutiveMemoryFailures)
	}
	if !session.state.LastMemoryFailureAt.IsZero() {
		t.Fatalf("expected last failure time to reset, got %v", session.state.LastMemoryFailureAt)
	}
	memory, err := session.readMemory()
	if err != nil {
		t.Fatalf("read memory: %v", err)
	}
	if strings.TrimSpace(memory) != "# Updated Memory" {
		t.Fatalf("expected updated memory content, got %q", memory)
	}
}

func TestMaybeUpdateSessionMemoryRetriesAndCoolsDownAfterFailure(t *testing.T) {
	useTestScannerConfig(t)
	config.AI = config.AIConfig{Model: "gpt-4o-mini"}
	config.Scanner.SessionMemory.MaxRetries = 3
	config.Scanner.SessionMemory.RetryBackoffSeconds = 0
	config.Scanner.SessionMemory.FailureCooldownSeconds = 60

	task := newTestTask(t)
	session, err := newScanSession(task, "auth", "prompt", true)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	toolRound := openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleAssistant,
		ToolCalls: []openai.ToolCall{
			{ID: "call-1", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "grep_files", Arguments: `{"pattern":"TODO"}`}},
		},
	}
	if err := session.appendChatMessage(toolRound); err != nil {
		t.Fatalf("append tool round: %v", err)
	}
	if err := session.appendToolMessage("call-1", "grep_files", "delta result", ""); err != nil {
		t.Fatalf("append tool message: %v", err)
	}

	callCount := 0
	logs := []string{}
	logFunc := func(msg string) {
		logs = append(logs, msg)
	}
	client := newTestAIClient(testHTTPDoer(func(req *http.Request) (*http.Response, error) {
		callCount++
		return nil, context.DeadlineExceeded
	}))

	session.maybeUpdateSessionMemory(context.Background(), client, logFunc)

	if callCount != 3 {
		t.Fatalf("expected 3 total memory update attempts, got %d", callCount)
	}
	if session.state.ConsecutiveMemoryFailures != 1 {
		t.Fatalf("expected one consecutive memory failure, got %d", session.state.ConsecutiveMemoryFailures)
	}
	if session.state.LastMemoryFailureAt.IsZero() {
		t.Fatal("expected last failure time to be recorded")
	}

	retryLogs := 0
	for _, line := range logs {
		if strings.Contains(line, "Retrying") {
			retryLogs++
		}
	}
	if retryLogs != 2 {
		t.Fatalf("expected 2 retry logs for 3 attempts, got %d (%v)", retryLogs, logs)
	}

	session.maybeUpdateSessionMemory(context.Background(), client, logFunc)
	session.maybeUpdateSessionMemory(context.Background(), client, logFunc)

	if callCount != 3 {
		t.Fatalf("expected cooldown to suppress new HTTP calls, got %d", callCount)
	}

	cooldownLogs := 0
	for _, line := range logs {
		if strings.Contains(line, "cooling down") {
			cooldownLogs++
		}
	}
	if cooldownLogs != 1 {
		t.Fatalf("expected exactly one cooldown log, got %d (%v)", cooldownLogs, logs)
	}
}
