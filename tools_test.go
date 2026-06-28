package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// helper: create a fresh Storage for each test
func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	store, err := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// helper: build a CallToolRequest with arguments
func newToolRequest(args map[string]interface{}) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestMemorySaveTool(t *testing.T) {
	store := newTestStorage(t)
	tool := memorySaveTool(store)

	req := newToolRequest(map[string]interface{}{
		"key":   "memory/test/tool-save",
		"value": `{"content":"test content","tags":["test"]}`,
		"text":  "test content for embedding",
	})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	if res == nil {
		t.Fatal("Handler returned nil result")
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Memory saved") {
		t.Fatalf("Result = %q, want 'Memory saved'", text)
	}

	// Verify via Get
	got, _ := store.Get("memory/test/tool-save")
	if got == nil || got.Content != "test content" {
		t.Fatal("Save tool did not actually save")
	}
}

func TestMemorySaveToolAutoKey(t *testing.T) {
	store := newTestStorage(t)
	tool := memorySaveTool(store)

	req := newToolRequest(map[string]interface{}{
		"value":    `{"content":"auto-key test"}`,
		"text":     "auto-key test text",
		"auto_key": true,
	})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "memory/auto/") {
		t.Fatalf("Result = %q, want memory/auto/ key", text)
	}
}

func TestMemorySaveToolArbitraryJSON(t *testing.T) {
	store := newTestStorage(t)
	tool := memorySaveTool(store)
	value := `{"wife":"Эка","city":"Москва","beer":"Budweiser Budvar"}`

	req := newToolRequest(map[string]interface{}{
		"key":   "memory/test/arbitrary-json",
		"value": value,
		"text":  "arbitrary user data",
	})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Memory saved") {
		t.Fatalf("Result = %q, want 'Memory saved'", text)
	}

	// Verify the raw JSON value was preserved as Content
	got, _ := store.Get("memory/test/arbitrary-json")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Content != value {
		t.Fatalf("Content = %q, want %q", got.Content, value)
	}
}

func TestMemorySaveToolArbitraryJSONTimestamp(t *testing.T) {
	store := newTestStorage(t)
	tool := memorySaveTool(store)

	req := newToolRequest(map[string]interface{}{
		"key":   "memory/test/arbitrary-json-timestamp",
		"value": `{"foo":"bar","baz":123}`,
		"text":  "arbitrary user data",
	})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Memory saved") {
		t.Fatalf("Result = %q, want 'Memory saved'", text)
	}

	got, _ := store.Get("memory/test/arbitrary-json-timestamp")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Content == "" {
		t.Fatal("Content is empty — raw JSON was not preserved")
	}
	if got.Timestamp == "" {
		t.Fatal("Timestamp is empty — should be auto-populated by Save()")
	}
}

func TestMemorySaveToolStructuredValue(t *testing.T) {
	store := newTestStorage(t)
	tool := memorySaveTool(store)

	req := newToolRequest(map[string]interface{}{
		"key":   "memory/test/structured-value",
		"value": `{"content":"explicit content","summary":"test summary","tags":["test"]}`,
		"text":  "structured value test",
	})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Memory saved") {
		t.Fatalf("Result = %q, want 'Memory saved'", text)
	}

	got, _ := store.Get("memory/test/structured-value")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Content != "explicit content" {
		t.Fatalf("Content = %q, want 'explicit content'", got.Content)
	}
	if got.Summary != "test summary" {
		t.Fatalf("Summary = %q, want 'test summary'", got.Summary)
	}
}

func TestMemorySaveToolMissingValue(t *testing.T) {
	store := newTestStorage(t)
	tool := memorySaveTool(store)

	req := newToolRequest(map[string]interface{}{"key": "x"})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error message", text)
	}
}

func TestMemoryGetTool(t *testing.T) {
	store := newTestStorage(t)
	store.Save("memory/test/get", &MemoryValue{Content: "hello"}, "hello", false)

	tool := memoryGetTool(store)
	req := newToolRequest(map[string]interface{}{"key": "memory/test/get"})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "hello") {
		t.Fatalf("Result = %q, want hello", text)
	}
}

func TestMemoryGetToolMissingKey(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryGetTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestMemoryDeleteTool(t *testing.T) {
	store := newTestStorage(t)
	store.Save("memory/test/del", &MemoryValue{Content: "bye"}, "bye", false)

	tool := memoryDeleteTool(store)
	req := newToolRequest(map[string]interface{}{"key": "memory/test/del"})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Deleted") {
		t.Fatalf("Result = %q, want Deleted", text)
	}

	_, err = store.Get("memory/test/del")
	if err == nil {
		t.Fatal("Get after delete succeeded, want error")
	}
}

func TestMemoryDeleteToolEmptyKey(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryDeleteTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestMemoryListTool(t *testing.T) {
	store := newTestStorage(t)
	store.Save("memory/list/a", &MemoryValue{Content: "A"}, "A", false)
	store.Save("memory/list/b", &MemoryValue{Content: "B"}, "B", false)

	tool := memoryListTool(store)
	req := newToolRequest(map[string]interface{}{"prefix": "memory/list/"})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Found") {
		t.Fatalf("Result = %q, want Found", text)
	}
}

func TestMemoryListToolEmpty(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryListTool(store)

	req := newToolRequest(map[string]interface{}{"prefix": "memory/none/"})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No memories") {
		t.Fatalf("Result = %q, want No memories", text)
	}
}

func TestMemorySearchTool(t *testing.T) {
	store := newTestStorage(t)
	store.Save("memory/search/test", &MemoryValue{Content: "searchable content"}, "searchable content", false)

	tool := memorySearchTool(store)
	req := newToolRequest(map[string]interface{}{"query": "searchable", "limit": 5.0})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	// May or may not have results depending on Ollama
	text := res.Content[0].(mcp.TextContent).Text
	if text == "" {
		t.Fatal("Result is empty")
	}
}

func TestMemorySearchToolMissingQuery(t *testing.T) {
	store := newTestStorage(t)
	tool := memorySearchTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestMemoryGetContextTool(t *testing.T) {
	store := newTestStorage(t)
	store.Save("memory/ctx/test", &MemoryValue{Content: "context test"}, "context test", false)

	tool := memoryGetContextTool(store)
	req := newToolRequest(map[string]interface{}{"query": "context", "limit": 5.0})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if text == "" {
		t.Fatal("Result is empty")
	}
}

func TestMemoryGetContextToolMissingQuery(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryGetContextTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestMemoryExtractTool(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryExtractTool(store)

	req := newToolRequest(map[string]interface{}{
		"text":      "I love programming in Go.",
		"auto_save": false,
	})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	// Without Ollama may return "No facts extracted"
	text := res.Content[0].(mcp.TextContent).Text
	if text == "" {
		t.Fatal("Result is empty")
	}
}

func TestMemoryExtractToolMissingText(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryExtractTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestMemoryGoalCreateTool(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryGoalCreateTool(store)

	req := newToolRequest(map[string]interface{}{
		"title":       "Test Goal",
		"description": "Test desc",
		"priority":    8.0,
	})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Goal created") {
		t.Fatalf("Result = %q, want Goal created", text)
	}
}

func TestMemoryGoalCreateToolMissingTitle(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryGoalCreateTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestMemoryGoalListTool(t *testing.T) {
	store := newTestStorage(t)
	store.CreateGoal("Goal A", "", "", 5, nil)
	store.CreateGoal("Goal B", "", "", 5, nil)

	tool := memoryGoalListTool(store)
	req := newToolRequest(map[string]interface{}{})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "2") {
		t.Fatalf("Result = %q, want '2' goals", text)
	}
}

func TestMemoryGoalListToolEmpty(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryGoalListTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No goals") {
		t.Fatalf("Result = %q, want No goals", text)
	}
}

func TestMemoryGoalUpdateTool(t *testing.T) {
	store := newTestStorage(t)
	created, _ := store.CreateGoal("Old Title", "", "", 5, nil)

	tool := memoryGoalUpdateTool(store)
	req := newToolRequest(map[string]interface{}{
		"id":    created.ID,
		"title": "New Title",
	})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "New Title") {
		t.Fatalf("Result = %q, want New Title", text)
	}
}

func TestMemoryGoalUpdateToolMissingID(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryGoalUpdateTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestMemoryGoalDeleteTool(t *testing.T) {
	store := newTestStorage(t)
	created, _ := store.CreateGoal("To Delete", "", "", 5, nil)

	tool := memoryGoalDeleteTool(store)
	req := newToolRequest(map[string]interface{}{"id": created.ID})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Deleted goal") {
		t.Fatalf("Result = %q, want Deleted goal", text)
	}
}

func TestMemoryGoalDeleteToolMissingID(t *testing.T) {
	store := newTestStorage(t)
	tool := memoryGoalDeleteTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestMemoryTimelineTool(t *testing.T) {
	store := newTestStorage(t)
	store.LogEvent("test", "key", "summary", "details")

	tool := memoryTimelineTool(store)
	req := newToolRequest(map[string]interface{}{})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "timeline") {
		t.Fatalf("Result = %q, want timeline", text)
	}
}

func TestMemorySuggestTool(t *testing.T) {
	store := newTestStorage(t)
	tool := memorySuggestTool(store)

	req := newToolRequest(map[string]interface{}{"context": "test", "limit": 3.0})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	// May fail gracefully without Ollama
	text := res.Content[0].(mcp.TextContent).Text
	if text == "" {
		t.Fatal("Result is empty")
	}
}

func TestSessionSaveTool(t *testing.T) {
	store := newTestStorage(t)
	tool := sessionSaveTool(store)

	req := newToolRequest(map[string]interface{}{
		"project": "test-proj",
		"data":    `{"task":"test"}`,
	})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Session saved") {
		t.Fatalf("Result = %q, want Session saved", text)
	}
}

func TestSessionSaveToolMissingArgs(t *testing.T) {
	store := newTestStorage(t)
	tool := sessionSaveTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestSessionGetTool(t *testing.T) {
	store := newTestStorage(t)
	store.SessionSave("test-proj", []byte(`{"task":"test"}`))

	tool := sessionGetTool(store)
	req := newToolRequest(map[string]interface{}{"project": "test-proj"})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if text == "" {
		t.Fatal("Result is empty")
	}
}

func TestSessionGetToolMissingProject(t *testing.T) {
	store := newTestStorage(t)
	tool := sessionGetTool(store)

	req := newToolRequest(map[string]interface{}{})
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestSessionListTool(t *testing.T) {
	store := newTestStorage(t)
	store.SessionSave("proj-a", []byte("data-a"))
	store.SessionSave("proj-b", []byte("data-b"))

	tool := sessionListTool(store)
	req := newToolRequest(map[string]interface{}{})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Found") {
		t.Fatalf("Result = %q, want Found", text)
	}
}

func TestSessionCompactTool(t *testing.T) {
	store := newTestStorage(t)
	store.SessionSave("proj", []byte(`{"task":"test"}`))

	tool := sessionCompactTool(store)
	req := newToolRequest(map[string]interface{}{"max_age_hours": 0.0})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Compacted") {
		t.Fatalf("Result = %q, want Compacted", text)
	}
}

func TestToolsList(t *testing.T) {
	store := newTestStorage(t)
	list := tools(store)

	if len(list) == 0 {
		t.Fatal("tools() returned empty list")
	}
	if len(list) < 17 {
		t.Fatalf("tools() returned %d tools, want >= 17", len(list))
	}

	// Verify tool names
	names := make(map[string]bool)
	for _, st := range list {
		names[st.Tool.Name] = true
	}
	expected := []string{
		"memory_save", "memory_get", "memory_delete", "memory_search",
		"memory_list", "memory_get_context", "memory_extract",
		"memory_goal_create", "memory_goal_list", "memory_goal_update",
		"memory_goal_delete", "memory_timeline", "memory_suggest",
		"session_save", "session_get", "session_list", "session_compact",
	}
	for _, name := range expected {
		if !names[name] {
			t.Fatalf("tools() missing tool %q", name)
		}
	}
}

func TestLogWrap(t *testing.T) {
	store := newTestStorage(t)

	called := false
	wrapped := logWrap("test_tool", store, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("wrapped result"), nil
	})

	req := newToolRequest(map[string]interface{}{"key": "test-key"})
	res, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("logWrap error = %v", err)
	}
	if !called {
		t.Fatal("logWrap did not call inner handler")
	}
	text := res.Content[0].(mcp.TextContent).Text
	if text != "wrapped result" {
		t.Fatalf("Result = %q, want wrapped result", text)
	}
}

func TestMemorySaveToolInvalidJSON(t *testing.T) {
	store := newTestStorage(t)
	tool := memorySaveTool(store)

	req := newToolRequest(map[string]interface{}{
		"key":   "x",
		"value": `not valid json`,
		"text":  "text",
	})

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("Handler error = %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Fatalf("Result = %q, want error", text)
	}
}

func TestMemoryResourcesJSON(t *testing.T) {
	store := newTestStorage(t)
	store.Save("memory/test/res", &MemoryValue{Content: "resource test"}, "resource test", false)
	store.LogEvent("test", "key", "summary", "details")

	resources := memoryResourcesJSON(store)
	if len(resources) == 0 {
		t.Fatal("memoryResourcesJSON() returned empty map")
	}
	for _, key := range []string{"memory://context/current", "memory://goals/active", "memory://timeline/today"} {
		if _, ok := resources[key]; !ok {
			t.Fatalf("memoryResourcesJSON() missing key %q", key)
		}
	}
}
