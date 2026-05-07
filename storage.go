// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"crypto/md5"
	"encoding/hex"

	"github.com/kirill-scherba/keyvalembd"
	_ "github.com/tursodatabase/go-libsql"
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
	ID          string   `json:"id"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	Progress    int      `json:"progress"`
	Deadline    string   `json:"deadline,omitempty"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

// ContextResult is the aggregated context returned by GetContext.
type ContextResult struct {
	Query      string              `json:"query"`
	Memories   []ContextMemoryItem `json:"memories"`
	Goals      []Goal              `json:"goals,omitempty"`
	TotalCount int                 `json:"total_count"`
}

// ContextMemoryItem is a single memory item in the context result.
type ContextMemoryItem struct {
	Key       string      `json:"key"`
	Value     MemoryValue `json:"value"`
	Score     float64     `json:"score,omitempty"`
	CreatedAt string      `json:"created_at"`
}

// TimelineEntry represents a single entry in the timeline.
type TimelineEntry struct {
	Key       string      `json:"key"`
	Value     MemoryValue `json:"value"`
	CreatedAt string      `json:"created_at"`
}

// Suggestion is a proactive suggestion returned by Suggest.
type Suggestion struct {
	Type        string `json:"type"` // reminder, followup, goal_next_step, insight
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
			labels      TEXT NOT NULL DEFAULT '[]',
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

	if err := ensureGoalSchema(goalsDB); err != nil {
		kv.Close()
		goalsDB.Close()
		return nil, err
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

// SearchResult is a search result entry enriched with value.
type SearchResult struct {
	Key       string  `json:"key"`
	Score     float64 `json:"score"`
	Value     string  `json:"value,omitempty"`
	CreatedAt string  `json:"created_at,omitempty"`
}

// Search performs semantic search across all memories.
func (s *Storage) Search(query string, limit int) ([]SearchResult, error) {
	rawResults, err := s.kv.SearchSemantic(query, limit)
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, len(rawResults))
	for i, r := range rawResults {
		results[i] = SearchResult{Key: r.Key, Score: r.Score}
	}
	return results, nil
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
	goals, err := s.ListGoals("active", nil)
	if err == nil && len(goals) > 0 {
		result.Goals = goals
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Goal tracker
// ---------------------------------------------------------------------------

// CreateGoal creates a new goal and returns its ID.
func (s *Storage) CreateGoal(title, description, deadline string, priority int, labels []string) (*Goal, error) {
	id := fmt.Sprintf("goal/%s/%d",
		time.Now().UTC().Format("2006-01-02"),
		time.Now().UnixNano(),
	)
	now := time.Now().UTC().Unix()
	labels = normalizeLabels(labels)
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, fmt.Errorf("marshal labels: %w", err)
	}
	progress := 0
	if done, total := countSubtasksFromDescription(description); total > 0 {
		progress = done * 100 / total
	}

	_, err = s.goals.Exec(`
		INSERT INTO goals (id, title, description, status, labels, priority, progress, deadline, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?, ?, ?, ?)
	`, id, title, description, string(labelsJSON), priority, progress, deadline, now, now)
	if err != nil {
		return nil, fmt.Errorf("create goal: %w", err)
	}

	goal, err := s.GetGoal(id)
	if err != nil {
		return nil, err
	}
	if err := s.syncGoalMemory(goal); err != nil {
		return nil, err
	}
	return goal, nil
}

// GetGoal retrieves a single goal by ID.
func (s *Storage) GetGoal(id string) (*Goal, error) {
	var g Goal
	var labelsJSON, deadline string

	err := s.goals.QueryRow(`
		SELECT id, title, description, status, labels, priority, progress, deadline, created_at, updated_at
		FROM goals WHERE id = ?
	`, id).Scan(&g.ID, &g.Title, &g.Description, &g.Status, &labelsJSON, &g.Priority, &g.Progress, &deadline, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("goal %s not found", id)
		}
		return nil, fmt.Errorf("get goal %s: %w", id, err)
	}

	g.Labels = parseStoredLabels(labelsJSON)
	g.Deadline = deadline

	return &g, nil
}

// ListGoals returns goals filtered by status and labels. If status is empty,
// returns all statuses. Labels are matched as JSON strings in the labels column.
func (s *Storage) ListGoals(status string, labelsFilter []string) ([]Goal, error) {
	query := `
		SELECT id, title, description, status, labels, priority, progress, deadline, created_at, updated_at
		FROM goals
	`
	var conditions []string
	var args []interface{}

	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	for _, label := range normalizeLabels(labelsFilter) {
		conditions = append(conditions, "labels LIKE ?")
		args = append(args, `%`+strconv.Quote(label)+`%`)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY priority DESC, created_at DESC"

	rows, err := s.goals.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	defer rows.Close()

	var goals []Goal
	for rows.Next() {
		var g Goal
		var createdAt, updatedAt, labelsJSON, deadline string
		if err := rows.Scan(&g.ID, &g.Title, &g.Description, &g.Status, &labelsJSON, &g.Priority, &g.Progress, &deadline, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan goal: %w", err)
		}
		g.Labels = parseStoredLabels(labelsJSON)
		g.Deadline = deadline
		// Parse as int64 if numeric, otherwise parse as time string -> unix
		if ct, err := parseIntOrTime(createdAt); err == nil {
			g.CreatedAt = ct
		}
		if ut, err := parseIntOrTime(updatedAt); err == nil {
			g.UpdatedAt = ut
		}
		goals = append(goals, g)
	}

	return goals, nil
}

// UpdateGoal updates an existing goal's fields.
func (s *Storage) UpdateGoal(id, title, description, status, deadline string, priority, progress int, labels []string) (*Goal, error) {
	if _, err := s.GetGoal(id); err != nil {
		return nil, err
	}

	// Build dynamic UPDATE
	now := time.Now().UTC().Unix()
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
	if labels != nil {
		labelsJSON, err := json.Marshal(normalizeLabels(labels))
		if err != nil {
			return nil, fmt.Errorf("marshal labels: %w", err)
		}
		sets = append(sets, "labels = ?")
		args = append(args, string(labelsJSON))
	}
	if priority >= 0 {
		sets = append(sets, "priority = ?")
		args = append(args, priority)
	}
	if progress >= 0 {
		sets = append(sets, "progress = ?")
		args = append(args, progress)
	} else if description != "" {
		done, total := countSubtasksFromDescription(description)
		if total > 0 {
			sets = append(sets, "progress = ?")
			args = append(args, done*100/total)
		}
	}

	args = append(args, id)

	query := fmt.Sprintf("UPDATE goals SET %s WHERE id = ?", strings.Join(sets, ", "))
	_, err := s.goals.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("update goal %s: %w", id, err)
	}

	goal, err := s.GetGoal(id)
	if err != nil {
		return nil, err
	}
	if err := s.syncGoalMemory(goal); err != nil {
		return nil, err
	}
	return goal, nil
}

// DeleteGoal deletes a goal and its mirrored memory entry.
func (s *Storage) DeleteGoal(id string) error {
	if id == "" {
		return fmt.Errorf("goal id is required")
	}
	result, err := s.goals.Exec(`DELETE FROM goals WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete goal %s: %w", id, err)
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return fmt.Errorf("goal %s not found", id)
	}
	return s.deleteGoalMemory(id)
}

func (s *Storage) syncGoalMemory(goal *Goal) error {
	if goal == nil {
		return nil
	}
	if err := s.deleteGoalMemory(goal.ID); err != nil {
		return err
	}

	status := goal.Status
	if status == "" {
		status = "active"
	}
	key := "memory/goals/" + status + "/" + goal.ID
	textForEmbedding := strings.TrimSpace(goal.Title + " " + goal.Description + " " + strings.Join(goal.Labels, " "))
	value := &MemoryValue{
		Content:  goal.Description,
		Summary:  goal.Title,
		Tags:     goal.Labels,
		Source:   "goal-tracker",
		Status:   status,
		Priority: goal.Priority,
		GoalID:   goal.ID,
	}
	_, err := s.Save(key, value, textForEmbedding, false)
	if err != nil {
		return fmt.Errorf("sync goal memory %s: %w", goal.ID, err)
	}
	return nil
}

func (s *Storage) deleteGoalMemory(id string) error {
	for _, status := range []string{"active", "completed", "archived"} {
		if err := s.Delete("memory/goals/" + status + "/" + id); err != nil {
			return fmt.Errorf("delete mirrored goal %s/%s: %w", status, id, err)
		}
	}
	return nil
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
func (s *Storage) Suggest(currentContext string, limit int, lang string) ([]Suggestion, error) {
	if limit <= 0 {
		limit = 5
	}

	// Get active goals
	goals, err := s.ListGoals("active", nil)
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

	sysPrompt := suggestSystemPrompt(lang)
	msg := []OllamaChatMessage{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: suggestPrompt},
	}

	answer, err := generateAnswer(msg)
	if err != nil {
		return nil, fmt.Errorf("LLM suggest failed: %w", err)
	}

	// Sanitise malformed JSON from LLM.
	// Common LLM errors:
	//   "key="value"   -> "key":"value"   (equals opens value quote, e.g. "description="text")
	//   "key=value"    -> "key":"value"   (equals with plain unquoted value, e.g. "description=text,)
	// Order matters: full pattern (with closing quote) must come BEFORE partial.
	sanitiseJSON := func(s string) string {
		// Pattern 1: "description="text with any chars"  -> "description":"text with any chars"
		// Must run BEFORE Pattern 2, otherwise Pattern 2 would eat the opening quote.
		re := regexp.MustCompile(`"(description|title|type|summary)="([^"]*)"`)
		s = re.ReplaceAllString(s, `"$1":"$2"`)
		// Pattern 2: "description=plain_value,   -> "description":"plain_value",
		// Catches: "description=text", "description=text} or "description=text]
		re = regexp.MustCompile(`"(description|title|type|summary)=([^",}\]]+)`)
		s = re.ReplaceAllString(s, `"$1":"$2"`)
		return s
	}
	answer = sanitiseJSON(answer)

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

// parseIntOrTime attempts to parse a string as int64 (Unix timestamp) first,
// then as a time string in "2006-01-02 15:04:05" format.
func parseIntOrTime(s string) (int64, error) {
	// Try as int64 (Unix timestamp stored as TEXT in SQLite)
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return val, nil
	}
	// Try as time string
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

func ensureGoalSchema(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(goals)`)
	if err != nil {
		return fmt.Errorf("read goals schema: %w", err)
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan goals schema: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate goals schema: %w", err)
	}
	if !columns["labels"] {
		if _, err := db.Exec(`ALTER TABLE goals ADD COLUMN labels TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return fmt.Errorf("add goals.labels column: %w", err)
		}
	}
	return nil
}

func normalizeLabels(labels []string) []string {
	seen := make(map[string]bool, len(labels))
	normalized := make([]string, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		normalized = append(normalized, label)
	}
	return normalized
}

func parseStoredLabels(labelsJSON string) []string {
	var labels []string
	if err := json.Unmarshal([]byte(labelsJSON), &labels); err != nil {
		return nil
	}
	return normalizeLabels(labels)
}

func countSubtasksFromDescription(description string) (done, total int) {
	re := regexp.MustCompile(`(?m)^\s*[-*+]\s+\[([ xX])\]`)
	for _, match := range re.FindAllStringSubmatch(description, -1) {
		total++
		if strings.EqualFold(match[1], "x") {
			done++
		}
	}
	return done, total
}

// truncate shortens a string to maxLen runes, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// ───────────────────────────────────────────────────────────────────────────
// Telegram bridge methods for the Telegram bot.
// ───────────────────────────────────────────────────────────────────────────

// SaveFromTelegram saves a note and returns the raw memory key.
func (s *Storage) SaveFromTelegram(title, description string, tags []string) (string, error) {
	val := &MemoryValue{
		Content:   description,
		Summary:   title,
		Tags:      tags,
		Source:    "telegram",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	text := title + " " + description
	key, err := s.Save("", val, text, true)
	if err != nil {
		return "", err
	}
	return key, nil
}

// CreateGoalFromTelegram creates a goal and returns it as JSON.
func (s *Storage) CreateGoalFromTelegram(title, description, deadline string, priority int, labels []string) (string, error) {
	goal, err := s.CreateGoal(title, description, deadline, priority, labels)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(goal)
	return string(data), nil
}

// UpdateGoalFromTelegram updates a goal and returns it as JSON.
func (s *Storage) UpdateGoalFromTelegram(id, title, description, status, deadline string, priority, progress int, labels []string) (string, error) {
	goal, err := s.UpdateGoal(id, title, description, status, deadline, priority, progress, labels)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(goal)
	return string(data), nil
}

// DeleteGoalFromTelegram deletes a goal by ID.
func (s *Storage) DeleteGoalFromTelegram(id string) error {
	return s.DeleteGoal(id)
}

// GetMemoryFromTelegram retrieves a memory by key and returns its JSON string.
func (s *Storage) GetMemoryFromTelegram(key string) (string, error) {
	val, err := s.Get(key)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(val)
	return string(data), nil
}

// DeleteMemoryFromTelegram deletes a memory by key.
func (s *Storage) DeleteMemoryFromTelegram(key string) error {
	return s.Delete(key)
}

// SearchFromTelegram searches and returns results as JSON array string.
// Enriches each result with full value from storage.
func (s *Storage) SearchFromTelegram(query string, limit int) (string, error) {
	results, err := s.Search(query, limit)
	if err != nil {
		return "", err
	}
	// Enrich each result with full value from storage
	for i, r := range results {
		val, errGet := s.Get(r.Key)
		if errGet == nil && val != nil {
			valJSON, _ := json.Marshal(val)
			results[i].Value = string(valJSON)
		}
	}
	data, _ := json.Marshal(results)
	return string(data), nil
}

// ListGoalsFromTelegram lists goals and returns them as JSON string.
func (s *Storage) ListGoalsFromTelegram(status string, labelsFilter []string) (string, error) {
	goals, err := s.ListGoals(status, labelsFilter)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(goals)
	return string(data), nil
}

// GetGoalFromTelegram gets a goal and returns it as JSON string.
func (s *Storage) GetGoalFromTelegram(id string) (string, error) {
	goal, err := s.GetGoal(id)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(goal)
	return string(data), nil
}

// GetTimelineFromTelegram returns timeline entries as JSON string.
func (s *Storage) GetTimelineFromTelegram(from, to string, limit int) (string, error) {
	entries, err := s.GetTimeline(from, to, limit)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(entries)
	return string(data), nil
}

// SuggestFromTelegram returns suggestions as JSON string.
func (s *Storage) SuggestFromTelegram(currentContext string, limit int, lang string) (string, error) {
	suggestions, err := s.Suggest(currentContext, limit, lang)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(suggestions)
	return string(data), nil
}

// GetContextFromTelegram returns context as JSON string.
func (s *Storage) GetContextFromTelegram(query string, limit int) (string, error) {
	ctx, err := s.GetContext(query, limit)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(ctx)
	return string(data), nil
}

// LLMQuestionProcess answers a user question using provided memory context + LLM.
// The context is pre-built by the caller (e.g. from GetContext) and passed here.
func (s *Storage) LLMQuestionProcess(question string, contextStr string, lang string) (string, error) {
	// 1. Build system prompt based on language
	systemPrompt := "You are a helpful AI assistant with access to the user's long-term memory."
	systemPrompt += " Answer the user's question based on the memory context provided."
	systemPrompt += " If the context doesn't contain relevant information, say so honestly."
	if lang == "ru" {
		systemPrompt = "Ты — полезный AI-ассистент с доступом к долговременной памяти пользователя."
		systemPrompt += " Ответь на вопрос пользователя на основе предоставленного контекста из памяти."
		systemPrompt += " Если в контексте нет нужной информации, честно скажи об этом."
	}

	// 4. Build user prompt with context
	userPrompt := fmt.Sprintf(`## Memory Context
%s

## User Question
%s

Please answer the question based on the memory context above.`,
		contextStr, question)

	// 5. Generate LLM answer
	messages := []OllamaChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	answer, err := generateAnswer(messages)
	if err != nil {
		return "", fmt.Errorf("LLM answer generation: %w", err)
	}

	return answer, nil
}