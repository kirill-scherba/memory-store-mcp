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
	s = strings.ReplaceAll(s, "&", "&#38;")
	s = strings.ReplaceAll(s, "<", "&#60;")
	s = strings.ReplaceAll(s, ">", "&#62;")
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
		return "—"
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
			return "📋 Нет целей."
		}
		return "📋 No goals."
	}

	var reply string
	if lang == "ru" {
		reply = fmt.Sprintf("🎯 <b>Активные цели (%d):</b>\n\n", len(goals))
	} else {
		reply = fmt.Sprintf("🎯 <b>Active goals (%d):</b>\n\n", len(goals))
	}
	for i, g := range goals {
		statusEmoji := "⏳"
		if g.Progress >= 100 {
			statusEmoji = "✅"
		}
		reply += fmt.Sprintf(
			"%d. %s <b>%s</b>\n   📝 %s\n   📊 %d%% 🔥 %d/10\n   🔔 <code>%s</code>\n\n",
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
		"active":    "⏳",
		"completed": "✅",
		"archived":  "📦",
	}[g.Status]
	if statusEmoji == "" {
		statusEmoji = "❓"
	}

	var statusLabel string
	switch lang {
	case "en":
		statusLabel = g.Status
	default:
		switch g.Status {
		case "active":
			statusLabel = "активна"
		case "completed":
			statusLabel = "завершена"
		case "archived":
			statusLabel = "в архиве"
		default:
			statusLabel = g.Status
		}
	}

	labels := formatLabels(g.Labels)
	reply := fmt.Sprintf(
		"<b>%s %s</b>\n\n📝 %s\n\n📊 <b>%s</b> %d%%\n🔥 <b>%s</b> %d/10\n📌 <b>%s</b> %s\n🏷 <b>%s</b> %s\n🔔 <code>%s</code>",
		statusEmoji, escapeHTML(g.Title), escapeHTML(g.Description),
		map[string]string{"ru": "Прогресс:", "en": "Progress:"}[lang], g.Progress,
		map[string]string{"ru": "Приоритет:", "en": "Priority:"}[lang], g.Priority,
		map[string]string{"ru": "Статус:", "en": "Status:"}[lang], statusLabel,
		map[string]string{"ru": "Метки:", "en": "Labels:"}[lang], labels, g.ID,
	)

	if g.Deadline != "" {
		reply += fmt.Sprintf("\n📅 <b>%s</b> %s",
			map[string]string{"ru": "Дедлайн:", "en": "Deadline:"}[lang], g.Deadline)
	}
	return reply
}

// formatSearchResults formats search results.
func formatSearchResults(results []SearchResult, lang string) string {
	if len(results) == 0 {
		if lang == "ru" {
			return "🔍 Ничего не найдено."
		}
		return "🔍 Nothing found."
	}

	var reply string
	if lang == "ru" {
		reply = fmt.Sprintf("🔍 Найдено %d результатов:\n\n", len(results))
	} else {
		reply = fmt.Sprintf("🔍 Found %d results:\n\n", len(results))
	}
	for i, r := range results {
		var mv MemoryValue
		title := ""
		content := ""
		if err := json.Unmarshal([]byte(r.Value), &mv); err == nil {
			title = mv.Summary
			if title == "" {
				title = mv.Content
			}
			content = mv.Content
			if content == "" {
				content = mv.Summary
			}
			if title == content {
				content = ""
			}
		}
		if title == "" {
			title = r.Key
		}
		reply += fmt.Sprintf("%d. <b>%s</b>\n", i+1, escapeHTML(title))
		if content != "" {
			reply += fmt.Sprintf("   %s\n", escapeHTML(truncateText(content, 200)))
		}
		reply += fmt.Sprintf("   🔑 <code>%s</code>  (%.0f%%)\n\n", escapeHTML(r.Key), r.Score*100)
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
			return "📅 Нет событий."
		}
		return "📅 No events."
	}

	var reply string
	if lang == "ru" {
		reply = fmt.Sprintf("📅 <b>События (%d):</b>\n\n", len(entries))
	} else {
		reply = fmt.Sprintf("📅 <b>Events (%d):</b>\n\n", len(entries))
	}
	for i, e := range entries {
		summary := e.Value.Summary
		if summary == "" {
			summary = truncateText(e.Value.Content, 60)
		}
		if summary == "" {
			summary = e.Key
		}
		date := e.CreatedAt
		if len(date) > 10 {
			date = date[:10]
		}
		reply += fmt.Sprintf("%d. [%s] <b>%s</b>\n   🔑 <code>%s</code>\n\n",
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
			return "💡 Нет предложений."
		}
		return "💡 No suggestions."
	}

	var reply string
	if lang == "ru" {
		reply = "💡 <b>Предложения:</b>\n\n"
	} else {
		reply = "💡 <b>Suggestions:</b>\n\n"
	}
	for i, s := range suggestions {
		typeEmoji := map[string]string{
			"reminder":       "⏰",
			"followup":       "🔄",
			"goal_next_step": "🎯",
			"insight":        "💡",
		}[s.Type]
		if typeEmoji == "" {
			typeEmoji = "•"
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
		reply = "📊 <b>Контекст:</b>\n\n"
	} else {
		reply = "📊 <b>Context:</b>\n\n"
	}

	if len(ctx.Goals) > 0 {
		if lang == "ru" {
			reply += "<b>Активные цели:</b>\n"
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
			reply += fmt.Sprintf("<b>Память (%d):</b>\n", len(ctx.Memories))
		} else {
			reply += fmt.Sprintf("<b>Memories (%d):</b>\n", len(ctx.Memories))
		}
		for i, m := range ctx.Memories {
			summary := m.Value.Summary
			if summary == "" {
				summary = truncateText(m.Value.Content, 80)
			}
			if summary == "" {
				summary = m.Key
			}
		reply += fmt.Sprintf("%d. <b>%s</b> (%.0f%%)\n   📅 %s\n   🔑 <code>%s</code>\n\n",
			i+1, escapeHTML(summary), m.Score*100, m.CreatedAt, escapeHTML(m.Key))
		}
	}

	if len(ctx.Goals) == 0 && len(ctx.Memories) == 0 {
		if lang == "ru" {
			reply += "Нет данных в памяти.\n"
		} else {
			reply += "No data in memory.\n"
		}
	}

	if len(reply) > 4000 {
		reply = reply[:4000] + "\n..."
	}
	return reply
}