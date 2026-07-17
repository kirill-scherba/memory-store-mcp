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

// cmdFind handles /find <keyword> — exact keyword search.
func (b *Bot) cmdFind(msg *tgbotapi.Message, lang string) {
	keyword := strings.TrimSpace(msg.CommandArguments())
	if keyword == "" {
		b.sendText(msg.Chat.ID, t("find_usage", lang))
		return
	}

	b.sendText(msg.Chat.ID, t("find_searching", lang))

	jsonStr, err := b.funcs.Find(keyword, 20)
	if err != nil {
		log.Printf("⚠ Find error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("find_error", lang), err))
		return
	}

	var results []FindResult
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		log.Printf("⚠ Error parsing find JSON: %v", err)
		b.sendText(msg.Chat.ID, t("find_parse_error", lang))
		return
	}

	reply := formatFindResults(results, lang)
	b.sendText(msg.Chat.ID, reply)
}

// cmdDig handles /dig <query> [--keywords k1,k2] [--window 2h] [--max 10].
func (b *Bot) cmdDig(msg *tgbotapi.Message, lang string) {
	query, keywords, window, max := b.parseDigArgs(msg.CommandArguments())
	if query == "" {
		b.sendText(msg.Chat.ID, t("dig_usage", lang))
		return
	}

	b.sendText(msg.Chat.ID, t("dig_searching", lang))

	jsonStr, err := b.funcs.Dig(query, keywords, window, max)
	if err != nil {
		log.Printf("⚠ Dig error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("dig_error", lang), err))
		return
	}

	var result DigResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		log.Printf("⚠ Error parsing dig JSON: %v", err)
		b.sendText(msg.Chat.ID, t("dig_parse_error", lang))
		return
	}

	reply := formatDigResults(result, lang)
	b.sendText(msg.Chat.ID, reply)
}

// parseDigArgs parses /dig arguments.
// Supports: "query", "query --keywords a,b --window 1d --max 5".
func (b *Bot) parseDigArgs(args string) (query string, keywords []string, window string, max int) {
	window = "2h"
	max = 10

	parts := strings.Fields(args)
	var queryParts []string
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		switch p {
		case "--keywords":
			if i+1 < len(parts) {
				keywords = strings.Split(parts[i+1], ",")
				for j := range keywords {
					keywords[j] = strings.TrimSpace(keywords[j])
				}
				i++
			}
		case "--window":
			if i+1 < len(parts) {
				window = parts[i+1]
				i++
			}
		case "--max":
			if i+1 < len(parts) {
				fmt.Sscanf(parts[i+1], "%d", &max)
				i++
			}
		default:
			queryParts = append(queryParts, p)
		}
	}
	query = strings.Join(queryParts, " ")
	return
}

// cmdList handles /list [prefix] — list memory keys by prefix.
func (b *Bot) cmdList(msg *tgbotapi.Message, lang string) {
	prefix := strings.TrimSpace(msg.CommandArguments())

	jsonStr, err := b.funcs.List(prefix)
	if err != nil {
		log.Printf("⚠ List error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("list_error", lang), err))
		return
	}

	var keys []string
	if err := json.Unmarshal([]byte(jsonStr), &keys); err != nil {
		log.Printf("⚠ Error parsing list JSON: %v", err)
		b.sendText(msg.Chat.ID, t("list_parse_error", lang))
		return
	}

	reply := formatListResults(keys, lang)
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
	context := "telegram user request: /suggest"
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
	log.Printf("Suggestions: %v", suggestions)

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
