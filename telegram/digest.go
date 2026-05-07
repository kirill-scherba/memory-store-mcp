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

// cmdDigest handles /digest — generates a daily/weekly summary.
func (b *Bot) cmdDigest(msg *tgbotapi.Message, lang string) {
	args := strings.TrimSpace(msg.CommandArguments())
	period := "day"
	limit := 20

	if args != "" {
		switch args {
		case "day", "d":
			period = "day"
		case "week", "w":
			period = "week"
		case "month", "m":
			period = "month"
		default:
			b.sendText(msg.Chat.ID, t("digest_usage", lang))
			return
		}
	}

	b.sendText(msg.Chat.ID, fmt.Sprintf(t("digest_loading", lang), period))

	// Get timeline for the period
	jsonStr, err := b.funcs.GetTimeline("", "", limit)
	if err != nil {
		log.Printf("⚠ GetTimeline error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("digest_error", lang), err))
		return
	}

	var entries []TimelineEntry
	if err := json.Unmarshal([]byte(jsonStr), &entries); err != nil {
		log.Printf("⚠ Error parsing timeline JSON: %v", err)
		b.sendText(msg.Chat.ID, t("digest_parse_error", lang))
		return
	}

	if len(entries) == 0 {
		b.sendText(msg.Chat.ID, t("digest_empty", lang))
		return
	}

	// Count by type
	noteCount := 0
	goalCount := 0
	var contentItems []string

	for _, e := range entries {
		summary := e.Value.Summary
		if summary == "" {
			summary = e.Value.Content
		}
		if summary == "" {
			summary = e.Key
		}
		contentItems = append(contentItems, summary)
	}
	noteCount = len(contentItems)

	// Get active goals
	goalsJSON, err := b.funcs.ListGoals("active", nil)
	var activeGoals int
	if err == nil {
		var goals []Goal
		if err := json.Unmarshal([]byte(goalsJSON), &goals); err == nil {
			activeGoals = len(goals)
			goalCount = activeGoals
		}
	}

	// Build digest
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>📊 %s %s</b>\n\n", t("digest_title_prefix", lang), period))
	sb.WriteString(fmt.Sprintf("📝 <b>%s</b> %d\n", t("digest_notes_label", lang), noteCount))
	sb.WriteString(fmt.Sprintf("🎯 <b>%s</b> %d\n", t("digest_goals_label", lang), goalCount))
	sb.WriteString(fmt.Sprintf("🔍 <b>%s</b> %d\n\n", t("digest_events_label", lang), len(entries)))

	if len(contentItems) > 0 {
		sb.WriteString(fmt.Sprintf("<b>%s</b>\n", t("digest_recent_label", lang)))
		maxItems := 5
		if len(contentItems) < maxItems {
			maxItems = len(contentItems)
		}
		for i := 0; i < maxItems; i++ {
			sb.WriteString(fmt.Sprintf("  • %s\n", escapeHTML(contentItems[i])))
		}
	}

	result := sb.String()
	if len(result) > 4000 {
		result = result[:4000] + "\n..."
	}

	b.sendText(msg.Chat.ID, result)
}