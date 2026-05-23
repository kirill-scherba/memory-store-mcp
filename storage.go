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
	"strings"
	"time"

	"crypto/md5"
	"encoding/hex"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/kirill-scherba/sqlh"
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

// goalRow is the internal database representation for sqlh.
type goalRow struct {
	ID          string    `db:"id" db_key:"primary key"`
	Title       string    `db:"title" db_key:"not null"`
	Description string    `db:"description"`
	Status      string    `db:"status"`
	Labels      string    `db:"labels"` // JSON array string
	Priority    int       `db:"priority"`
	Progress    int       `db:"progress"`
	Deadline    string    `db:"deadline"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

func (goalRow) TableName() string { return "goals" }

// toGoal converts a goalRow to the public Goal type.
func (r *goalRow) toGoal() *Goal {
	return &Goal{
		ID:          r.ID,
		Title:       r.Title,
		Description: r.Description,
		Labels:      parseStoredLabels(r.Labels),
		Status:      r.Status,
		Priority:    r.Priority,
		Progress:    r.Progress,
		Deadline:    r.Deadline,
		CreatedAt:   r.CreatedAt.Unix(),
		UpdatedAt:   r.UpdatedAt.Unix(),
	}
}

// fromGoal converts a public Goal to goalRow.
func goalFrom(g *Goal) *goalRow {
	labels, _ := json.Marshal(normalizeLabels(g.Labels))
	return &goalRow{
		ID:          g.ID,
		Title:       g.Title,
		Description: g.Description,
		Status:      g.Status,
		Labels:      string(labels),
		Priority:    g.Priority,
		Progress:    g.Progress,
		Deadline:    g.Deadline,
		CreatedAt:   time.Unix(g.CreatedAt, 0),
		UpdatedAt:   time.Unix(g.UpdatedAt, 0),
	}
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

// TimelineEvent represents a single usage event for the timeline_events table.
type TimelineEvent struct {
	ID        int64     `db:"id" db_key:"primary key autoincrement"`
	EventType string    `db:"event_type" db_key:"not null"`
	Key       string    `db:"key"`
	Summary   string    `db:"summary"`
	Details   string    `db:"details"`
	CreatedAt time.Time `db:"created_at"`
}

func (TimelineEvent) TableName() string {
	return "timeline_events"
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

	// Create goals table via sqlh
	if err := sqlh.Create[goalRow](goalsDB); err != nil {
		kv.Close()
		goalsDB.Close()
		return nil, fmt.Errorf("create goals table: %w", err)
	}

	// Create timeline_events table for usage tracking
	if err := sqlh.Create[TimelineEvent](goalsDB); err != nil {
		kv.Close()
		goalsDB.Close()
		return nil, fmt.Errorf("create timeline_events table: %w", err)
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
	now := time.Now().UTC()
	labels = normalizeLabels(labels)
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, fmt.Errorf("marshal labels: %w", err)
	}
	progress := 0
	if done, total := countSubtasksFromDescription(description); total > 0 {
		progress = done * 100 / total
	}

	row := goalRow{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      "active",
		Labels:      string(labelsJSON),
		Priority:    priority,
		Progress:    progress,
		Deadline:    deadline,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := sqlh.Insert(s.goals, row); err != nil {
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
	row, err := sqlh.Get[goalRow](s.goals, sqlh.Where{Field: "id=", Value: id})
	if err != nil {
		return nil, fmt.Errorf("get goal %s: %w", id, err)
	}
	return row.toGoal(), nil
}

// ListGoals returns goals filtered by status and labels. If status is empty,
// returns all statuses. Labels are matched as JSON strings in the labels column.
func (s *Storage) ListGoals(status string, labelsFilter []string) ([]Goal, error) {
	var wheres []interface{}

	if status != "" {
		wheres = append(wheres, sqlh.Where{Field: "status=", Value: status})
	}
	for _, label := range normalizeLabels(labelsFilter) {
		wheres = append(wheres, sqlh.Where{Field: "labels LIKE", Value: `%"` + label + `%"`})
	}

	var listErr error
	var goals []Goal
	for _, row := range sqlh.ListRange[goalRow](
		s.goals, 0, "", "priority DESC, created_at DESC", 1000,
		append([]any{func(err error) { listErr = err }}, wheres...)...,
	) {
		goals = append(goals, *row.toGoal())
	}
	if listErr != nil {
		return nil, fmt.Errorf("list goals: %w", listErr)
	}

	return goals, nil
}

// UpdateGoal updates an existing goal's fields.
func (s *Storage) UpdateGoal(id, title, description, status, deadline string, priority, progress int, labels []string) (*Goal, error) {
	existing, err := s.GetGoal(id)
	if err != nil {
		return nil, err
	}

	row := goalFrom(existing)
	now := time.Now().UTC()

	if title != "" {
		row.Title = title
	}
	if description != "" {
		row.Description = description
	}
	if status != "" {
		row.Status = status
	}
	if deadline != "" {
		row.Deadline = deadline
	}
	if labels != nil {
		labelsB, _ := json.Marshal(normalizeLabels(labels))
		row.Labels = string(labelsB)
	}
	if priority >= 0 {
		row.Priority = priority
	}
	if progress >= 0 {
		row.Progress = progress
	} else if description != "" {
		done, total := countSubtasksFromDescription(description)
		if total > 0 {
			row.Progress = done * 100 / total
		}
	}
	row.UpdatedAt = now

	if err := sqlh.Update(s.goals, sqlh.UpdateAttr[goalRow]{
		Row:    *row,
		Wheres: []sqlh.Where{{Field: "id=", Value: id}},
	}); err != nil {
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
	if err := sqlh.Delete[goalRow](s.goals, sqlh.Where{Field: "id=", Value: id}); err != nil {
		return fmt.Errorf("delete goal %s: %w", id, err)
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

// LogEvent records a usage event in the timeline_events table.
func (s *Storage) LogEvent(eventType string, key, summary, details string) {
	event := TimelineEvent{
		EventType: eventType,
		Key:       key,
		Summary:   summary,
		Details:   details,
		CreatedAt: time.Now().UTC(),
	}
	if err := sqlh.Insert(s.goals, event); err != nil {
		log.Printf("⚠ LogEvent failed: %v", err)
	}
}

// GetTimeline returns events from timeline_events within the given date range.
// If from or to is empty, no bound is applied.
func (s *Storage) GetTimeline(from, to string, limit int) ([]TimelineEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	var wheres []sqlh.Where
	if from != "" {
		wheres = append(wheres, sqlh.Where{Field: "created_at>=", Value: from})
	}
	if to != "" {
		wheres = append(wheres, sqlh.Where{Field: "created_at<=", Value: to})
	}

	var listErr error
	events := make([]TimelineEntry, 0, limit)
	for _, ev := range sqlh.ListRange[TimelineEvent](
		s.goals, 0, "", "created_at DESC", limit,
		func(err error) { listErr = err },
	) {
		events = append(events, TimelineEntry{
			Key:       ev.Key,
			Value:     MemoryValue{Content: ev.Summary, Summary: ev.EventType},
			CreatedAt: ev.CreatedAt.Format(time.RFC3339),
		})
		if len(events) >= limit {
			break
		}
	}
	if listErr != nil {
		return nil, fmt.Errorf("timeline query: %w", listErr)
	}

	return events, nil
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

// ---------------------------------------------------------------------------
// Session state
// ---------------------------------------------------------------------------

// SessionSave saves session state for a project. Stores two entries:
//   session/project/<project>/latest         — always overwritten (restore)
//   session/project/<project>/<timestamp>    — timestamp snapshot (history)
func (s *Storage) SessionSave(project string, data []byte) (string, error) {
	project = sanitizeSessionProject(project)
	latestKey := fmt.Sprintf("session/project/%s/latest", project)

	if _, err := s.kv.Set(latestKey, data); err != nil {
		return "", fmt.Errorf("session save latest: %w", err)
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	tsKey := fmt.Sprintf("session/project/%s/%s", project, ts)
	if _, err := s.kv.Set(tsKey, data); err != nil {
		return "", fmt.Errorf("session save timestamp: %w", err)
	}

	return latestKey, nil
}

// SessionGet retrieves the latest session state for a project.
func (s *Storage) SessionGet(project string) ([]byte, error) {
	project = sanitizeSessionProject(project)
	key := fmt.Sprintf("session/project/%s/latest", project)
	data, err := s.kv.Get(key)
	if err != nil {
		return nil, fmt.Errorf("session get %s: %w", project, err)
	}
	return data, nil
}

// SessionList returns session keys with the given prefix.
func (s *Storage) SessionList(prefix string) ([]string, error) {
	if prefix == "" {
		prefix = "session/"
	}
	var keys []string
	for key := range s.kv.List(prefix) {
		keys = append(keys, key)
	}
	return keys, nil
}

// SessionCompact deletes timestamped session entries older than maxAge.
// Never deletes */latest keys.
func (s *Storage) SessionCompact(maxAge time.Duration) (int, error) {
	if maxAge <= 0 {
		maxAge = 7 * 24 * time.Hour
	}

	cutoff := time.Now().UTC().Add(-maxAge)
	var deleted int

	for key := range s.kv.List("session/") {
		if strings.HasSuffix(key, "/latest") {
			continue
		}
		info, err := s.kv.GetInfo(key)
		if err != nil {
			continue
		}
		if info.CreatedAt.Before(cutoff) {
			if err := s.kv.Del(key); err == nil {
				deleted++
			}
		}
	}

	return deleted, nil
}

// sanitizeSessionProject replaces path separators to keep session keys flat.
func sanitizeSessionProject(project string) string {
	return strings.ReplaceAll(project, "/", "-")
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