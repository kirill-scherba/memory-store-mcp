// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Shared helpers — used across all telegram files.
// ---------------------------------------------------------------------------

// escapeHTML escapes special HTML characters for Telegram HTML parse mode.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// truncateText truncates text to maxLen runes.
func truncateText(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}

// formatLabels joins labels into a comma-separated string.
func formatLabels(labels []string) string {
	if len(labels) == 0 {
		return "\xe2\x80\x94"
	}
	return strings.Join(labels, ", ")
}

// ---------------------------------------------------------------------------
// Shared formatters — used by both /command handlers and LLM agent dispatch.
// All functions take parsed Go types and a language code, return formatted text.
// ---------------------------------------------------------------------------

// formatGoalsList formats a list of goals.
func formatGoalsList(goals []Goal, lang string) string {
	if len(goals) == 0 {
		if lang == "ru" {
			return "\xf0\x9f\x93\x8b \xd0\x9d\xd0\xb5\xd1\x82 \xd1\x86\xd0\xb5\xd0\xbb\xd0\xb5\xd0\xb9."
		}
		return "\xf0\x9f\x93\x8b No goals."
	}

	var reply string
	if lang == "ru" {
		reply = fmt.Sprintf("\xf0\x9f\x8e\xaf <b>\xd0\x90\xd0\xba\xd1\x82\xd0\xb8\xd0\xb2\xd0\xbd\xd1\x8b\xd0\xb5 \xd1\x86\xd0\xb5\xd0\xbb\xd0\xb8 (%d):</b>\n\n", len(goals))
	} else {
		reply = fmt.Sprintf("\xf0\x9f\x8e\xaf <b>Active goals (%d):</b>\n\n", len(goals))
	}
	for i, g := range goals {
		statusEmoji := "\xe2\x8f\xb3"
		if g.Progress >= 100 {
			statusEmoji = "\xe2\x9c\x85"
		}
		reply += fmt.Sprintf(
			"%d. %s <b>%s</b>\n   \xf0\x9f\x93\x9d %s\n   \xf0\x9f\x93\x8a %d%% \xf0\x9f\x94\xa5 %d/10\n   \xf0\x9f\x86\x94 <code>%s</code>\n\n",
			i+1, statusEmoji, escapeHTML(g.Title), escapeHTML(g.Description),
			g.Progress, g.Priority, g.ID,
		)
	}
	if len(reply) > 4000 {
		reply = reply[:4000] + "\n..."
	}
	return reply
}

// formatGoalDetail formats a single goal detail view.
func formatGoalDetail(g Goal, lang string) string {
	statusEmoji := map[string]string{
		"active":    "\xe2\x8f\xb3",
		"completed": "\xe2\x9c\x85",
		"archived":  "\xf0\x9f\x93\xa6",
	}[g.Status]
	if statusEmoji == "" {
		statusEmoji = "\xe2\x9d\x93"
	}

	var statusLabel string
	switch lang {
	case "en":
		statusLabel = g.Status
	default:
		switch g.Status {
		case "active":
			statusLabel = "\xd0\xb0\xd0\xba\xd1\x82\xd0\xb8\xd0\xb2\xd0\xbd\xd0\xb0"
		case "completed":
			statusLabel = "\xd0\xb7\xd0\xb0\xd0\xb2\xd0\xb5\xd1\x80\xd1\x88\xd0\xb5\xd0\xbd\xd0\xb0"
		case "archived":
			statusLabel = "\xd0\xb2 \xd0\xb0\xd1\x80\xd1\x85\xd0\xb8\xd0\xb2\xd0\xb5"
		default:
			statusLabel = g.Status
		}
	}

	labels := formatLabels(g.Labels)
	reply := fmt.Sprintf(
		"<b>%s %s</b>\n\n\xf0\x9f\x93\x9d %s\n\n\xf0\x9f\x93\x8a <b>%s</b> %d%%\n\xf0\x9f\x94\xa5 <b>%s</b> %d/10\n\xf0\x9f\x93\x8c <b>%s</b> %s\n\xf0\x9f\x8f\xb7 <b>%s</b> %s\n\xf0\x9f\x86\x94 <code>%s</code>",
		statusEmoji, escapeHTML(g.Title), escapeHTML(g.Description),
		map[string]string{"ru": "\xd0\x9f\xd1\x80\xd0\xbe\xd0\xb3\xd1\x80\xd0\xb5\xd1\x81\xd1\x81:", "en": "Progress:"}[lang], g.Progress,
		map[string]string{"ru": "\xd0\x9f\xd1\x80\xd0\xb8\xd0\xbe\xd1\x80\xd0\xb8\xd1\x82\xd0\xb5\xd1\x82:", "en": "Priority:"}[lang], g.Priority,
		map[string]string{"ru": "\xd0\xa1\xd1\x82\xd0\xb0\xd1\x82\xd1\x83\xd1\x81:", "en": "Status:"}[lang], statusLabel,
		map[string]string{"ru": "\xd0\x9c\xd0\xb5\xd1\x82\xd0\xba\xd0\xb8:", "en": "Labels:"}[lang], labels, g.ID,
	)

	if g.Deadline != "" {
		reply += fmt.Sprintf("\n\xf0\x9f\x93\x85 <b>%s</b> %s",
			map[string]string{"ru": "\xd0\x94\xd0\xb5\xd0\xb4\xd0\xbb\xd0\xb0\xd0\xb9\xd0\xbd:", "en": "Deadline:"}[lang], g.Deadline)
	}
	return reply
}

// formatSearchResults formats search results.
func formatSearchResults(results []SearchResult, lang string) string {
	if len(results) == 0 {
		if lang == "ru" {
			return "\xf0\x9f\x94\x8d \xd0\x9d\xd0\xb8\xd1\x87\xd0\xb5\xd0\xb3\xd0\xbe \xd0\xbd\xd0\xb5 \xd0\xbd\xd0\xb0\xd0\xb9\xd0\xb4\xd0\xb5\xd0\xbd\xd0\xbe."
		}
		return "\xf0\x9f\x94\x8d Nothing found."
	}

	var reply string
	if lang == "ru" {
		reply = fmt.Sprintf("\xf0\x9f\x94\x8d \xd0\x9d\xd0\xb0\xd0\xb9\xd0\xb4\xd0\xb5\xd0\xbd\xd0\xbe %d \xd1\x80\xd0\xb5\xd0\xb7\xd1\x83\xd0\xbb\xd1\x8c\xd1\x82\xd0\xb0\xd1\x82\xd0\xbe\xd0\xb2:\n\n", len(results))
	} else {
		reply = fmt.Sprintf("\xf0\x9f\x94\x8d Found %d results:\n\n", len(results))
	}
	for i, r := range results {
		var mv MemoryValue
		title := ""
		if err := json.Unmarshal([]byte(r.Value), &mv); err == nil {
			title = mv.Summary
			if title == "" {
				title = mv.Content
			}
		}
		if title == "" {
			title = r.Key
		}
		reply += fmt.Sprintf("%d. <b>%s</b>\n   \xf0\x9f\x94\x91 %s  (%.0f%%)\n\n", i+1, escapeHTML(title), escapeHTML(r.Key), r.Score*100)
	}
	if len(reply) > 4000 {
		reply = reply[:4000] + "\n..."
	}
	return reply
}

// formatTimelineResults formats timeline entries.
func formatTimelineResults(entries []TimelineEntry, lang string) string {
	if len(entries) == 0 {
		if lang == "ru" {
			return "\xf0\x9f\x93\x85 \xd0\x9d\xd0\xb5\xd1\x82 \xd1\x81\xd0\xbe\xd0\xb1\xd1\x8b\xd1\x82\xd0\xb8\xd0\xb9."
		}
		return "\xf0\x9f\x93\x85 No events."
	}

	var reply string
	if lang == "ru" {
		reply = fmt.Sprintf("\xf0\x9f\x93\x85 <b>\xd0\xa1\xd0\xbe\xd0\xb1\xd1\x8b\xd1\x82\xd0\xb8\xd1\x8f (%d):</b>\n\n", len(entries))
	} else {
		reply = fmt.Sprintf("\xf0\x9f\x93\x85 <b>Events (%d):</b>\n\n", len(entries))
	}
	for i, e := range entries {
		summary := e.Value.Summary
		if summary == "" {
			summary = truncateText(e.Value.Content, 60)
		}
		date := e.CreatedAt
		if len(date) > 10 {
			date = date[:10]
		}
		reply += fmt.Sprintf("%d. [%s] <b>%s</b>\n   %s\n\n",
			i+1, date, escapeHTML(summary), escapeHTML(e.Key))
	}
	if len(reply) > 4000 {
		reply = reply[:4000] + "\n..."
	}
	return reply
}

// formatSuggestions formats suggestion results.
func formatSuggestions(suggestions []Suggestion, lang string) string {
	if len(suggestions) == 0 {
		if lang == "ru" {
			return "\xf0\x9f\x92\xa1 \xd0\x9d\xd0\xb5\xd1\x82 \xd0\xbf\xd1\x80\xd0\xb5\xd0\xb4\xd0\xbb\xd0\xbe\xd0\xb6\xd0\xb5\xd0\xbd\xd0\xb8\xd0\xb9."
		}
		return "\xf0\x9f\x92\xa1 No suggestions."
	}

	var reply string
	if lang == "ru" {
		reply = "\xf0\x9f\x92\xa1 <b>\xd0\x9f\xd1\x80\xd0\xb5\xd0\xb4\xd0\xbb\xd0\xbe\xd0\xb6\xd0\xb5\xd0\xbd\xd0\xb8\xd1\x8f:</b>\n\n"
	} else {
		reply = "\xf0\x9f\x92\xa1 <b>Suggestions:</b>\n\n"
	}
	for i, s := range suggestions {
		typeEmoji := map[string]string{
			"reminder":       "\xe2\x8f\xb0",
			"followup":       "\xf0\x9f\x94\x84",
			"goal_next_step": "\xf0\x9f\x8e\xaf",
			"insight":        "\xf0\x9f\x92\xa1",
		}[s.Type]
		if typeEmoji == "" {
			typeEmoji = "\xe2\x80\xa2"
		}
		reply += fmt.Sprintf("%d. %s <b>%s</b>\n   %s\n\n",
			i+1, typeEmoji, escapeHTML(s.Title), escapeHTML(s.Description))
	}
	if len(reply) > 4000 {
		reply = reply[:4000] + "\n..."
	}
	return reply
}

// formatContextResult formats context results.
func formatContextResult(ctx ContextResult, lang string) string {
	var reply string
	if lang == "ru" {
		reply = "\xf0\x9f\x93\x8a <b>\xd0\x9a\xd0\xbe\xd0\xbd\xd1\x82\xd0\xb5\xd0\xba\xd1\x81\xd1\x82:</b>\n\n"
	} else {
		reply = "\xf0\x9f\x93\x8a <b>Context:</b>\n\n"
	}

	if len(ctx.Goals) > 0 {
		if lang == "ru" {
			reply += "<b>\xd0\x90\xd0\xba\xd1\x82\xd0\xb8\xd0\xb2\xd0\xbd\xd1\x8b\xd0\xb5 \xd1\x86\xd0\xb5\xd0\xbb\xd0\xb8:</b>\n"
		} else {
			reply += "<b>Active goals:</b>\n"
		}
		for i, g := range ctx.Goals {
			reply += fmt.Sprintf("  %d. %s (%d%%)\n", i+1, escapeHTML(g.Title), g.Progress)
		}
		reply += "\n"
	}

	if len(ctx.Memories) > 0 {
		if lang == "ru" {
			reply += fmt.Sprintf("<b>\xd0\x9f\xd0\xb0\xd0\xbc\xd1\x8f\xd1\x82\xd1\x8c (%d):</b>\n", len(ctx.Memories))
		} else {
			reply += fmt.Sprintf("<b>Memories (%d):</b>\n", len(ctx.Memories))
		}
		for i, m := range ctx.Memories {
			summary := m.Value.Summary
			if summary == "" {
				summary = truncateText(m.Value.Content, 80)
			}
			reply += fmt.Sprintf("%d. <b>%s</b> (%.0f%%)\n   \xf0\x9f\x93\x85 %s\n\n",
				i+1, escapeHTML(summary), m.Score*100, m.CreatedAt)
		}
	}

	if len(ctx.Goals) == 0 && len(ctx.Memories) == 0 {
		if lang == "ru" {
			reply += "\xd0\x9d\xd0\xb5\xd1\x82 \xd0\xb4\xd0\xb0\xd0\xbd\xd0\xbd\xd1\x8b\xd1\x85 \xd0\xb2 \xd0\xbf\xd0\xb0\xd0\xbc\xd1\x8f\xd1\x82\xd0\xb8.\n"
		} else {
			reply += "No data in memory.\n"
		}
	}

	if len(reply) > 4000 {
		reply = reply[:4000] + "\n..."
	}
	return reply
}
