// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// ---------------------------------------------------------------------------
// LLM Agent — replaces the old notebook/classifier approach.
//
// The agent receives the user's raw message, builds a system prompt describing
// available memory operations, sends everything to the LLM, and the LLM responds
// with either:
//  1. A function call (JSON) — "call": "save_note", "create_goal", "search", etc.
//  2. A plain text answer to the user's question.
//
// The agent ALWAYS answers naturally (it's a full assistant), but when it detects
// that the user wants to store something, create a goal, search, etc., it returns
// a structured function call that the bot executes.
// ---------------------------------------------------------------------------

// AgentCommand is a structured command that the LLM can request.
type AgentCommand struct {
	Call       string `json:"call"`       // operation name
	Query      string `json:"query"`      // for search/get_context
	Title      string `json:"title"`      // for save_note / create_goal / update_goal
	Text       string `json:"text"`       // for save_note / extract
	Content    string `json:"content"`    // for save_note (description)
	Priority   int    `json:"priority"`   // for create_goal / update_goal
	Progress   int    `json:"progress"`   // for update_goal
	Status     string `json:"status"`     // for update_goal
	GoalID     string `json:"goal_id"`    // for update_goal / delete_goal
	Key        string `json:"key"`        // for memory_get / memory_delete
	Limit      int    `json:"limit"`      // for search / get_context
	Labels     string `json:"labels"`     // JSON array string for goals
	Lang       string `json:"lang"`       // language
	From       string `json:"from"`       // timeline from
	To         string `json:"to"`         // timeline to
	Answer     string `json:"answer"`     // plain text answer to user
}

// buildAgentSystemPrompt builds the system prompt for the LLM agent.
func buildAgentSystemPrompt(lang string, funcs BotFuncs) string {
	// Determine available operations based on what callbacks are set
	avail := func(name string) string {
		switch name {
		case "save_note":
			if funcs.SaveNote != nil {
				return "✅ available"
			}
		case "create_goal":
			if funcs.CreateGoal != nil {
				return "✅ available"
			}
		case "update_goal":
			if funcs.UpdateGoal != nil {
				return "✅ available"
			}
		case "delete_memory":
			if funcs.DeleteMemory != nil {
				return "✅ available"
			}
		case "search":
			if funcs.Search != nil {
				return "✅ available"
			}
		case "list_goals":
			if funcs.ListGoals != nil {
				return "✅ available"
			}
		case "get_goal":
			if funcs.GetGoal != nil {
				return "✅ available"
			}
		case "get_timeline":
			if funcs.GetTimeline != nil {
				return "✅ available"
			}
		case "get_context":
			if funcs.GetContext != nil {
				return "✅ available"
			}
		case "suggest":
			if funcs.Suggest != nil {
				return "✅ available"
			}
		}
		return "❌ unavailable"
	}

	prompt := `You are an AI assistant integrated with a long-term memory system. 
Your purpose is to help the user manage their MEMORY and GOALS.

## Core responsibilities:
1. Answer user questions about stored memories, goals, and timelines.
2. When the user wants to SAVE something — use save_note.
3. When the user expresses an intention, task, or project — use create_goal.
4. When the user wants to UPDATE a goal (change status, progress, priority) — use update_goal.
5. When the user asks about stored information — use search or get_context.
6. When the user asks about goals — use list_goals or get_goal.
7. When the user asks about timeline — use get_timeline.
8. When the user wants suggestions — use suggest.
9. When the user asks to delete something — use delete_memory.
10. When the user asks a general question not requiring storage operations — ANSWER NATURALLY in plain text.

## Available functions:
`

	prompt += fmt.Sprintf("  - save_note (%s): Save a note/memory\n", avail("save_note"))
	prompt += fmt.Sprintf("  - create_goal (%s): Create a new tracked goal\n", avail("create_goal"))
	prompt += fmt.Sprintf("  - update_goal (%s): Update goal status/progress/priority\n", avail("update_goal"))
	prompt += fmt.Sprintf("  - delete_memory (%s): Delete a memory by key\n", avail("delete_memory"))
	prompt += fmt.Sprintf("  - search (%s): Semantic search across memories\n", avail("search"))
	prompt += fmt.Sprintf("  - list_goals (%s): List goals by status\n", avail("list_goals"))
	prompt += fmt.Sprintf("  - get_goal (%s): Get a specific goal by ID\n", avail("get_goal"))
	prompt += fmt.Sprintf("  - get_timeline (%s): Get timeline of events\n", avail("get_timeline"))
	prompt += fmt.Sprintf("  - get_context (%s): Get aggregated context\n", avail("get_context"))
	prompt += fmt.Sprintf("  - suggest (%s): Get proactive suggestions\n", avail("suggest"))

	prompt += `
## Response format:
You MUST respond with a JSON object. The JSON object can be one of two types:

### Type 1: Function call
When the user wants to perform an operation:
{
  "call": "<function_name>",
  "title": "...",
  "content": "...",
  "priority": <number>,
  "labels": "[\"label1\",\"label2\"]",
  "query": "...",
  "limit": <number>,
  "key": "...",
  "goal_id": "...",
  "status": "...",
  "progress": <number>,
  "from": "...",
  "to": "...",
  "text": "...",
  "answer": ""
}

### Type 2: Plain text answer
When the user asks a question or chats:
{
  "call": "",
  "answer": "Your natural language response here..."
}

## CRITICAL RULES:
- If the user says something like "запомни", "сохрани", "save", "remember" → use save_note.
- If the user expresses an intention like "я буду", "I will", "хочу сделать", "plan to" → use create_goal.
- If the user asks a question → ANSWER NATURALLY in the "answer" field. Do NOT make up function calls.
- NEVER make up data. Only use information the user explicitly provides.
- "priority" is 0-10 (default 5). "progress" is 0-100.
- "labels" is a JSON array string like "[\"work\",\"project\"]" or empty string.
- "limit" defaults to 10 if not specified.
- "status" for goals: "active", "completed", "archived".
- Respond in the SAME LANGUAGE as the user's message (Russian or English).
- When the user provides information that should be stored, ALWAYS prefer saving it.
- When in doubt between answer and function call — prefer function call.

`

	if lang == "ru" {
		prompt = strings.ReplaceAll(prompt, "You are an AI assistant", "Ты — AI-ассистент")
		prompt += "\nВАЖНО: Отвечай на РУССКОМ языке. Если пользователь пишет по-русски — отвечай по-русски.\n"
		prompt += "Если пользователь просто общается, отвечай естественно в поле 'answer'.\n"
		prompt += "Если пользователь хочет что-то сохранить — используй save_note.\n"
		prompt += "Если пользователь выражает намерение/план — используй create_goal.\n"
		prompt += "Если пользователь просит обновить цель — используй update_goal.\n"
	}

	return prompt
}

// processWithLLMAgent sends the user message to the LLM with the system prompt
// and parses the response into an AgentCommand.
func processWithLLMAgent(userMessage string, lang string, funcs BotFuncs) (*AgentCommand, error) {
	systemPrompt := buildAgentSystemPrompt(lang, funcs)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	answer, err := funcs.LLMRequest(systemPrompt, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM agent request failed: %w", err)
	}

	return parseAgentResponse(answer)
}

// parseAgentResponse parses the LLM JSON response into an AgentCommand.
func parseAgentResponse(raw string) (*AgentCommand, error) {
	raw = strings.TrimSpace(raw)

	// Try direct JSON parse first
	var cmd AgentCommand
	if err := json.Unmarshal([]byte(raw), &cmd); err == nil {
		if cmd.Call != "" || cmd.Answer != "" {
			return &cmd, nil
		}
	}

	// Try to extract JSON from markdown code blocks or surrounding text
	cleaned := raw
	// Check for ```json ... ``` blocks
	if idx := strings.Index(raw, "```json"); idx >= 0 {
		start := idx + 7 // skip ```json
		if end := strings.Index(raw[start:], "```"); end >= 0 {
			cleaned = strings.TrimSpace(raw[start : start+end])
		}
	} else if idx := strings.Index(raw, "```"); idx >= 0 {
		start := idx + 3
		if end := strings.Index(raw[start:], "```"); end >= 0 {
			cleaned = strings.TrimSpace(raw[start : start+end])
		}
	}

	// Try to find JSON object with braces
	if braceStart := strings.Index(cleaned, "{"); braceStart >= 0 {
		if braceEnd := strings.LastIndex(cleaned, "}"); braceEnd > braceStart {
			cleaned = cleaned[braceStart : braceEnd+1]
		}
	}

	if err := json.Unmarshal([]byte(cleaned), &cmd); err == nil {
		if cmd.Call != "" || cmd.Answer != "" {
			return &cmd, nil
		}
	}

	// If all parsing fails, treat the entire response as a plain text answer
	log.Printf("⚠ LLM agent response not valid JSON, treating as plain answer: %s", truncate(raw, 100))
	return &AgentCommand{Answer: raw}, nil
}

// dispatchAgentCommand executes the command requested by the LLM agent.
// Returns the text response to send back to the user.
func dispatchAgentCommand(cmd *AgentCommand, funcs BotFuncs, lang string) string {
	if cmd.Answer != "" {
		return cmd.Answer
	}

	switch cmd.Call {
	case "save_note":
		if funcs.SaveNote == nil {
			return "Save note is not available."
		}
		title := cmd.Title
		content := cmd.Content
		if content == "" {
			content = cmd.Text
		}
		if title == "" {
			// Use first line as title
			lines := strings.SplitN(content, "\n", 2)
			title = lines[0]
		}
		labels := parseLabels(cmd.Labels)
		key, err := funcs.SaveNote(title, content, labels)
		if err != nil {
			log.Printf("⚠ save_note error: %v", err)
			return fmt.Sprintf("❌ Failed to save note: %v", err)
		}
		log.Printf("✅ saved note: %s", key)
		if lang == "ru" {
			return fmt.Sprintf("✅ Запомнил! (ключ: %s)", key)
		}
		return fmt.Sprintf("✅ Saved! (key: %s)", key)

	case "create_goal":
		if funcs.CreateGoal == nil {
			return "Create goal is not available."
		}
		labels := parseLabels(cmd.Labels)
		result, err := funcs.CreateGoal(cmd.Title, cmd.Content, cmd.Priority, labels)
		if err != nil {
			log.Printf("⚠ create_goal error: %v", err)
			return fmt.Sprintf("❌ Failed to create goal: %v", err)
		}
		log.Printf("✅ created goal: %s", result)
		if lang == "ru" {
			return fmt.Sprintf("✅ Цель создана! %s", result)
		}
		return fmt.Sprintf("✅ Goal created! %s", result)

	case "update_goal":
		if funcs.UpdateGoal == nil {
			return "Update goal is not available."
		}
		if cmd.GoalID == "" {
			if lang == "ru" {
				return "❌ Не указан ID цели. Используй /goals чтобы увидеть список целей и их ID."
			}
			return "❌ No goal ID specified. Use /goals to see goals and their IDs."
		}
		labels := parseLabels(cmd.Labels)
		result, err := funcs.UpdateGoal(cmd.GoalID, cmd.Title, cmd.Content, cmd.Status, cmd.Priority, cmd.Progress, labels)
		if err != nil {
			log.Printf("⚠ update_goal error: %v", err)
			return fmt.Sprintf("❌ Failed to update goal: %v", err)
		}
		log.Printf("✅ updated goal: %s", cmd.GoalID)
		if lang == "ru" {
			return fmt.Sprintf("✅ Цель обновлена! %s", result)
		}
		return fmt.Sprintf("✅ Goal updated! %s", result)

	case "delete_memory":
		if funcs.DeleteMemory == nil {
			return "Delete memory is not available."
		}
		if cmd.Key == "" {
			if lang == "ru" {
				return "❌ Не указан ключ для удаления."
			}
			return "❌ No key specified for deletion."
		}
		if err := funcs.DeleteMemory(cmd.Key); err != nil {
			log.Printf("⚠ delete_memory error: %v", err)
			return fmt.Sprintf("❌ Failed to delete: %v", err)
		}
		if lang == "ru" {
			return fmt.Sprintf("✅ Удалено: %s", cmd.Key)
		}
		return fmt.Sprintf("✅ Deleted: %s", cmd.Key)

	case "search":
		if funcs.Search == nil {
			return "Search is not available."
		}
		limit := cmd.Limit
		if limit <= 0 {
			limit = 10
		}
		results, err := funcs.Search(cmd.Query, limit)
		if err != nil {
			log.Printf("⚠ search error: %v", err)
			return fmt.Sprintf("❌ Search failed: %v", err)
		}
		return formatSearchResults(results, lang)

	case "list_goals":
		if funcs.ListGoals == nil {
			return "List goals is not available."
		}
		labels := parseLabels(cmd.Labels)
		status := cmd.Status
		if status == "" {
			status = "active"
		}
		results, err := funcs.ListGoals(status, labels)
		if err != nil {
			log.Printf("⚠ list_goals error: %v", err)
			return fmt.Sprintf("❌ Failed to list goals: %v", err)
		}
		return formatGoalsList(results, lang)

	case "get_goal":
		if funcs.GetGoal == nil {
			return "Get goal is not available."
		}
		if cmd.GoalID == "" {
			if lang == "ru" {
				return "❌ Не указан ID цели."
			}
			return "❌ No goal ID specified."
		}
		result, err := funcs.GetGoal(cmd.GoalID)
		if err != nil {
			return fmt.Sprintf("❌ %v", err)
		}
		return formatGoalDetail(result, lang)

	case "get_timeline":
		if funcs.GetTimeline == nil {
			return "Get timeline is not available."
		}
		limit := cmd.Limit
		if limit <= 0 {
			limit = 10
		}
		results, err := funcs.GetTimeline(cmd.From, cmd.To, limit)
		if err != nil {
			log.Printf("⚠ get_timeline error: %v", err)
			return fmt.Sprintf("❌ Failed to get timeline: %v", err)
		}
		return formatTimelineResults(results, lang)

	case "get_context":
		if funcs.GetContext == nil {
			return "Get context is not available."
		}
		limit := cmd.Limit
		if limit <= 0 {
			limit = 5
		}
		result, err := funcs.GetContext(cmd.Query, limit)
		if err != nil {
			log.Printf("⚠ get_context error: %v", err)
			return fmt.Sprintf("❌ Failed to get context: %v", err)
		}
		return formatContextResult(result, lang)

	case "suggest":
		if funcs.Suggest == nil {
			return "Suggest is not available."
		}
		limit := cmd.Limit
		if limit <= 0 {
			limit = 5
		}
		results, err := funcs.Suggest(cmd.Query, limit, lang)
		if err != nil {
			log.Printf("⚠ suggest error: %v", err)
			return fmt.Sprintf("❌ Failed to get suggestions: %v", err)
		}
		return formatSuggestions(results, lang)

	default:
		if lang == "ru" {
			return fmt.Sprintf("❌ Неизвестная команда: %s", cmd.Call)
		}
		return fmt.Sprintf("❌ Unknown command: %s", cmd.Call)
	}
}

// ---------------------------------------------------------------------------
// Formatters for LLM agent results
// ---------------------------------------------------------------------------

func formatSearchResults(resultsJSON string, lang string) string {
	var results []struct {
		Key   string `json:"key"`
		Value struct {
			Content string   `json:"content"`
			Summary string   `json:"summary"`
			Tags    []string `json:"tags"`
		} `json:"value"`
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(resultsJSON), &results); err != nil {
		return resultsJSON
	}
	if len(results) == 0 {
		if lang == "ru" {
			return "🔍 Ничего не найдено."
		}
		return "🔍 Nothing found."
	}

	var sb strings.Builder
	if lang == "ru" {
		sb.WriteString(fmt.Sprintf("🔍 Найдено %d результатов:\n\n", len(results)))
	} else {
		sb.WriteString(fmt.Sprintf("🔍 Found %d results:\n\n", len(results)))
	}
	for i, r := range results {
		summary := r.Value.Summary
		if summary == "" {
			summary = truncate(r.Value.Content, 80)
		}
		sb.WriteString(fmt.Sprintf("%d. <b>%s</b> (%.0f%%)\n", i+1, escapeHTML(summary), r.Score*100))
		if len(r.Value.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("   🏷 %s\n", strings.Join(r.Value.Tags, ", ")))
		}
		sb.WriteString(fmt.Sprintf("   <code>%s</code>\n", escapeHTML(r.Key)))
	}
	return sb.String()
}

func formatGoalsList(goalsJSON string, lang string) string {
	var goals []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		Priority    int      `json:"priority"`
		Progress    int      `json:"progress"`
		Labels      []string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(goalsJSON), &goals); err != nil {
		return goalsJSON
	}
	if len(goals) == 0 {
		if lang == "ru" {
			return "📋 Нет целей."
		}
		return "📋 No goals."
	}

	var sb strings.Builder
	if lang == "ru" {
		sb.WriteString(fmt.Sprintf("📋 <b>Цели (%d):</b>\n\n", len(goals)))
	} else {
		sb.WriteString(fmt.Sprintf("📋 <b>Goals (%d):</b>\n\n", len(goals)))
	}
	for _, g := range goals {
		statusEmoji := "⏳"
		if g.Status == "completed" {
			statusEmoji = "✅"
		} else if g.Status == "archived" {
			statusEmoji = "📦"
		}
		sb.WriteString(fmt.Sprintf("%s <b>%s</b> [%d%%] (p%d)\n", statusEmoji, escapeHTML(g.Title), g.Progress, g.Priority))
		if g.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", escapeHTML(truncate(g.Description, 100))))
		}
		if len(g.Labels) > 0 {
			sb.WriteString(fmt.Sprintf("   🏷 %s\n", strings.Join(g.Labels, ", ")))
		}
		sb.WriteString(fmt.Sprintf("   <code>%s</code>\n", escapeHTML(g.ID)))
	}
	return sb.String()
}

func formatGoalDetail(goalJSON string, lang string) string {
	var goal struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		Priority    int      `json:"priority"`
		Progress    int      `json:"progress"`
		Deadline    string   `json:"deadline"`
		Labels      []string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(goalJSON), &goal); err != nil {
		return goalJSON
	}

	statusEmoji := "⏳"
	statusText := "active"
	if goal.Status == "completed" {
		statusEmoji = "✅"
		statusText = "completed"
	} else if goal.Status == "archived" {
		statusEmoji = "📦"
		statusText = "archived"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s <b>%s</b>\n", statusEmoji, escapeHTML(goal.Title)))
	sb.WriteString(fmt.Sprintf("📊 Progress: %d%%\n", goal.Progress))
	sb.WriteString(fmt.Sprintf("📌 Priority: %d\n", goal.Priority))
	sb.WriteString(fmt.Sprintf("📋 Status: %s\n", statusText))
	if goal.Description != "" {
		sb.WriteString(fmt.Sprintf("\n%s\n", escapeHTML(goal.Description)))
	}
	if goal.Deadline != "" {
		sb.WriteString(fmt.Sprintf("\n📅 Deadline: %s\n", goal.Deadline))
	}
	if len(goal.Labels) > 0 {
		sb.WriteString(fmt.Sprintf("🏷 %s\n", strings.Join(goal.Labels, ", ")))
	}
	sb.WriteString(fmt.Sprintf("\n<code>%s</code>", escapeHTML(goal.ID)))
	return sb.String()
}

func formatTimelineResults(timelineJSON string, lang string) string {
	var entries []struct {
		Key   string `json:"key"`
		Value struct {
			Content string `json:"content"`
			Summary string `json:"summary"`
		} `json:"value"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal([]byte(timelineJSON), &entries); err != nil {
		return timelineJSON
	}
	if len(entries) == 0 {
		if lang == "ru" {
			return "📅 Нет событий."
		}
		return "📅 No events."
	}

	var sb strings.Builder
	if lang == "ru" {
		sb.WriteString(fmt.Sprintf("📅 <b>События (%d):</b>\n\n", len(entries)))
	} else {
		sb.WriteString(fmt.Sprintf("📅 <b>Events (%d):</b>\n\n", len(entries)))
	}
	for _, e := range entries {
		summary := e.Value.Summary
		if summary == "" {
			summary = truncate(e.Value.Content, 80)
		}
		date := e.CreatedAt
		if len(date) > 10 {
			date = date[:10]
		}
		sb.WriteString(fmt.Sprintf("• [%s] <b>%s</b>\n", date, escapeHTML(summary)))
		sb.WriteString(fmt.Sprintf("  <code>%s</code>\n", escapeHTML(e.Key)))
	}
	return sb.String()
}

func formatContextResult(ctxJSON string, lang string) string {
	var ctx struct {
		Query      string `json:"query"`
		TotalCount int    `json:"total_count"`
		Memories   []struct {
			Key   string `json:"key"`
			Value struct {
				Content string `json:"content"`
				Summary string `json:"summary"`
			} `json:"value"`
			Score float64 `json:"score"`
		} `json:"memories"`
		Goals []struct {
			Title    string `json:"title"`
			Progress int    `json:"progress"`
			Status   string `json:"status"`
		} `json:"goals"`
	}
	if err := json.Unmarshal([]byte(ctxJSON), &ctx); err != nil {
		return ctxJSON
	}

	var sb strings.Builder
	if lang == "ru" {
		sb.WriteString(fmt.Sprintf("📊 <b>Контекст:</b> %s\n\n", escapeHTML(ctx.Query)))
	} else {
		sb.WriteString(fmt.Sprintf("📊 <b>Context:</b> %s\n\n", escapeHTML(ctx.Query)))
	}

	if len(ctx.Goals) > 0 {
		if lang == "ru" {
			sb.WriteString("<b>Активные цели:</b>\n")
		} else {
			sb.WriteString("<b>Active goals:</b>\n")
		}
		for _, g := range ctx.Goals {
			sb.WriteString(fmt.Sprintf("  • %s [%d%%]\n", escapeHTML(g.Title), g.Progress))
		}
		sb.WriteString("\n")
	}

	if len(ctx.Memories) > 0 {
		if lang == "ru" {
			sb.WriteString(fmt.Sprintf("<b>Память (%d):</b>\n", len(ctx.Memories)))
		} else {
			sb.WriteString(fmt.Sprintf("<b>Memories (%d):</b>\n", len(ctx.Memories)))
		}
		for _, m := range ctx.Memories {
			summary := m.Value.Summary
			if summary == "" {
				summary = truncate(m.Value.Content, 80)
			}
			sb.WriteString(fmt.Sprintf("  • %s (%.0f%%)\n", escapeHTML(summary), m.Score*100))
		}
	}

	if ctx.TotalCount == 0 && len(ctx.Goals) == 0 {
		if lang == "ru" {
			sb.WriteString("Нет данных в памяти.\n")
		} else {
			sb.WriteString("No data in memory.\n")
		}
	}

	return sb.String()
}

func formatSuggestions(suggestJSON string, lang string) string {
	var suggestions []struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    int    `json:"priority"`
	}
	if err := json.Unmarshal([]byte(suggestJSON), &suggestions); err != nil {
		return suggestJSON
	}
	if len(suggestions) == 0 {
		if lang == "ru" {
			return "💡 Нет предложений."
		}
		return "💡 No suggestions."
	}

	var sb strings.Builder
	if lang == "ru" {
		sb.WriteString("💡 <b>Предложения:</b>\n\n")
	} else {
		sb.WriteString("💡 <b>Suggestions:</b>\n\n")
	}
	for _, s := range suggestions {
		typeEmoji := "💭"
		switch s.Type {
		case "reminder":
			typeEmoji = "⏰"
		case "followup":
			typeEmoji = "🔄"
		case "goal_next_step":
			typeEmoji = "🎯"
		case "insight":
			typeEmoji = "💡"
		}
		sb.WriteString(fmt.Sprintf("%s <b>%s</b> (p%d)\n", typeEmoji, escapeHTML(s.Title), s.Priority))
		if s.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", escapeHTML(s.Description)))
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseLabels parses a labels JSON string into a slice.
func parseLabels(labelsJSON string) []string {
	if labelsJSON == "" {
		return nil
	}
	var labels []string
	if err := json.Unmarshal([]byte(labelsJSON), &labels); err != nil {
		// Try comma-separated
		parts := strings.Split(labelsJSON, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				labels = append(labels, p)
			}
		}
	}
	return labels
}

// truncate shortens a string to maxLen runes, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}