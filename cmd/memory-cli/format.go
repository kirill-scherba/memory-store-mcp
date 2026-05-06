// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"
)

// OutputFormat defines the output formatting mode.
type OutputFormat string

const (
	OutputJSON    OutputFormat = "json"
	OutputTable   OutputFormat = "table"
	OutputSummary OutputFormat = "summary"
)

// ParseOutputFormat parses an output format flag value.
func ParseOutputFormat(s string) OutputFormat {
	switch OutputFormat(s) {
	case OutputJSON, OutputTable, OutputSummary:
		return OutputFormat(s)
	default:
		return OutputTable
	}
}

// ---------------------------------------------------------------------------
// Goal helpers (parsed from JSON)
// ---------------------------------------------------------------------------

// goalRow is a parsed goal for table/summary rendering.
type goalRow struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	Progress    int      `json:"progress"`
	Deadline    string   `json:"deadline"`
	Labels      []string `json:"labels"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

func parseGoalsJSON(raw string) ([]goalRow, error) {
	// The server wraps goals in "Found N goals:\n<json>"
	idx := strings.Index(raw, "\n")
	if idx >= 0 {
		raw = raw[idx+1:]
	}
	var goals []goalRow
	if err := json.Unmarshal([]byte(raw), &goals); err != nil {
		return nil, err
	}
	return goals, nil
}

func parseGoalJSON(raw string) (*goalRow, error) {
	// The server wraps in "Goal created:\n<json>" or "Goal updated:\n<json>"
	// or "Deleted goal: <id>"
	if strings.Contains(raw, "Deleted goal") {
		return nil, nil
	}
	idx := strings.Index(raw, "\n")
	if idx >= 0 {
		raw = raw[idx+1:]
	}
	var g goalRow
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// ---------------------------------------------------------------------------
// Goals output
// ---------------------------------------------------------------------------

func formatGoalsList(raw string, format OutputFormat) string {
	goals, err := parseGoalsJSON(raw)
	if err != nil {
		return raw // fallback to raw
	}
	if len(goals) == 0 {
		return "No goals found."
	}

	switch format {
	case OutputTable:
		return renderGoalsTable(goals)
	case OutputSummary:
		return renderGoalsSummary(goals)
	default:
		return raw
	}
}

func renderGoalsTable(goals []goalRow) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 3, ' ', 0)

	fmt.Fprintln(w, "ID\tTITLE\tSTATUS\tPROGRESS\tPRIORITY\tLABELS\tDEADLINE")
	fmt.Fprintln(w, "--\t-----\t------\t--------\t--------\t------\t--------")
	for _, g := range goals {
		shortID := shortenID(g.ID, 24)
		pbar := progressBar(g.Progress, 10)
		labels := strings.Join(g.Labels, ", ")
		deadline := g.Deadline
		if deadline == "" {
			deadline = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s %d%%\t%d\t%s\t%s\n",
			shortID, truncate(g.Title, 30), g.Status, pbar, g.Progress, g.Priority, truncate(labels, 20), deadline)
	}
	w.Flush()
	return b.String()
}

func renderGoalsSummary(goals []goalRow) string {
	var lines []string
	for _, g := range goals {
		shortID := shortenID(g.ID, 16)
		pbar := progressBar(g.Progress, 10)
		labels := strings.Join(g.Labels, ", ")
		line := fmt.Sprintf("%s %s %s [%s] %s %s",
			g.StatusIcon(), shortID, g.Title, pbar, labels, g.Deadline)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// renderGoalDetail renders a single goal with TODO subtask breakdown.
func renderGoalDetail(g *goalRow) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Goal: %s\n", g.Title)
	fmt.Fprintf(&b, "  ID:       %s\n", g.ID)
	fmt.Fprintf(&b, "  Status:   %s\n", g.Status)
	fmt.Fprintf(&b, "  Priority: %d/10\n", g.Priority)
	fmt.Fprintf(&b, "  Progress: %s %d%%\n", progressBar(g.Progress, 20), g.Progress)

	if g.Deadline != "" {
		fmt.Fprintf(&b, "  Deadline: %s\n", g.Deadline)
	}
	if len(g.Labels) > 0 {
		fmt.Fprintf(&b, "  Labels:   %s\n", strings.Join(g.Labels, ", "))
	}
	if g.CreatedAt > 0 {
		fmt.Fprintf(&b, "  Created:  %s\n", formatGoalTime(g.CreatedAt))
	}
	if g.UpdatedAt > 0 {
		fmt.Fprintf(&b, "  Updated:  %s\n", formatGoalTime(g.UpdatedAt))
	}

	// Subtask breakdown from description
	if subtasks := extractSubtasks(g.Description); len(subtasks) > 0 {
		fmt.Fprintf(&b, "\nSubtasks:\n")
		for _, s := range subtasks {
			mark := "[ ]"
			if s.done {
				mark = "[x]"
			}
			fmt.Fprintf(&b, "  %s %s\n", mark, s.text)
		}
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers to clean server responses
// ---------------------------------------------------------------------------

// stripCodeFence removes markdown code fences (```json ... ```) and any
// leading text before a JSON array. If the response doesn't start with '[',
// it finds the first '[' and last ']' and extracts the JSON array.
func stripCodeFence(raw string) string {
	// Strip markdown code fences
	raw = regexp.MustCompile("(?s)```(?:json)?\n?").ReplaceAllString(raw, "")
	raw = strings.TrimSpace(raw)

	// If the response doesn't start with '[', extract the JSON array
	// by finding the first '[' and the last ']'.
	if !strings.HasPrefix(raw, "[") {
		lb := strings.IndexByte(raw, '[')
		rb := strings.LastIndexByte(raw, ']')
		if lb >= 0 && rb > lb {
			raw = raw[lb : rb+1]
		}
	}
	return strings.TrimSpace(raw)
}

// ---------------------------------------------------------------------------
// Suggest output
// ---------------------------------------------------------------------------

// suggestRow is a parsed suggestion.
type suggestRow struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

func parseSuggestJSON(raw string) ([]suggestRow, error) {
	raw = stripCodeFence(raw)
	var rows []suggestRow
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func formatSuggest(raw string, format OutputFormat) string {
	rows, err := parseSuggestJSON(raw)
	if err != nil {
		// If JSON parse fails, return raw text as-is (it's probably an error message)
		return raw
	}
	if len(rows) == 0 {
		return "No suggestions."
	}

	switch format {
	case OutputTable:
		var b strings.Builder
		w := tabwriter.NewWriter(&b, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "TYPE\tTITLE\tPRIORITY\tDESCRIPTION")
		fmt.Fprintln(w, "----\t-----\t--------\t-----------")
		for _, r := range rows {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
				truncate(r.Type, 12),
				truncate(r.Title, 28),
				r.Priority,
				truncate(r.Description, 60))
		}
		w.Flush()
		return b.String()
	case OutputSummary:
		var lines []string
		for _, r := range rows {
			lines = append(lines, fmt.Sprintf("[%s] %s", r.Type, r.Title))
		}
		return strings.Join(lines, "\n")
	default:
		return raw
	}
}

// ---------------------------------------------------------------------------
// Timeline output
// ---------------------------------------------------------------------------

// timelineRow is a parsed timeline entry.
type timelineRow struct {
	Key       string `json:"key"`
	CreatedAt string `json:"created_at"`
	Value     struct {
		Content string   `json:"content"`
		Summary string   `json:"summary"`
		Tags    []string `json:"tags"`
	} `json:"value"`
}

func parseTimelineJSON(raw string) ([]timelineRow, error) {
	idx := strings.Index(raw, "\n")
	if idx >= 0 {
		raw = raw[idx+1:]
	}
	var entries []timelineRow
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func formatTimeline(raw string, format OutputFormat) string {
	entries, err := parseTimelineJSON(raw)
	if err != nil {
		return raw
	}
	if len(entries) == 0 {
		return "No timeline entries found."
	}

	switch format {
	case OutputTable:
		return renderTimelineTable(entries)
	case OutputSummary:
		return renderTimelineSummary(entries)
	default:
		return raw
	}
}

func renderTimelineTable(entries []timelineRow) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "DATE\tKEY\tCONTENT")
	fmt.Fprintln(w, "----\t---\t-------")
	for _, e := range entries {
		date := truncate(e.CreatedAt, 10)
		content := e.Value.Content
		if content == "" {
			content = e.Value.Summary
		}
		shortKey := shortenID(e.Key, 28)
		fmt.Fprintf(w, "%s\t%s\t%s\n", date, shortKey, truncate(content, 60))
	}
	w.Flush()
	return b.String()
}

func renderTimelineSummary(entries []timelineRow) string {
	var lines []string
	for _, e := range entries {
		date := truncate(e.CreatedAt, 10)
		content := e.Value.Content
		if content == "" {
			content = e.Value.Summary
		}
		lines = append(lines, fmt.Sprintf("[%s] %s", date, truncate(content, 80)))
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Search output
// ---------------------------------------------------------------------------

// searchRow is a parsed search result.
type searchRow struct {
	Key   string  `json:"key"`
	Score float64 `json:"score"`
	Value struct {
		Content string `json:"content"`
		Summary string `json:"summary"`
	} `json:"value"`
}

func parseSearchJSON(raw string) ([]searchRow, error) {
	var rows []searchRow
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func formatSearch(raw string, format OutputFormat) string {
	rows, err := parseSearchJSON(raw)
	if err != nil {
		return raw
	}
	if len(rows) == 0 {
		return "No results found."
	}

	switch format {
	case OutputTable:
		var b strings.Builder
		w := tabwriter.NewWriter(&b, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "RELEVANCE\tKEY\tCONTENT")
		fmt.Fprintln(w, "---------\t---\t-------")
		for _, r := range rows {
			score := fmt.Sprintf("%.0f%%", r.Score*100)
			content := r.Value.Content
			if content == "" {
				content = r.Value.Summary
			}
			shortKey := shortenID(r.Key, 28)
			fmt.Fprintf(w, "%s\t%s\t%s\n", score, shortKey, truncate(content, 60))
		}
		w.Flush()
		return b.String()
	case OutputSummary:
		var lines []string
		for _, r := range rows {
			score := fmt.Sprintf("%.0f%%", r.Score*100)
			content := r.Value.Content
			if content == "" {
				content = r.Value.Summary
			}
			lines = append(lines, fmt.Sprintf("[%s] %s", score, truncate(content, 80)))
		}
		return strings.Join(lines, "\n")
	default:
		return raw
	}
}

// ---------------------------------------------------------------------------
// Get output (single memory)
// ---------------------------------------------------------------------------

func formatGet(raw string, format OutputFormat) string {
	// Server may wrap with "Memory 'key':\n<json>"
	idx := strings.Index(raw, "\n")
	var maybeJSON string
	if idx >= 0 {
		maybeJSON = raw[idx+1:]
	} else {
		maybeJSON = raw
	}

	var v map[string]any
	if err := json.Unmarshal([]byte(maybeJSON), &v); err != nil {
		return raw
	}

	switch format {
	case OutputTable:
		b := &strings.Builder{}
		// Extract key fields
		content, _ := v["content"].(string)
		summary, _ := v["summary"].(string)
		tags, _ := v["tags"].([]any)
		createdAt, _ := v["created_at"].(string)

		if content != "" {
			fmt.Fprintf(b, "Content:   %s\n", truncate(content, 80))
		}
		if summary != "" {
			fmt.Fprintf(b, "Summary:   %s\n", truncate(summary, 80))
		}
		if createdAt != "" {
			fmt.Fprintf(b, "Created:   %s\n", createdAt)
		}
		if len(tags) > 0 {
			var tagStrs []string
			for _, t := range tags {
				tagStrs = append(tagStrs, fmt.Sprint(t))
			}
			fmt.Fprintf(b, "Tags:      %s\n", strings.Join(tagStrs, ", "))
		}
		return b.String()
	case OutputSummary:
		content, _ := v["content"].(string)
		summary, _ := v["summary"].(string)
		if content != "" {
			return content
		}
		if summary != "" {
			return summary
		}
		return raw
	default:
		return raw
	}
}

// ---------------------------------------------------------------------------
// List output (memory keys)
// ---------------------------------------------------------------------------

func formatList(raw string, format OutputFormat) string {
	var keys []string

	// Try parsing as JSON array first
	if err := json.Unmarshal([]byte(raw), &keys); err == nil {
		if len(keys) == 0 {
			return "No keys found."
		}
		return renderKeys(keys, format)
	}

	// Not JSON — likely "Found N memories:\nkey1\nkey2"
	// Split by newline and take lines after the header
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Found ") {
			continue
		}
		keys = append(keys, line)
	}
	if len(keys) == 0 {
		return "No keys found."
	}
	return renderKeys(keys, format)
}

func renderKeys(keys []string, format OutputFormat) string {
	switch format {
	case OutputTable:
		var b strings.Builder
		w := tabwriter.NewWriter(&b, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "INDEX\tKEY")
		fmt.Fprintln(w, "-----\t---")
		for i, k := range keys {
			fmt.Fprintf(w, "%d\t%s\n", i+1, k)
		}
		w.Flush()
		return b.String()
	case OutputSummary:
		var lines []string
		for _, k := range keys {
			lines = append(lines, k)
		}
		return strings.Join(lines, "\n")
	default:
		b, _ := json.MarshalIndent(keys, "", "  ")
		return string(b)
	}
}

// ---------------------------------------------------------------------------
// Subtask extraction from description (mirrors storage.go logic)
// ---------------------------------------------------------------------------

type subtask struct {
	text string
	done bool
}

func extractSubtasks(description string) []subtask {
	re := regexp.MustCompile(`(?m)^\s*[-*+]\s+\[([ xX])\]\s+(.+)`)
	var result []subtask
	for _, match := range re.FindAllStringSubmatch(description, -1) {
		done := strings.EqualFold(match[1], "x")
		result = append(result, subtask{text: match[2], done: done})
	}
	return result
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (g *goalRow) StatusIcon() string {
	switch g.Status {
	case "active":
		return "▶"
	case "completed":
		return "✓"
	case "archived":
		return "▦"
	default:
		return "○"
	}
}

func progressBar(pct, width int) string {
	filled := pct * width / 100
	if filled > width {
		filled = width
	}
	empty := width - filled
	return strings.Repeat("█", filled) + strings.Repeat("░", empty)
}

func shortenID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen-3] + "..."
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// formatGoalTime converts a Unix timestamp (int64) to a human-readable date string.
func formatGoalTime(ts int64) string {
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}
