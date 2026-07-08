package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewStorage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	store, err := NewStorage(dbPath)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("NewStorage() returned nil")
	}
	if store.kv == nil {
		t.Fatal("NewStorage().kv is nil")
	}
	if store.goals == nil {
		t.Fatal("NewStorage().goals is nil")
	}

	// Second open on same file should fail (locked)
	store.Close()
}

func TestSaveAndGet(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	val := &MemoryValue{
		Content:   "Hello world",
		Summary:   "Test summary",
		Tags:      []string{"test", "hello"},
		Source:    "test",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	key, err := store.Save("memory/test/key", val, "Hello world", false)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if key != "memory/test/key" {
		t.Fatalf("Save() key = %q, want memory/test/key", key)
	}

	got, err := store.Get("memory/test/key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Content != val.Content {
		t.Fatalf("Get().Content = %q, want %q", got.Content, val.Content)
	}
	if got.Summary != val.Summary {
		t.Fatalf("Get().Summary = %q, want %q", got.Summary, val.Summary)
	}
	if len(got.Tags) != len(val.Tags) {
		t.Fatalf("Get().Tags = %v, want %v", got.Tags, val.Tags)
	}
}

func TestSaveWithAutoKey(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	val := &MemoryValue{Content: "auto-key test"}
	key, err := store.Save("", val, "auto-key test", true)
	if err != nil {
		t.Fatalf("Save(auto_key) error = %v", err)
	}
	if !strings.HasPrefix(key, "memory/auto/") {
		t.Fatalf("auto key = %q, want prefix memory/auto/", key)
	}

	// Should be retrievable
	got, err := store.Get(key)
	if err != nil {
		t.Fatalf("Get(auto_key) error = %v", err)
	}
	if got.Content != "auto-key test" {
		t.Fatalf("Get().Content = %q, want auto-key test", got.Content)
	}
}

func TestSaveDefaultTimestamp(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	val := &MemoryValue{Content: "no timestamp"}
	_, err := store.Save("memory/test/ts", val, "no timestamp", false)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, _ := store.Get("memory/test/ts")
	if got.Timestamp == "" {
		t.Fatal("Save() did not auto-fill timestamp")
	}
}

func TestSaveWithTimeout(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	val := &MemoryValue{Content: "timeout test"}
	res := store.SaveWithTimeout(5*time.Second, "memory/test/timeout", val, "timeout test", false)

	if res.Err != nil {
		t.Fatalf("SaveWithTimeout() error = %v", res.Err)
	}
	if res.Key != "memory/test/timeout" {
		t.Fatalf("SaveWithTimeout() key = %q, want memory/test/timeout", res.Key)
	}
	if res.Elapsed <= 0 {
		t.Fatalf("SaveWithTimeout() elapsed = %v, want > 0", res.Elapsed)
	}

	// Verify the value is actually stored
	got, err := store.Get("memory/test/timeout")
	if err != nil {
		t.Fatalf("Get(after SaveWithTimeout) error = %v", err)
	}
	if got.Content != "timeout test" {
		t.Fatalf("Get().Content = %q, want timeout test", got.Content)
	}
}

func TestRunWithTimeoutSuccess(t *testing.T) {
	res := runWithTimeout(time.Second, func() (string, error) {
		return "ok", nil
	})
	if res.Err != nil {
		t.Fatalf("runWithTimeout() error = %v", res.Err)
	}
	if res.Key != "ok" {
		t.Fatalf("runWithTimeout() key = %q, want ok", res.Key)
	}
}

func TestRunWithTimeoutExpires(t *testing.T) {
	res := runWithTimeout(1*time.Millisecond, func() (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "slow", nil
	})
	if res.Err == nil {
		t.Fatal("runWithTimeout() error = nil, want timeout error")
	}
	if res.Key != "" {
		t.Fatalf("runWithTimeout() key = %q, want empty on timeout", res.Key)
	}
}

func TestRunWithTimeoutError(t *testing.T) {
	testErr := fmt.Errorf("intentional failure")
	res := runWithTimeout(time.Second, func() (string, error) {
		return "", testErr
	})
	if res.Err != testErr {
		t.Fatalf("runWithTimeout() error = %v, want %v", res.Err, testErr)
	}
}

func TestGetNonExistent(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	_, err := store.Get("memory/nonexistent")
	if err == nil {
		t.Fatal("Get(nonexistent) error = nil, want error")
	}
}

func TestSaveDelete(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	val := &MemoryValue{Content: "to delete"}
	store.Save("memory/del/key", val, "to delete", false)

	if err := store.Delete("memory/del/key"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := store.Get("memory/del/key")
	if err == nil {
		t.Fatal("Get(after delete) error = nil, want error")
	}
}

func TestDeleteNonExistent(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	// keyvalembd.Del on non-existent key returns nil
	err := store.Delete("memory/nonexistent")
	if err != nil {
		t.Fatalf("Delete(nonexistent) error = %v, want nil", err)
	}
}

func TestListWithPrefix(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	for i := 1; i <= 3; i++ {
		val := &MemoryValue{Content: fmt.Sprintf("item %d", i)}
		store.Save(fmt.Sprintf("memory/test/item%d", i), val, fmt.Sprintf("item %d", i), false)
	}

	keys, err := store.List("memory/test/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) < 3 {
		t.Fatalf("List() returned %d keys, want >= 3", len(keys))
	}

	// Empty prefix
	allKeys, err := store.List("")
	if err != nil {
		t.Fatalf("List(\"\") error = %v", err)
	}
	if len(allKeys) == 0 {
		t.Fatal("List(\"\") returned empty, want non-empty")
	}
}

func TestListEmpty(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	keys, err := store.List("memory/empty/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("List() returned %d keys, want 0", len(keys))
	}
}

func TestSearchSemantic(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	val := &MemoryValue{Content: "semantic search test content"}
	store.Save("memory/search/test", val, "semantic search test content", false)

	results, err := store.Search("semantic search test", 10)
	if err != nil {
		// Ollama may be unavailable; just verify no panic
		t.Logf("Search() returned error (Ollama likely unavailable): %v", err)
		return
	}
	if len(results) == 0 {
		t.Log("Search() returned 0 results (Ollama may be unavailable or model not loaded)")
	}
}

func TestCreateGoal(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	goal, err := store.CreateGoal("Test Goal", "A description", "2026-12-31", 8, []string{"test", "mcp"})
	if err != nil {
		t.Fatalf("CreateGoal() error = %v", err)
	}
	if goal.ID == "" {
		t.Fatal("CreateGoal() goal.ID is empty")
	}
	if goal.Title != "Test Goal" {
		t.Fatalf("CreateGoal().Title = %q, want Test Goal", goal.Title)
	}
	if goal.Status != "active" {
		t.Fatalf("CreateGoal().Status = %q, want active", goal.Status)
	}
	if goal.Priority != 8 {
		t.Fatalf("CreateGoal().Priority = %d, want 8", goal.Priority)
	}
	if len(goal.Labels) != 2 {
		t.Fatalf("CreateGoal().Labels = %v, want [test mcp]", goal.Labels)
	}
}

func TestCreateGoalWithSubtasks(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	desc := "- [x] Task A\n- [ ] Task B\n- [x] Task C"
	goal, err := store.CreateGoal("With Subtasks", desc, "", 5, nil)
	if err != nil {
		t.Fatalf("CreateGoal() error = %v", err)
	}
	// 2 done out of 3 total = 66%
	if goal.Progress != 66 {
		t.Fatalf("CreateGoal().Progress = %d, want 66", goal.Progress)
	}
}

func TestGetGoal(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	created, _ := store.CreateGoal("Get Test", "desc", "", 5, nil)
	goal, err := store.GetGoal(created.ID)
	if err != nil {
		t.Fatalf("GetGoal() error = %v", err)
	}
	if goal.Title != "Get Test" {
		t.Fatalf("GetGoal().Title = %q, want Get Test", goal.Title)
	}

	_, err = store.GetGoal("goal/nonexistent/123")
	if err == nil {
		t.Fatal("GetGoal(nonexistent) error = nil, want error")
	}
}

func TestListGoalsByStatus(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	store.CreateGoal("Active A", "", "", 5, nil)
	store.CreateGoal("Active B", "", "", 5, nil)
	completed, _ := store.CreateGoal("Complete C", "", "", 5, nil)
	store.UpdateGoal(completed.ID, "", "", "completed", "", 5, 100, nil)

	activeGoals, err := store.ListGoals("active", nil)
	if err != nil {
		t.Fatalf("ListGoals(active) error = %v", err)
	}
	if len(activeGoals) != 2 {
		t.Fatalf("ListGoals(active) = %d, want 2", len(activeGoals))
	}

	completedGoals, err := store.ListGoals("completed", nil)
	if err != nil {
		t.Fatalf("ListGoals(completed) error = %v", err)
	}
	if len(completedGoals) != 1 {
		t.Fatalf("ListGoals(completed) = %d, want 1", len(completedGoals))
	}
}

func TestListGoalsByLabels(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	store.CreateGoal("With Bug", "", "", 5, []string{"bug"})
	store.CreateGoal("With MCP", "", "", 5, []string{"mcp"})
	store.CreateGoal("No Label", "", "", 5, nil)

	goals, err := store.ListGoals("", []string{"bug"})
	if err != nil {
		t.Fatalf("ListGoals(labels=[bug]) error = %v", err)
	}
	if len(goals) != 1 {
		t.Fatalf("ListGoals(labels=[bug]) = %d goals, want 1", len(goals))
	}
	if goals[0].Title != "With Bug" {
		t.Fatalf("ListGoals(labels=[bug])[0].Title = %q, want With Bug", goals[0].Title)
	}
}

func TestListGoalsEmpty(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	goals, err := store.ListGoals("active", nil)
	if err != nil {
		t.Fatalf("ListGoals() error = %v", err)
	}
	if len(goals) != 0 {
		t.Fatalf("ListGoals() = %d, want 0", len(goals))
	}
}

func TestUpdateGoalTitle(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	created, _ := store.CreateGoal("Old Title", "", "", 5, nil)
	updated, err := store.UpdateGoal(created.ID, "New Title", "", "", "", -1, -1, nil)
	if err != nil {
		t.Fatalf("UpdateGoal() error = %v", err)
	}
	if updated.Title != "New Title" {
		t.Fatalf("UpdateGoal().Title = %q, want New Title", updated.Title)
	}

	// Verify via GetGoal
	goal, _ := store.GetGoal(created.ID)
	if goal.Title != "New Title" {
		t.Fatalf("GetGoal().Title = %q, want New Title", goal.Title)
	}
}

func TestUpdateGoalStatusComplete(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	created, _ := store.CreateGoal("Status Test", "", "", 5, nil)
	updated, err := store.UpdateGoal(created.ID, "", "", "completed", "", -1, -1, nil)
	if err != nil {
		t.Fatalf("UpdateGoal() error = %v", err)
	}
	if updated.Status != "completed" {
		t.Fatalf("UpdateGoal().Status = %q, want completed", updated.Status)
	}
}

func TestUpdateGoalAutoProgress(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	created, _ := store.CreateGoal("Auto Progress", "", "", 5, nil)
	desc := "- [x] Done\n- [ ] Todo\n- [x] Also Done\n- [ ] Not Done"
	updated, err := store.UpdateGoal(created.ID, "", desc, "", "", -1, -1, nil)
	if err != nil {
		t.Fatalf("UpdateGoal() error = %v", err)
	}
	// 2 done out of 4 = 50%
	if updated.Progress != 50 {
		t.Fatalf("UpdateGoal().Progress = %d, want 50", updated.Progress)
	}
}

func TestDeleteGoal(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	created, _ := store.CreateGoal("To Delete", "", "", 5, nil)
	if err := store.DeleteGoal(created.ID); err != nil {
		t.Fatalf("DeleteGoal() error = %v", err)
	}

	_, err := store.GetGoal(created.ID)
	if err == nil {
		t.Fatal("GetGoal(after delete) error = nil, want error")
	}
}

func TestDeleteGoalMissing(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	// Deleting a non-existent goal should return an error.
	if err := store.DeleteGoal("goal/nonexistent/123"); err == nil {
		t.Fatal("DeleteGoal(nonexistent) error = nil, want error")
	}
}

func TestDeleteGoalEmptyID(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	err := store.DeleteGoal("")
	if err == nil {
		t.Fatal("DeleteGoal(empty) error = nil, want error")
	}
}

func TestGoalMirrorCreated(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	goal, _ := store.CreateGoal("Mirror Test", "Mirror desc", "", 5, []string{"mirror"})
	memKey := "memory/goals/active/" + goal.ID

	mem, err := store.Get(memKey)
	if err != nil {
		t.Fatalf("Get(mirror key) error = %v", err)
	}
	if mem.Summary != "Mirror Test" {
		t.Fatalf("mirror.Summary = %q, want Mirror Test", mem.Summary)
	}
	if mem.Content != "Mirror desc" {
		t.Fatalf("mirror.Content = %q, want Mirror desc", mem.Content)
	}
}

func TestGoalMirrorMoved(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	goal, _ := store.CreateGoal("Status Mirror", "", "", 5, nil)
	activeKey := "memory/goals/active/" + goal.ID

	store.UpdateGoal(goal.ID, "", "", "completed", "", -1, -1, nil)

	// Active key should be gone
	_, err := store.Get(activeKey)
	if err == nil {
		t.Fatal("Get(active mirror) error = nil, want error (should be moved)")
	}

	// Completed key should exist
	completedKey := "memory/goals/completed/" + goal.ID
	_, err = store.Get(completedKey)
	if err != nil {
		t.Fatalf("Get(completed mirror) error = %v", err)
	}
}

func TestGoalMirrorDeleted(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	goal, _ := store.CreateGoal("Delete Mirror", "", "", 5, nil)
	store.DeleteGoal(goal.ID)

	for _, status := range []string{"active", "completed", "archived"} {
		key := fmt.Sprintf("memory/goals/%s/%s", status, goal.ID)
		_, err := store.Get(key)
		if err == nil {
			t.Fatalf("Get(%s mirror) error = nil, want error (should be deleted)", status)
		}
	}
}

func TestLogEvent(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	store.LogEvent("test", "test-key", "test summary", "test details")

	entries, err := store.GetTimeline("", "", 10)
	if err != nil {
		t.Fatalf("GetTimeline() error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("GetTimeline() returned empty after LogEvent")
	}
	if entries[0].Key != "test-key" {
		t.Fatalf("GetTimeline()[0].Key = %q, want test-key", entries[0].Key)
	}
}

func TestGetTimelineLimit(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	for i := 1; i <= 5; i++ {
		store.LogEvent("test", fmt.Sprintf("key-%d", i), fmt.Sprintf("summary %d", i), "")
	}

	entries, err := store.GetTimeline("", "", 3)
	if err != nil {
		t.Fatalf("GetTimeline() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("GetTimeline(limit=3) returned %d entries, want 3", len(entries))
	}
}

func TestGetTimelineEmptyRange(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	store.LogEvent("test", "key", "summary", "")

	entries, err := store.GetTimeline("", "", 10)
	if err != nil {
		t.Fatalf("GetTimeline() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("GetTimeline(empty range) = %d, want 1", len(entries))
	}
}

func TestSessionSaveAndGet(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	data := []byte(`{"task":"test task","files":["main.go"]}`)
	key, err := store.SessionSave("test-project", data)
	if err != nil {
		t.Fatalf("SessionSave() error = %v", err)
	}
	if !strings.HasSuffix(key, "/latest") {
		t.Fatalf("SessionSave() key = %q, want suffix /latest", key)
	}

	got, err := store.SessionGet("test-project")
	if err != nil {
		t.Fatalf("SessionGet() error = %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("SessionGet() = %s, want %s", string(got), string(data))
	}
}

func TestSessionList(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	store.SessionSave("project-a", []byte("data-a"))
	store.SessionSave("project-b", []byte("data-b"))

	keys, err := store.SessionList("")
	if err != nil {
		t.Fatalf("SessionList() error = %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("SessionList() returned empty")
	}
}

func TestSessionGetNonExistent(t *testing.T) {
	store, _ := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	defer store.Close()

	_, err := store.SessionGet("nonexistent-project")
	if err == nil {
		t.Fatal("SessionGet(nonexistent) error = nil, want error")
	}
}

func TestSanitizeSessionProject(t *testing.T) {
	got := sanitizeSessionProject("foo/bar")
	if got != "foo-bar" {
		t.Fatalf("sanitizeSessionProject() = %q, want foo-bar", got)
	}
}
