// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock BotFuncs for testing
// ---------------------------------------------------------------------------

type mockStore struct {
	notes    []string
	goals    []string
	memories map[string]string
}

func newMockStore() *mockStore {
	return &mockStore{
		memories: make(map[string]string),
	}
}

func (m *mockStore) saveNote(title, description string, tags []string) (string, error) {
	key := fmt.Sprintf("memory/test/%d", len(m.notes))
	data, _ := json.Marshal(map[string]interface{}{
		"title":       title,
		"description": description,
		"tags":        tags,
		"key":         key,
	})
	m.memories[key] = string(data)
	m.notes = append(m.notes, key)
	return key, nil
}

func (m *mockStore) createGoal(title, description, deadline string, priority int, labels []string) (string, error) {
	goalID := fmt.Sprintf("goal-%d", len(m.goals))
	m.goals = append(m.goals, goalID)
	return goalID, nil
}

func (m *mockStore) search(query string, limit int) (string, error) {
	valBytes, _ := json.Marshal(MemoryValue{
		Content: "Some relevant content for testing.",
		Summary: fmt.Sprintf("Found memory related to: %s", query),
		Tags:    []string{"test", "mock"},
	})
	results := []SearchResult{
		{
			Key:       "memory/test/search-result-1",
			Value:     string(valBytes),
			Score:     0.95,
			CreatedAt: "2026-05-07T10:00:00Z",
		},
	}
	data, _ := json.Marshal(results)
	return string(data), nil
}

func (m *mockStore) listGoals(status string, labelsFilter []string) (string, error) {
	goals := []map[string]interface{}{
		{
			"id":          "goal-0",
			"title":       "Test Goal",
			"description": "This is a mock test goal for testing.",
			"status":      "active",
			"priority":    5,
			"progress":    30,
			"labels":      []string{"test"},
		},
	}
	data, _ := json.Marshal(goals)
	return string(data), nil
}

func (m *mockStore) getGoal(id string) (string, error) {
	goal := map[string]interface{}{
		"id":          id,
		"title":       "Test Goal",
		"description": "Mock goal description.",
		"status":      "active",
		"priority":    5,
		"progress":    30,
	}
	data, _ := json.Marshal(goal)
	return string(data), nil
}

func (m *mockStore) getTimeline(from, to string, limit int) (string, error) {
	events := []map[string]interface{}{
		{
			"key":       "memory/test/event-1",
			"content":   "Test timeline event",
			"createdAt": "2026-05-07T10:00:00Z",
		},
	}
	data, _ := json.Marshal(events)
	return string(data), nil
}

func (m *mockStore) suggest(currentContext string, limit int, lang string) (string, error) {
	suggestions := []Suggestion{
		{
			Type:        "goal_next_step",
			Title:       "Continue working on the test goal",
			Description: "You have an active goal in progress (30% complete). Focus on completing the next milestone.",
		},
		{
			Type:        "insight",
			Title:       "Review recent memories",
			Description: "Based on your active goals and recent memories, consider reviewing what you've learned this week.",
		},
	}
	data, _ := json.Marshal(suggestions)
	return string(data), nil
}

func (m *mockStore) getContext(query string, limit int) (string, error) {
	result := ContextResult{
		Memories: []ContextMemoryItem{
			{
				Key: "memory/test/ctx-1",
				Value: MemoryValue{
					Content: "Relevant context for testing.",
					Summary: "Test context summary",
					Tags:    []string{"test"},
				},
				Score:     0.92,
				CreatedAt: "2026-05-07T10:00:00Z",
			},
		},
		Goals: []Goal{
			{
				ID:          "goal-0",
				Title:       "Test Goal",
				Description: "Active test goal.",
				Progress:    30,
			},
		},
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

func (m *mockStore) updateGoal(id, title, description, status, deadline string, priority, progress int, labels []string) (string, error) {
	goal := map[string]interface{}{
		"id":          id,
		"title":       title,
		"description": description,
		"status":      status,
		"priority":    priority,
		"progress":    progress,
	}
	data, _ := json.Marshal(goal)
	return string(data), nil
}

func (m *mockStore) deleteMemory(key string) error {
	delete(m.memories, key)
	return nil
}

func (m *mockStore) getMemory(key string) (string, error) {
	if v, ok := m.memories[key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("memory not found: %s", key)
}

// Mock LLM requester that simulates the agent responses.
// Returns a properly formatted JSON agent command in the new format:
// {"call":"...", "title":"...", "content":"...", ...}
//
// IMPORTANT: case order matters — more specific patterns must come before
// broader ones to avoid false matches. For example, "покажи цели" must be
// before a case that matches just "цель".
func mockLLMRequest(systemPrompt string, messages []ChatMessage) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages")
	}
	userMsg := messages[len(messages)-1].Content

	// Strip appended context for matching purposes — the original user message
	// is what determines intent. Context ("Current context:\n...") is appended
	// by handleTextWithAgent and should not influence classification.
	if idx := strings.Index(userMsg, "\n\nCurrent context:"); idx >= 0 {
		userMsg = userMsg[:idx]
	}
	if idx := strings.Index(userMsg, "\n\nCurrent state:"); idx >= 0 {
		userMsg = userMsg[:idx]
	}
	userMsg = strings.TrimSpace(userMsg)

	// More specific patterns FIRST, broader ones later.
	switch {
	// 1. Save note — only exact save/remember keywords
	case strings.Contains(userMsg, "запомни") || strings.Contains(userMsg, "remember") ||
		strings.Contains(userMsg, "сохрани") || strings.Contains(userMsg, "save"):
		return `{"call":"save_note","title":"Test Note","content":"Test note content","labels":["test"]}`, nil

	// 2. List goals — must come BEFORE create_goal because "покажи цели" contains "цели"~"цель"
	case strings.Contains(userMsg, "покажи цели") || strings.Contains(userMsg, "show goals") ||
		strings.Contains(userMsg, "список целей") || strings.Contains(userMsg, "list goals"):
		return `{"call":"list_goals","status":"active"}`, nil

	// 3. Delete memory — specific patterns
	case strings.Contains(userMsg, "удали") || strings.Contains(userMsg, "delete") ||
		strings.Contains(userMsg, "remove"):
		return `{"call":"delete_memory","key":"memory/test/target"}`, nil

	// 4. Get memory — specific "show me", "get", "достань" patterns
	case strings.Contains(userMsg, "достань") || strings.Contains(userMsg, "get memory") ||
		strings.Contains(userMsg, "покажи заметку") || strings.Contains(userMsg, "show note"):
		return `{"call":"get_memory","key":"memory/test/target"}`, nil

	// 5. Update goal — update/progress keywords
	case strings.Contains(userMsg, "обнови") || strings.Contains(userMsg, "update") ||
		strings.Contains(userMsg, "прогресс") || strings.Contains(userMsg, "progress"):
		return `{"call":"update_goal","goal_id":"goal-0","title":"Updated Goal","content":"Updated description","progress":50}`, nil

	// 6. Create goal — only match "создай" (most specific), NOT broad "цель" or "goal"
	case strings.Contains(userMsg, "создай") || strings.Contains(userMsg, "create"):
		return `{"call":"create_goal","title":"Test Goal","content":"Test goal description","priority":5,"labels":["test"]}`, nil

	// 7. Search
	case strings.Contains(userMsg, "найди") || strings.Contains(userMsg, "search") ||
		strings.Contains(userMsg, "поиск"):
		return `{"call":"search","query":"test search","limit":5}`, nil

	// 8. Suggest
	case strings.Contains(userMsg, "подскажи") || strings.Contains(userMsg, "suggest") ||
		strings.Contains(userMsg, "чем заняться") || strings.Contains(userMsg, "what") && strings.Contains(userMsg, "do"):
		return `{"call":"suggest","query":"","limit":5}`, nil

	// 9. Timeline
	case strings.Contains(userMsg, "таймлайн") || strings.Contains(userMsg, "timeline") ||
		strings.Contains(userMsg, "история") || strings.Contains(userMsg, "history"):
		return `{"call":"get_timeline","from":"","to":"","limit":5}`, nil

	// 10. Greeting
	case strings.Contains(userMsg, "привет") || strings.Contains(userMsg, "hello") ||
		strings.Contains(userMsg, "как дела") || strings.Contains(userMsg, "how are you"):
		return `{"call":"","answer":"Привет! Я твой AI ассистент. Чем могу помочь? 🎯"}`, nil

	// 11. Context
	case strings.Contains(userMsg, "context") || strings.Contains(userMsg, "контекст"):
		return `{"call":"get_context","query":"current context","limit":5}`, nil

	// 12. Broader fallbacks — only after all specific patterns checked
	case strings.Contains(userMsg, "цель") || strings.Contains(userMsg, "goal"):
		// "цель" or "goal" left over (not "создай", not "обнови", not list/show) — probably "покажи цель" or just "goals"
		return `{"call":"list_goals","status":"active"}`, nil

	case strings.Contains(userMsg, "покажи") || strings.Contains(userMsg, "show") || strings.Contains(userMsg, "get"):
		// Generic "show" that wasn't caught above
		return `{"call":"get_memory","key":"memory/test/target"}`, nil

	default:
		return `{"call":"","answer":"Извините, я не совсем понял ваш запрос. Попробуйте:\n- «запомни ...» — сохранить заметку\n- «создай цель ...» — создать цель\n- «что у меня по целям?» — список целей\n- «найди ...» — поиск в памяти\n- «подскажи» — рекомендации"}`, nil
	}
}

// Mock LLM question processor
func mockLLMProcess(question string, context string, lang string) (string, error) {
	return fmt.Sprintf("📝 <b>Ответ на вопрос:</b> %s\n\n<blockquote>На основе найденной информации в вашей памяти.</blockquote>", question), nil
}

// createTestBot creates a Bot with mock functions for testing.
func createTestBot(t *testing.T) *Bot {
	t.Helper()

	mock := newMockStore()

	funcs := BotFuncs{
		SaveNote:     mock.saveNote,
		CreateGoal:   mock.createGoal,
		Search:       mock.search,
		ListGoals:    mock.listGoals,
		GetGoal:      mock.getGoal,
		GetTimeline:  mock.getTimeline,
		Suggest:      mock.suggest,
		GetContext:   mock.getContext,
		UpdateGoal:   mock.updateGoal,
		DeleteMemory: mock.deleteMemory,
		GetMemory:    mock.getMemory,
		LLMProcess:   mockLLMProcess,
		LLMRequest:   mockLLMRequest,
	}

	bot := &Bot{
		funcs:    funcs,
		userLang: make(map[int64]string),
	}

	return bot
}

// ---------------------------------------------------------------------------
// Test: Save Note
// ---------------------------------------------------------------------------

func TestAgentSaveNote(t *testing.T) {
	bot := createTestBot(t)

	tests := []struct {
		name     string
		text     string
		lang     string
		contains []string
	}{
		{
			name:     "RU: запомни",
			text:     "запомни что я люблю программировать на Go",
			lang:     "ru",
			contains: []string{"запомнил", "ключ"},
		},
		{
			name:     "EN: remember",
			text:     "remember that I like programming in Go",
			lang:     "en",
			contains: []string{"saved", "key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.contains {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Create Goal
// ---------------------------------------------------------------------------

func TestAgentCreateGoal(t *testing.T) {
	bot := createTestBot(t)

	tests := []struct {
		name     string
		text     string
		lang     string
		contains []string
	}{
		{
			name:     "RU: создать цель",
			text:     "создай цель изучить микросервисы на Go",
			lang:     "ru",
			contains: []string{"цель", "создан"},
		},
		{
			name:     "EN: create goal",
			text:     "create a goal to learn microservices in Go",
			lang:     "en",
			contains: []string{"goal", "created"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.contains {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Update Goal
// ---------------------------------------------------------------------------

func TestAgentUpdateGoal(t *testing.T) {
	bot := createTestBot(t)

	tests := []struct {
		name     string
		text     string
		lang     string
		contains []string
	}{
		{
			name:     "RU: обновить прогресс",
			text:     "обнови прогресс по цели изучения Go до 50%",
			lang:     "ru",
			contains: []string{"обновлен"},
		},
		{
			name:     "EN: update goal",
			text:     "update the progress of Go learning goal to 50%",
			lang:     "en",
			contains: []string{"updated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.contains {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Search
// ---------------------------------------------------------------------------

func TestAgentSearch(t *testing.T) {
	bot := createTestBot(t)

	tests := []struct {
		name     string
		text     string
		lang     string
		contains []string
	}{
		{
			name:     "RU: поиск",
			text:     "найди информацию про Go микросервисы",
			lang:     "ru",
			contains: []string{"найдено"},
		},
		{
			name:     "EN: search",
			text:     "search for Go microservices information",
			lang:     "en",
			contains: []string{"found", "result"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.contains {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: List Goals
// ---------------------------------------------------------------------------

func TestAgentListGoals(t *testing.T) {
	bot := createTestBot(t)

	tests := []struct {
		name     string
		text     string
		lang     string
		contains []string
	}{
		{
			name:     "RU: покажи цели",
			text:     "покажи цели",
			lang:     "ru",
			contains: []string{"цел"},
		},
		{
			name:     "EN: list my goals",
			text:     "list my goals",
			lang:     "en",
			contains: []string{"goal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.contains {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Suggest
// ---------------------------------------------------------------------------

func TestAgentSuggest(t *testing.T) {
	bot := createTestBot(t)

	tests := []struct {
		name     string
		text     string
		lang     string
		contains []string
	}{
		{
			name:     "RU: подскажи",
			text:     "подскажи чем заняться",
			lang:     "ru",
			contains: []string{"предложен"},
		},
		{
			name:     "EN: what should I do",
			text:     "what should I do?",
			lang:     "en",
			contains: []string{"suggest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.contains {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: General Chat / Greeting
// ---------------------------------------------------------------------------

func TestAgentGreeting(t *testing.T) {
	bot := createTestBot(t)

	tests := []struct {
		name     string
		text     string
		lang     string
		contains []string
	}{
		{
			name:     "RU: привет",
			text:     "привет!",
			lang:     "ru",
			contains: []string{"привет", "помочь"},
		},
		{
			name:     "EN: hello",
			text:     "hello!",
			lang:     "en",
			contains: []string{"помочь"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.contains {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Get Context
// ---------------------------------------------------------------------------

func TestAgentGetContext(t *testing.T) {
	bot := createTestBot(t)

	tests := []struct {
		name     string
		text     string
		lang     string
		contains []string
	}{
		{
			name:     "EN: context",
			text:     "give me the current context",
			lang:     "en",
			contains: []string{"context"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.contains {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Unknown / Unclear Request
// ---------------------------------------------------------------------------

func TestAgentUnknown(t *testing.T) {
	bot := createTestBot(t)

	tests := []struct {
		name     string
		text     string
		lang     string
		contains []string
	}{
		{
			name:     "RU: непонятный запрос",
			text:     "абракадабра xyzzy",
			lang:     "ru",
			contains: []string{"понял"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.contains {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Formatting — verify response has expected readable format
// ---------------------------------------------------------------------------

func TestAgentFormatting(t *testing.T) {
	bot := createTestBot(t)

	// Test that formatted outputs contain expected formatting elements
	tests := []struct {
		name     string
		text     string
		lang     string
		expect   []string // expected formatting elements
	}{
		{
			name:   "RU search содержит читаемое форматирование",
			text:   "найди Go",
			lang:   "ru",
			expect: []string{"найдено"},
		},
		{
			name:   "RU привет содержит эмодзи",
			text:   "привет",
			lang:   "ru",
			expect: []string{"🎯"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.expect {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Format check - Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Error handling — verify proper error messages
// ---------------------------------------------------------------------------

func TestAgentErrors(t *testing.T) {
	// Create bot with nil functions to simulate unavailable operations
	funcs := BotFuncs{
		LLMRequest: mockLLMRequest,
	}
	bot := &Bot{
		funcs:    funcs,
		userLang: make(map[int64]string),
	}

	tests := []struct {
		name     string
		text     string
		lang     string
		contains []string
	}{
		{
			name:     "RU: save_note when unavailable",
			text:     "запомни что-то важное",
			lang:     "ru",
			contains: []string{"not available"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := bot.DebugProcess(12345, tt.text, tt.lang)
			if err != nil {
				t.Fatalf("DebugProcess returned error: %v", err)
			}
			if resp == "" {
				t.Fatal("DebugProcess returned empty response")
			}
			for _, s := range tt.contains {
				if !containsAny(resp, s) {
					t.Errorf("response does not contain %q\nResponse:\n%s", s, resp)
				}
			}
			t.Logf("Response:\n%s\n---", resp)
		})
	}
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// containsAny checks if the given string contains any of the substrings (case-insensitive).
func containsAny(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}