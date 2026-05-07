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
	Call     string          `json:"call"`     // operation name
	Query    string          `json:"query"`    // for search/get_context
	Title    string          `json:"title"`    // for save_note / create_goal / update_goal
	Text     string          `json:"text"`     // for save_note / extract
	Content  string          `json:"content"`  // for save_note (description)
	Priority int             `json:"priority"` // for create_goal / update_goal
	Progress int             `json:"progress"` // for update_goal
	Status   string          `json:"status"`   // for update_goal
	GoalID   string          `json:"goal_id"`  // for update_goal / delete_goal
	Key      string          `json:"key"`      // for memory_get / memory_delete
	Limit    int             `json:"limit"`    // for search / get_context
	Labels   json.RawMessage `json:"labels"`   // JSON array string or raw array for goals
	Deadline string          `json:"deadline"` // for create_goal / update_goal
	Lang     string          `json:"lang"`     // language
	From     string          `json:"from"`     // timeline from
	To       string          `json:"to"`       // timeline to
	Answer   string          `json:"answer"`   // plain text answer to user
}

// buildAgentSystemPrompt builds the ONE system prompt for the LLM agent.
// The agent answers in the same language as the user's question.
// lang is the language code ("ru" or "en") that the agent MUST use in its answer.
func buildAgentSystemPrompt(funcs BotFuncs, lang string) string {
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
		case "delete_goal":
			if funcs.DeleteGoal != nil {
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
		case "get_memory":
			if funcs.GetMemory != nil {
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

	prompt := `You are an AI assistant for a memory and goal management system.

CRITICAL — YOU MUST FOLLOW THESE EXAMPLES:

User: "Привет!"
You: {"call":"","answer":"Привет! Чем могу помочь?"}

User: "Hello!"
You: {"call":"","answer":"Hi there! How can I help you?"}

User: "Что ты умеешь?"
You: {"call":"","answer":"Я могу сохранять заметки, создавать цели, искать в памяти, показывать таймлайн и предлагать что делать дальше. Что вас интересует?"}

User: "What can you do?"
You: {"call":"","answer":"I can save notes, create goals, search memory, show timeline, and suggest next actions. What would you like to do?"}

User: "запомни что я люблю пиццу"
You: {"call":"save_note","title":"Кирилл любит пиццу","content":"Кирилл любит пиццу","labels":["предпочтения"]}

User: "remind me that the server runs on port 8080"
You: {"call":"save_note","title":"Server port","content":"The server runs on port 8080","labels":["technical"]}

IMPORTANT: You MUST answer in LANGUAGE CODE: ` + lang + `. If lang is "ru" — answer in Russian. If lang is "en" — answer in English. This is a strict requirement.

CAPABILITIES:
- Answer questions using context from the user's memory
- Save notes and facts as memories
- Create, update, and delete goals
- Search memories by semantic similarity
- List goals with their progress
- Show timeline of events
- Suggest proactive next actions

RULES:
- By default answer naturally. NEVER save or create goals unless explicitly asked.
- GREETINGS ("hello", "hi", "привет", "здравствуйте"): ALWAYS answer with a plain greeting, NEVER call a function.
- When user asks about goals → call list_goals
- When user says "remember that...", "save this...", "note that...", "запомни...", "сохрани...", "напомни..." → call save_note
- When user says "I want to learn...", "new goal:", "goal:", "новая цель:", "хочу научиться...", "создай цель..." → call create_goal
- When user says "what are my goals", "show goals", "my goals", "мои цели", "покажи цели" → call list_goals
- When user asks "what do you know about...", "search for...", "найди...", "что ты знаешь о..." → call search
- When user says "удали цель", "delete goal" → call delete_goal

FUNCTIONS:
`
	prompt += fmt.Sprintf("  - save_note (%s): save a note\n", avail("save_note"))
	prompt += fmt.Sprintf("  - create_goal (%s): create a goal\n", avail("create_goal"))
	prompt += fmt.Sprintf("  - update_goal (%s): update a goal\n", avail("update_goal"))
	prompt += fmt.Sprintf("  - delete_memory (%s): delete a memory by key\n", avail("delete_memory"))
	prompt += fmt.Sprintf("  - delete_goal (%s): delete a goal by ID\n", avail("delete_goal"))
	prompt += fmt.Sprintf("  - search (%s): search memories by query\n", avail("search"))
	prompt += fmt.Sprintf("  - list_goals (%s): list goals (status: active/completed/archived)\n", avail("list_goals"))
	prompt += fmt.Sprintf("  - get_goal (%s): get a single goal by ID\n", avail("get_goal"))
	prompt += fmt.Sprintf("  - get_memory (%s): get a memory by key\n", avail("get_memory"))
	prompt += fmt.Sprintf("  - get_timeline (%s): get timeline of events\n", avail("get_timeline"))
	prompt += fmt.Sprintf("  - get_context (%s): get relevant context from memory\n", avail("get_context"))
	prompt += fmt.Sprintf("  - suggest (%s): get proactive suggestions\n", avail("suggest"))

	prompt += fmt.Sprintf(`RESPOND ONLY WITH RAW JSON%sno markdown, no code fences, no explanations.

WRONG (do NOT do this):
  %sjson
  {"call":"","answer":"hello"}
  %s
RIGHT (do this):
  {"call":"","answer":"hello"}

Function call format (use ONLY when user EXPLICITLY asks):
  {"call":"save_note","title":"...","content":"...","labels":[]}
  {"call":"create_goal","title":"...","content":"...","priority":5,"labels":["label1","label2"],"deadline":""}
  {"call":"list_goals","status":"active","labels":[]}
  {"call":"search","query":"...","limit":10}
  {"call":"get_timeline","from":"","to":"","limit":10}
  {"call":"get_context","query":"...","limit":5}
  {"call":"suggest","query":"...","limit":5}
  {"call":"update_goal","goal_id":"...","title":"","content":"","status":"","priority":-1,"progress":-1,"labels":[]}
  {"call":"delete_memory","key":"..."}
  {"call":"delete_goal","goal_id":"..."}
  {"call":"get_goal","goal_id":"..."}
  {"call":"get_memory","key":"..."}

Plain answer (default - use for greetings and most messages):
  {"call":"","answer":"your response in the user's language here"}

CRITICAL RULES:
- When user says hello/hi/privet - ALWAYS answer with a plain greeting, NEVER call a function.
- NEVER call save_note, create_goal, or any other function unless the user EXPLICITLY asks.
- Do NOT wrap your JSON response in backticks or markdown. Return raw JSON only.`, " — ", "```", "```")

	return prompt
}

// processWithLLMAgent sends the user message to the LLM with the system prompt
// and parses the response into an AgentCommand. Logs the full interaction.
func processWithLLMAgent(userMessage string, lang string, funcs BotFuncs) (*AgentCommand, error) {
	systemPrompt := buildAgentSystemPrompt(funcs, lang)

	messages := []ChatMessage{
		{Role: "user", Content: userMessage},
	}

	answer, err := funcs.LLMRequest(systemPrompt, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM agent request failed: %w", err)
	}

	cmd, parseErr := parseAgentResponse(answer)

	// Log the full interaction: what we sent to LLM and what we got back
	var parsedDump string
	if cmd != nil {
		if cmd.Call != "" {
			parsedDump = fmt.Sprintf("call=%s title=%q query=%q", cmd.Call, cmd.Title, cmd.Query)
		} else if cmd.Answer != "" {
			parsedDump = fmt.Sprintf("answer=%q", truncateText(cmd.Answer, 200))
		}
	}
	LogLLM(systemPrompt, userMessage, answer, parsedDump)

	return cmd, parseErr
}

// parseAgentResponse parses the LLM JSON response into an AgentCommand.
// Tries: extract JSON from backtick fences -> direct JSON -> find {...} -> fallback
func parseAgentResponse(raw string) (*AgentCommand, error) {
	raw = strings.TrimSpace(raw)

	// Check for empty or trivial JSON that cannot be a valid command
	if raw == "" || raw == "{}" || raw == `{"call":"","answer":""}` {
		return nil, fmt.Errorf("empty or trivial JSON response: %q", truncateText(raw, 80))
	}

	// ── Helper: try to parse any JSON object from text ──
	// Looks for the outermost { ... }, unmarshals, and checks for valid command.
	tryParseBraceJSON := func(text string) *AgentCommand {
		braceStart := strings.Index(text, "{")
		if braceStart < 0 {
			return nil
		}
		braceEnd := strings.LastIndex(text, "}")
		if braceEnd <= braceStart {
			return nil
		}
		candidate := strings.TrimSpace(text[braceStart : braceEnd+1])
		var cmd AgentCommand
		if err := json.Unmarshal([]byte(candidate), &cmd); err == nil {
			if cmd.Call != "" || cmd.Answer != "" {
				return &cmd
			}
		}
		return nil
	}

	// ── 0. Try direct JSON parse first (before any normalization) ──
	var cmd AgentCommand
	if err := json.Unmarshal([]byte(raw), &cmd); err == nil {
		if cmd.Call != "" || cmd.Answer != "" {
			return &cmd, nil
		}
	}

	// ── 1. Normalize: replace escaped \n with real newlines ──
	normalized := strings.ReplaceAll(raw, "\\n", "\n")

	// ── 2. Extract JSON from backtick fences (```json ... ``` or ``` ... ```) ──
	var jsonStr string
	if idx := strings.Index(normalized, "```json"); idx >= 0 {
		rest := normalized[idx+7:]           // after ```json
		if end := strings.Index(rest, "```"); end >= 0 {
			jsonStr = strings.TrimSpace(rest[:end])
		}
	} else if idx := strings.Index(normalized, "```"); idx >= 0 {
		rest := normalized[idx+3:]           // after ```
		if end := strings.Index(rest, "```"); end >= 0 {
			jsonStr = strings.TrimSpace(rest[:end])
		}
	}

	// Try to parse the extracted JSON
	if jsonStr != "" {
		// Try direct parse first
		var cmd AgentCommand
		if err := json.Unmarshal([]byte(jsonStr), &cmd); err == nil {
			if cmd.Call != "" || cmd.Answer != "" {
				return &cmd, nil
			}
		}
		// If that fails, try brace extraction on fence content
		if cmd := tryParseBraceJSON(jsonStr); cmd != nil {
			return cmd, nil
		}
	}

	// ── 3. Direct JSON parse on the full normalized string ──
	var cmd2 AgentCommand
	if err := json.Unmarshal([]byte(normalized), &cmd2); err == nil {
		if cmd2.Call != "" || cmd2.Answer != "" {
			return &cmd2, nil
		}
	}

	// ── 4. Find any JSON object { ... } in the full normalized text ──
	if cmd := tryParseBraceJSON(normalized); cmd != nil {
		return cmd, nil
	}

	// ── 5. Fallback: treat entire raw response as plain text answer ──
	Logf("⚠ LLM agent response not valid JSON, treating as plain answer. Raw: %s", truncateText(raw, 200))
	return &AgentCommand{Answer: raw}, nil
}

// dispatchAgentCommand executes the command requested by the LLM agent.
// Returns the text response to send back to the user.
func dispatchAgentCommand(cmd *AgentCommand, funcs BotFuncs, lang string) string {
	if cmd.Call == "" {
		if cmd.Answer != "" {
			return cmd.Answer
		}
		// Neither answer nor call — nothing to dispatch
		return ""
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
		result, err := funcs.CreateGoal(cmd.Title, cmd.Content, cmd.Deadline, cmd.Priority, labels)
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
		result, err := funcs.UpdateGoal(cmd.GoalID, cmd.Title, cmd.Content, cmd.Status, cmd.Deadline, cmd.Priority, cmd.Progress, labels)
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

	case "delete_goal":
		if funcs.DeleteGoal == nil {
			return "Delete goal is not available."
		}
		if cmd.GoalID == "" {
			if lang == "ru" {
				return "❌ Не указан ID цели. Используй /goals чтобы увидеть список целей и их ID."
			}
			return "❌ No goal ID specified. Use /goals to see goals and their IDs."
		}
		if err := funcs.DeleteGoal(cmd.GoalID); err != nil {
			log.Printf("⚠ delete_goal error: %v", err)
			return fmt.Sprintf("❌ Failed to delete goal: %v", err)
		}
		if lang == "ru" {
			return fmt.Sprintf("✅ Цель удалена! ID: %s", cmd.GoalID)
		}
		return fmt.Sprintf("✅ Goal deleted! ID: %s", cmd.GoalID)

	case "search":
		if funcs.Search == nil {
			return "Search is not available."
		}
		limit := cmd.Limit
		if limit <= 0 {
			limit = 10
		}
		resultsJSON, err := funcs.Search(cmd.Query, limit)
		if err != nil {
			log.Printf("⚠ search error: %v", err)
			return fmt.Sprintf("❌ Search failed: %v", err)
		}
		var results []SearchResult
		if err := json.Unmarshal([]byte(resultsJSON), &results); err != nil {
			return resultsJSON
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
		goalsJSON, err := funcs.ListGoals(status, labels)
		if err != nil {
			log.Printf("⚠ list_goals error: %v", err)
			return fmt.Sprintf("❌ Failed to list goals: %v", err)
		}
		var goals []Goal
		if err := json.Unmarshal([]byte(goalsJSON), &goals); err != nil {
			return goalsJSON
		}
		return formatGoalsList(goals, lang)

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
		goalJSON, err := funcs.GetGoal(cmd.GoalID)
		if err != nil {
			return fmt.Sprintf("❌ %v", err)
		}
		var g Goal
		if err := json.Unmarshal([]byte(goalJSON), &g); err != nil {
			return goalJSON
		}
		return formatGoalDetail(g, lang)

	case "get_memory":
		if funcs.GetMemory == nil {
			return "Get memory is not available."
		}
		if cmd.Key == "" {
			if lang == "ru" {
				return "❌ Не указан ключ памяти."
			}
			return "❌ No memory key specified."
		}
		memoryJSON, err := funcs.GetMemory(cmd.Key)
		if err != nil {
			return fmt.Sprintf("❌ %v", err)
		}
		// Parse and format nicely
		var mv MemoryValue
		if err := json.Unmarshal([]byte(memoryJSON), &mv); err == nil {
			var reply string
			if lang == "ru" {
				reply = fmt.Sprintf("📖 <b>Память:</b> <code>%s</code>\n\n", cmd.Key)
			} else {
				reply = fmt.Sprintf("📖 <b>Memory:</b> <code>%s</code>\n\n", cmd.Key)
			}
			if mv.Summary != "" {
				reply += fmt.Sprintf("📝 <b>%s</b>\n\n", escapeHTML(mv.Summary))
			}
			if mv.Content != "" {
				reply += fmt.Sprintf("%s\n\n", escapeHTML(mv.Content))
			}
			if len(mv.Tags) > 0 {
				reply += fmt.Sprintf("🏷 %s\n", formatLabels(mv.Tags))
			}
			if mv.Source != "" {
				if lang == "ru" {
					reply += fmt.Sprintf("📡 Источник: %s", mv.Source)
				} else {
					reply += fmt.Sprintf("📡 Source: %s", mv.Source)
				}
			}
			return reply
		}
		// Fallback: return raw JSON
		if lang == "ru" {
			return fmt.Sprintf("📖 Содержимое памяти (ключ: %s):\n%s", cmd.Key, memoryJSON)
		}
		return fmt.Sprintf("📖 Memory content (key: %s):\n%s", cmd.Key, memoryJSON)

	case "get_timeline":
		if funcs.GetTimeline == nil {
			return "Get timeline is not available."
		}
		limit := cmd.Limit
		if limit <= 0 {
			limit = 10
		}
		timelineJSON, err := funcs.GetTimeline(cmd.From, cmd.To, limit)
		if err != nil {
			log.Printf("⚠ get_timeline error: %v", err)
			return fmt.Sprintf("❌ Failed to get timeline: %v", err)
		}
		var entries []TimelineEntry
		if err := json.Unmarshal([]byte(timelineJSON), &entries); err != nil {
			return timelineJSON
		}
		return formatTimelineResults(entries, lang)

	case "get_context":
		if funcs.GetContext == nil {
			return "Get context is not available."
		}
		limit := cmd.Limit
		if limit <= 0 {
			limit = 5
		}
		ctxJSON, err := funcs.GetContext(cmd.Query, limit)
		if err != nil {
			log.Printf("⚠ get_context error: %v", err)
			return fmt.Sprintf("❌ Failed to get context: %v", err)
		}
		var ctx ContextResult
		if err := json.Unmarshal([]byte(ctxJSON), &ctx); err != nil {
			return ctxJSON
		}
		return formatContextResult(ctx, lang)

	case "suggest":
		if funcs.Suggest == nil {
			return "Suggest is not available."
		}
		limit := cmd.Limit
		if limit <= 0 {
			limit = 5
		}
		suggestJSON, err := funcs.Suggest(cmd.Query, limit, lang)
		if err != nil {
			log.Printf("⚠ suggest error: %v", err)
			return fmt.Sprintf("❌ Failed to get suggestions: %v", err)
		}
		var suggestions []Suggestion
		if err := json.Unmarshal([]byte(suggestJSON), &suggestions); err != nil {
			return suggestJSON
		}
		return formatSuggestions(suggestions, lang)

	default:
		if lang == "ru" {
			return fmt.Sprintf("❌ Неизвестная команда: %s", cmd.Call)
		}
		return fmt.Sprintf("❌ Unknown command: %s", cmd.Call)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseLabels parses a labels JSON raw message into a slice.
// Accepts both JSON array (["a","b"]) and JSON-string-of-array ("[\"a\",\"b\"]").
func parseLabels(labelsRaw json.RawMessage) []string {
	if len(labelsRaw) == 0 {
		return nil
	}
	// Try direct array parse first: ["a","b"]
	var labels []string
	if err := json.Unmarshal(labelsRaw, &labels); err == nil {
		return labels
	}
	// Try quoted string: "[\"a\",\"b\"]"
	var str string
	if err := json.Unmarshal(labelsRaw, &str); err == nil {
		if err := json.Unmarshal([]byte(str), &labels); err == nil {
			return labels
		}
		// Try comma-separated
		parts := strings.Split(str, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				labels = append(labels, p)
			}
		}
		return labels
	}
	// Last resort: comma-separated from raw string
	parts := strings.Split(strings.Trim(string(labelsRaw), `"[] `), ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			labels = append(labels, p)
		}
	}
	return labels
}