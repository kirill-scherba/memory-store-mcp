// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// escapeHTML escapes special HTML characters.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&")
	s = strings.ReplaceAll(s, "<", "<")
	s = strings.ReplaceAll(s, ">", ">")
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

// cmdSearch handles /search query — semantic search.
func (b *Bot) cmdSearch(msg *tgbotapi.Message, lang string) {
	query := strings.TrimSpace(msg.CommandArguments())
	if query == "" {
		b.sendText(msg.Chat.ID, t("search_usage", lang))
		return
	}

	b.sendText(msg.Chat.ID, t("searching", lang))

	jsonStr, err := b.funcs.Search(query, 10)
	if err != nil {
		log.Printf("⚠ Search error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("search_error", lang), err))
		return
	}

	var results []SearchResult
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		log.Printf("⚠ Error parsing search JSON: %v", err)
		b.sendText(msg.Chat.ID, t("search_parse_error", lang))
		return
	}

	if len(results) == 0 {
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("search_no_results", lang), escapeHTML(query)))
		return
	}

	var reply string
	reply = fmt.Sprintf(t("search_results_title", lang), escapeHTML(query))
	for i, r := range results {
		var mv MemoryValue
		var summary string
		if err := json.Unmarshal([]byte(r.Value), &mv); err == nil {
			summary = mv.Summary
			if summary == "" {
				summary = truncateText(mv.Content, 80)
			}
		} else {
			summary = truncateText(r.Value, 80)
		}
		reply += fmt.Sprintf("%d. <b>%s</b> (%.0f%%)\n", i+1, escapeHTML(summary), r.Score*100)
	}

	if len(reply) > 4000 {
		reply = reply[:4000] + "\n..."
	}
	b.sendText(msg.Chat.ID, reply)
}

// cmdGoals handles /goals — list active goals.
func (b *Bot) cmdGoals(msg *tgbotapi.Message, lang string) {
	jsonStr, err := b.funcs.ListGoals("active", nil)
	if err != nil {
		log.Printf("⚠ ListGoals error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("goals_error", lang), err))
		return
	}

	var goals []Goal
	if err := json.Unmarshal([]byte(jsonStr), &goals); err != nil {
		log.Printf("⚠ Error parsing goals JSON: %v", err)
		b.sendText(msg.Chat.ID, t("goals_parse_error", lang))
		return
	}

	if len(goals) == 0 {
		b.sendText(msg.Chat.ID, t("goals_empty", lang))
		return
	}

	var reply string
	reply = t("goals_title", lang)
	for i, g := range goals {
		statusEmoji := "⏳"
		if g.Progress >= 100 {
			statusEmoji = "✅"
		}
		reply += fmt.Sprintf(
			"%d. %s <b>%s</b>\n   📝 %s\n   📊 %d%% 🔥 %d/10\n   🆔 <code>%s</code>\n\n",
			i+1, statusEmoji, escapeHTML(g.Title), escapeHTML(g.Description),
			g.Progress, g.Priority, g.ID,
		)
	}

	if len(reply) > 4000 {
		reply = reply[:4000] + "\n..."
	}
	b.sendText(msg.Chat.ID, reply)
}

// cmdGoal handles /goal <id> — show goal details.
func (b *Bot) cmdGoal(msg *tgbotapi.Message, lang string) {
	id := strings.TrimSpace(msg.CommandArguments())
	if id == "" {
		b.sendText(msg.Chat.ID, t("goal_usage", lang))
		return
	}

	jsonStr, err := b.funcs.GetGoal(id)
	if err != nil {
		log.Printf("⚠ GetGoal error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("goal_error", lang), err))
		return
	}

	var g Goal
	if err := json.Unmarshal([]byte(jsonStr), &g); err != nil {
		log.Printf("⚠ Error parsing goal JSON: %v", err)
		b.sendText(msg.Chat.ID, t("goal_parse_error", lang))
		return
	}

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
		"<b>%s %s</b>\n\n📝 %s\n\n📊 <b>%s</b> %d%%\n🔥 <b>%s</b> %d/10\n📌 <b>%s</b> %s\n🏷 <b>%s</b> %s\n🆔 <code>%s</code>",
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

	b.sendText(msg.Chat.ID, reply)
}

// cmdTimeline handles /timeline — recent activity.
func (b *Bot) cmdTimeline(msg *tgbotapi.Message, lang string) {
	b.sendText(msg.Chat.ID, t("loading_timeline", lang))

	jsonStr, err := b.funcs.GetTimeline("", "", 10)
	if err != nil {
		log.Printf("⚠ GetTimeline error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("timeline_error", lang), err))
		return
	}

	var entries []TimelineEntry
	if err := json.Unmarshal([]byte(jsonStr), &entries); err != nil {
		log.Printf("⚠ Error parsing timeline JSON: %v", err)
		b.sendText(msg.Chat.ID, t("timeline_parse_error", lang))
		return
	}

	if len(entries) == 0 {
		b.sendText(msg.Chat.ID, t("timeline_empty", lang))
		return
	}

	var reply string
	reply = t("timeline_title", lang)
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
	b.sendText(msg.Chat.ID, reply)
}

// cmdSuggest handles /suggest — proactive suggestions.
func (b *Bot) cmdSuggest(msg *tgbotapi.Message, lang string) {
	b.sendText(msg.Chat.ID, t("suggest_thinking", lang))

	// Include language preference in the context so LLM generates in the right language
	context := fmt.Sprintf("telegram user request (language: %s)", lang)
	jsonStr, err := b.funcs.Suggest(context, 5, lang)
	if err != nil {
		log.Printf("⚠ Suggest error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("suggest_error", lang), err))
		return
	}

	var suggestions []Suggestion
	if err := json.Unmarshal([]byte(jsonStr), &suggestions); err != nil {
		log.Printf("⚠ Error parsing suggestions JSON: %v", err)
		b.sendText(msg.Chat.ID, t("suggest_parse_error", lang))
		return
	}

	if len(suggestions) == 0 {
		b.sendText(msg.Chat.ID, t("suggest_empty", lang))
		return
	}

	var reply string
	reply = t("suggest_title", lang)
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
	b.sendText(msg.Chat.ID, reply)
}

// cmdContext handles /context — current context.
func (b *Bot) cmdContext(msg *tgbotapi.Message, lang string) {
	b.sendText(msg.Chat.ID, t("context_loading", lang))

	query := strings.TrimSpace(msg.CommandArguments())
	if query == "" {
		query = "current context"
	}

	jsonStr, err := b.funcs.GetContext(query, 5)
	if err != nil {
		log.Printf("⚠ GetContext error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("context_error", lang), err))
		return
	}

	var ctx ContextResult
	if err := json.Unmarshal([]byte(jsonStr), &ctx); err != nil {
		log.Printf("⚠ Error parsing context JSON: %v", err)
		b.sendText(msg.Chat.ID, t("context_parse_error", lang))
		return
	}

	var reply string
	reply = t("context_title", lang)

	if len(ctx.Goals) > 0 {
		reply += t("context_goals_title", lang)
		for i, g := range ctx.Goals {
			reply += fmt.Sprintf("  %d. %s (%d%%)\n", i+1, escapeHTML(g.Title), g.Progress)
		}
		reply += "\n"
	}

	if len(ctx.Memories) > 0 {
		reply += t("context_memories_title", lang)
		for i, m := range ctx.Memories {
			summary := m.Value.Summary
			if summary == "" {
				summary = truncateText(m.Value.Content, 80)
			}
			reply += fmt.Sprintf("%d. <b>%s</b> (%.0f%%)\n   📅 %s\n\n",
				i+1, escapeHTML(summary), m.Score*100, m.CreatedAt)
		}
	}

	if reply == t("context_title", lang) {
		reply += t("context_no_data", lang)
	}

	b.sendText(msg.Chat.ID, reply)
}