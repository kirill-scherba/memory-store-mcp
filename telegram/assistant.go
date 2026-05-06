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

	reply := formatSearchResults(results, lang)
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

	reply := formatGoalsList(goals, lang)
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

	reply := formatGoalDetail(g, lang)
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

	reply := formatTimelineResults(entries, lang)
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

	reply := formatSuggestions(suggestions, lang)
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

	reply := formatContextResult(ctx, lang)
	b.sendText(msg.Chat.ID, reply)
}