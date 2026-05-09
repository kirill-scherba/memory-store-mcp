// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

// SearchResult represents a single search hit.
type SearchResult struct {
	Key       string  `json:"key"`
	Value     string  `json:"value"`
	Score     float64 `json:"score"`
	CreatedAt string  `json:"created_at"`
}

// MemoryValue is the parsed JSON content of a stored memory.
type MemoryValue struct {
	Content   string   `json:"content,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Source    string   `json:"source,omitempty"`
	Timestamp string   `json:"timestamp,omitempty"`
}

// Goal represents a tracked goal.
type Goal struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Progress    int      `json:"progress"`
	Priority    int      `json:"priority"`
	Deadline    string   `json:"deadline,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

// TimelineEntry represents one event on the timeline.
type TimelineEntry struct {
	Key       string      `json:"key"`
	Value     MemoryValue `json:"value"`
	CreatedAt string      `json:"created_at"`
}

// ContextMemoryItem is a single memory item in the context result, with score.
type ContextMemoryItem struct {
	Key       string      `json:"key"`
	Value     MemoryValue `json:"value"`
	Score     float64     `json:"score,omitempty"`
	CreatedAt string      `json:"created_at"`
}

// Suggestion represents a proactive suggestion.
type Suggestion struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ContextResult is the aggregated context response.
type ContextResult struct {
	Goals    []Goal              `json:"goals,omitempty"`
	Memories []ContextMemoryItem `json:"memories,omitempty"`
}

