// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"crypto/md5"
	"encoding/hex"

	_ "github.com/tursodatabase/go-libsql"
	"github.com/kirill-scherba/keyvalembd"
)

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// MemoryValue is the JSON value stored in memory entries.
type MemoryValue struct {
	Content   string            `json:"content"`
	Summary   string            `json:"summary,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Source    string            `json:"source,omitempty"`
	Timestamp string            `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Status    string            `json:"status,omitempty"`   // active, completed, archived
	Priority  int               `json:"priority,omitempty"` // 0-10
	GoalID    string            `json:"goal_id,omitempty"`
}

// Goal represents a tracked goal.
type Goal struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"` // active, completed, archived
	Priority    int       `json:"priority"`
	Progress    int       `json:"progress"` // 0-100
	Deadline    string    `json:"deadline,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ContextResult is the aggregated context returned by GetContext.
type ContextResult struct {
	Query      string               `json:"query"`
	Memories   []ContextMemoryItem  `json:"memories"`
	Goals      []Goal               `json:"goals,omitempty"`
	TotalCount int                  `json:"total_count"`
}

// ContextMemoryItem is a single memory item in the context result.
type ContextMemoryItem struct {
	Key       string       `json:"key"`
	Value     MemoryValue  `json:"value"`
	Score     float64      `json:"score,omitempty"`
	CreatedAt string       `json:"created_at"`
}

// TimelineEntry represents a single entry in the timeline.
type TimelineEntry struct {
	Key       string      `json:"key"`
	Value     MemoryValue `json:"value"`
	CreatedAt string      `json:"created_at"`
}

// Suggestion is a proactive suggestion returned by Suggest.
type Suggestion struct {
	Type        string `json:"type"`        // reminder, followup, goal_next_step, insight
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

// ---------------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------------

// Storage wraps KeyValueEmbd and adds goal tracking, timeline, and proactive
// features.
type Storage struct {
	kv     *keyvalembd.KeyValueEmbd
	goals  *sql.DB
	dbPath string
}

// NewStorage creates a new Storage, initialising both the KV store and the
// goals table in the same SQLite database.
func NewStorage(dbPath string) (*Storage, error) {
	kv, err := keyvalembd.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyvalembd: %w", err)
	}

	// Open a second connection for the goals table (same DB file)
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		dbPath,
	)
	goalsDB, err := sql.Open("libsql", dsn)
	if err != nil {
		kv.Close()
		return nil, fmt.Errorf("failed to open goals db: %w", err)
	}

	// Create goals table
	if _, err := goalsDB.Exec(`
		CREATE TABLE IF NOT EXISTS goals (
			id          TEXT PRIMARY KEY NOT NULL,
			title       TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'active',
			priority    INTEGER NOT NULL DEFAULT 5,
			progress    INTEGER NOT NULL DEFAULT 0,
			deadline    TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		kv.Close()
		goalsDB.Close()
		return nil, fmt.Errorf("create goals table: %w", err)
	}

	log.Printf("✅ storage ready at: %s", dbPath)
	return &Storage{kv: kv, goals: goalsDB, dbPath: dbPath}, nil
}

// Close releases all resources.
func (s *Storage) Close() {
	if s.goals != nil {
		s.goals.Close()
	}
	if s.kv != nil {
		s.kv.Close()
	}
}

// ---------------------------------------------------------------------------
// Auto-key generation
// ---------------------------------------------------------------------------

// autoKey generates a deterministic key from text content.
// Format: memory/auto/YYYY-MM-DD/<hash-prefix>
func autoKey(content string) string {
	date := time.Now().UTC().Format("2006-01-02")
	hash := md5.Sum([]byte(content))
	prefix := hex.EncodeToString(hash[:])[:8]
	return fmt.Sprintf("memory/auto/%s/%s", date, prefix)
}

// ---------------------------------------------------------------------------
// Memory CRUD
// ---------------------------------------------------------------------------

// Save stores a memory entry with optional auto-key generation.
// If autoKey is true, the key is generated from the text; otherwise key is used.
// The text is used for embedding generation.
func (s *Storage) Save(key string, value *MemoryValue, text string, autoGenKey bool) (string, error) {
	if value.Timestamp == "" {
		value.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	jsonValue, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal value: %w", err)
	}

	finalKey := key
	if autoGenKey || key == "" {
		finalKey = autoKey(text)
	}

	_, err = s.kv.SetWithEmbedding(finalKey, jsonValue, text)
	if err != nil {
		return "", fmt.Errorf("save to keyvalembd: %w", err)
	}

	return finalKey, nil
}

// Get retrieves a memory entry by key.
func (s *Storage) Get(key string) (*MemoryValue, error) {
	data, err := s.kv.Get(key)
	if err != nil {
		return nil, err
	}

	var val MemoryValue
	if err := json.Unmarshal(data, &val); err != nil {
		return nil, fmt.Errorf("unmarshal value for %s: %w", key, err)
	}

	return &val, nil
}

// Delete removes a memory entry by key.
func (s *Storage) Delete(key string) error {
	return s.kv.Del(key)
}

// List returns all keys with the given prefix.
func (s *Storage) List(prefix string) ([]string, error) {
	var keys []string
	for key := range s.kv.List(prefix) {
		keys = append(keys, key)
	}
	return keys, nil
}

// SearchResult is a search result entry.
type SearchResult = keyvalembd.SearchResult

// Search performs semantic search across all memories.
func (s *Storage) Search(query string, limit int) ([]SearchResult, error) {
	return s.kv.SearchSemantic(query, limit)
}

// ---------------------------------------------------------------------------
// Context injection
// ---------------------------------------------------------------------------

// GetContext retrieves aggregated relevant context for the current query.
// It performs semantic search, fetches top-N results with metadata,
// and includes active goals.
func (s *Storage) GetContext(query string, limit int) (*ContextResult, error) {
	if limit <= 0 {
		limit = 5
	}

	result := &ContextResult{
		Query: query,
	}

	// Semantic search
	searchResults, err := s.kv.SearchSemantic(query, limit)
	if err != nil {
		return result, fmt.Errorf("semantic search: %w", err)
	}

	result.TotalCount = len(searchResults)

	for _, sr := range searchResults {
		var val MemoryValue
		if err := json.Unmarshal([]byte(sr.Key), &val); err == nil {
			// If the key itself is valid JSON, we need to fetch the value
		}
		// Fetch actual value by key
		value, err := s.Get(sr.Key)
		if err != nil {
			continue
		}

		// Get info to fetch created_at
		info, err := s.kv.GetInfo(sr.Key)
		if err != nil {
			continue
		}

		result.Memories = append(result.Memories, ContextMemoryItem{
			Key:       sr.Key,
			Value:     *value,
			Score:     sr.Score,
			CreatedAt: info.CreatedAt.Format(time.RFC3339),
		})
	}

	// Fetch active goals
	goals, err := s.ListGoals("active")
	if err == nil && len(goals) > 0 {
		result.Goals = goals
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Goal tracker
// ---------------------------------------------------------------------------

// CreateGoal creates a new goal and returns its ID.
func (s *Storage) CreateGoal(title, description, deadline string, priority int) (*Goal, error) {
	id := fmt.Sprintf("goal/%s/%s",
		time.Now().UTC().Format("2006-01-02"),
		fmt.Sprintf("%x", md5.Sum([]byte(title)))[:8],
	)
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	_, err := s.goals.Exec(`
		INSERT INTO goals (id, title, description, status, priority, progress, deadline, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, 0, ?, ?, ?)
	`, id, title, description, priority, deadline, now, now)
	if err != nil {
		return nil, fmt.Errorf("create goal: %w", err)
	}

	return s.GetGoal(id)
}

// GetGoal retrieves a single goal by ID.
func (s *Storage) GetGoal(id string) (*Goal, error) {
	var g Goal
	var createdAt, updatedAt string
	var deadline string

	err := s.goals.QueryRow(`
		SELECT id, title, description, status, priority, progress, deadline, created_at, updated_at
		FROM goals WHERE id = ?
	`, id).Scan(&g.ID, &g.Title, &g.Description, &g.Status, &g.Priority, &g.Progress, &deadline, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("goal %s not found", id)
		}
		return nil, fmt.Errorf("get goal %s: %w", id, err)
	}

	g.Deadline = deadline
	g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	g.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	return &g, nil
}

// ListGoals returns goals filtered by status. If status is empty, returns all.
func (s *Storage) ListGoals(status string) ([]Goal, error) {
	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = s.goals.Query(`
			SELECT id, title, description, status, priority, progress, deadline, created_at, updated_at
			FROM goals WHERE status = ? ORDER BY priority DESC, created_at DESC
		`, status)
	} else {
		rows, err = s.goals.Query(`
			SELECT id, title, description, status, priority, progress, deadline, created_at, updated_at
			FROM goals ORDER BY priority DESC, created_at DESC
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	defer rows.Close()

	var goals []Goal
	for rows.Next() {
		var g Goal
		var createdAt, updatedAt, deadline string
		if err := rows.Scan(&g.ID, &g.Title, &g.Description, &g.Status, &g.Priority, &g.Progress, &deadline, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan goal: %w", err)
		}
		g.Deadline = deadline
		g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		g.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		goals = append(goals, g)
	}

	return goals, nil
}

// UpdateGoal updates an existing goal's fields.
func (s *Storage) UpdateGoal(id, title, description, status, deadline string, priority, progress int) (*Goal, error) {
	// Build dynamic UPDATE
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	sets := []string{"updated_at = ?"}
	args := []interface{}{now}

	if title != "" {
		sets = append(sets, "title = ?")
		args = append(args, title)
	}
	if description != "" {
		sets = append(sets, "description = ?")
		args = append(args, description)
	}
	if status != "" {
		sets = append(sets, "status = ?")
		args = append(args, status)
	}
	if deadline != "" {
		sets = append(sets, "deadline = ?")
		args = append(args, deadline)
	}
	if priority >= 0 {
		sets = append(sets, "priority = ?")
		args = append(args, priority)
	}
	if progress >= 0 {
		sets = append(sets, "progress = ?")
		args = append(args, progress)
	}

	args = append(args, id)

	query := fmt.Sprintf("UPDATE goals SET %s WHERE id = ?", strings.Join(sets, ", "))
	_, err := s.goals.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("update goal %s: %w", id, err)
	}

	return s.GetGoal(id)
}

// ---------------------------------------------------------------------------
// Timeline
// ---------------------------------------------------------------------------

// GetTimeline returns memory entries created within the given date range.
// If from or to is empty, no bound is applied.
func (s *Storage) GetTimeline(from, to string, limit int) ([]TimelineEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	// For timeline we use the List + Get approach with kv_data, but we need
	// access to created_at. Since keyvalembd doesn't expose raw queries,
	// we list all entries with prefix and filter.
	// A more efficient approach would use DB access, but for now we work
	// with the KV API.
	query := "SELECT key FROM kv_data"
	var args []interface{}
	var conditions []string

	if from != "" {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, from)
	}
	if to != "" {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, to)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	// We need DB access — use the goals connection (same DB)
	rows, err := s.goals.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("timeline query: %w", err)
	}
	defer rows.Close()

	var entries []TimelineEntry
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			continue
		}
		val, err := s.Get(key)
		if err != nil {
			continue
		}

		// Get created_at from keyvalembd info
		info, err := s.kv.GetInfo(key)
		if err != nil {
			continue
		}

		entries = append(entries, TimelineEntry{
			Key:       key,
			Value:     *val,
			CreatedAt: info.CreatedAt.Format(time.RFC3339),
		})
	}

	return entries, nil
}

// ---------------------------------------------------------------------------
// Extract & save (auto-save)
// ---------------------------------------------------------------------------

// ExtractAndSave analyses the given text using the LLM and saves extracted
// facts automatically. Returns the list of saved memory keys.
func (s *Storage) ExtractAndSave(text string) ([]string, error) {
	facts, err := ExtractFacts(text)
	if err != nil {
		return nil, fmt.Errorf("extract facts: %w", err)
	}

	var savedKeys []string
	for _, fact := range facts {
		val := &MemoryValue{
			Content:   fact.Content,
			Summary:   fact.Summary,
			Tags:      fact.Tags,
			Source:    "auto-extract",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}

		key, err := s.Save("", val, fact.Content, true)
		if err != nil {
			log.Printf("⚠ auto-save failed for fact '%s': %v", fact.Summary, err)
			continue
		}
		_ = key
		savedKeys = append(savedKeys, key)
	}

	return savedKeys, nil
}

// ---------------------------------------------------------------------------
// Proactive suggest
// ---------------------------------------------------------------------------

// Suggest analyses current context, active goals, and recent history to return
// proactive suggestions.
func (s *Storage) Suggest(currentContext string, limit int) ([]Suggestion, error) {
	if limit <= 0 {
		limit = 5
	}

	// Get active goals
	goals, err := s.ListGoals("active")
	if err != nil {
		return nil, fmt.Errorf("list active goals: %w", err)
	}

	// Get recent memories
	recentTimeline, err := s.GetTimeline("", "", 5)
	if err != nil {
		return nil, fmt.Errorf("get recent timeline: %w", err)
	}

	// Build prompt for the LLM
	var goalLines []string
	for _, g := range goals {
		status := "⏳"
		if g.Progress >= 100 {
			status = "✓"
		}
		goalLines = append(goalLines, fmt.Sprintf("%s %s: %s (progress: %d%%, priority: %d)",
			status, g.Title, g.Description, g.Progress, g.Priority))
	}

	var recentLines []string
	for _, e := range recentTimeline {
		recentLines = append(recentLines, fmt.Sprintf("[%s] %s: %s",
			e.CreatedAt[:10], e.Key, truncate(e.Value.Content, 80)))
	}

	prompt := fmt.Sprintf(`Analyse the following context and active goals, and suggest up to %d proactive suggestions. Current context: %s

Active goals:
%s

Recent activity:
%s

Return a JSON array of suggestions. Each suggestion has: type (reminder/followup/goal_next_step/insight), title, description, priority (0-10).`, limit, currentContext, strings.Join(goalLines, "\n"), strings.Join(recentLines, "\n"))

	suggestPrompt := SuggestPrompt(prompt)

	msg := []OllamaChatMessage{
		{Role: "system", Content: "You are a proactive assistant. Analyse context and goals to suggest next steps. Return ONLY a JSON array, nothing else."},
		{Role: "user", Content: suggestPrompt},
	}

	answer, err := generateAnswer(msg)
	if err != nil {
		return nil, fmt.Errorf("LLM suggest failed: %w", err)
	}

	// Parse JSON response
	var suggestions []Suggestion
	if err := json.Unmarshal([]byte(answer), &suggestions); err != nil {
		// Try to extract JSON array from the response if it contains markdown
		cleaned := answer
		if idx := strings.Index(answer, "["); idx >= 0 {
			if end := strings.LastIndex(answer, "]"); end > idx {
				cleaned = answer[idx : end+1]
			}
		}
		if err := json.Unmarshal([]byte(cleaned), &suggestions); err != nil {
			return nil, fmt.Errorf("parse suggestions JSON: %w (response: %s)", err, answer)
		}
	}

	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	return suggestions, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// truncate shortens a string to maxLen runes, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}