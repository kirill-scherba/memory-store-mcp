// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"strings"
	"unicode"
)

// ClassificationResult stores the result of message classification.
type ClassificationResult struct {
	Type        string   // "note", "goal", "question", "command"
	Title       string   // extracted title
	Description string   // full description / content
	Labels      []string // extracted labels
	Priority    int      // 0-10 (for goals)
	Command     string   // mapped bot command for "command" type (e.g. "goals", "suggest")
}

// classifyMessage classifies a text message using heuristics.
// No LLM call — purely rule-based for speed and reliability.
func classifyMessage(text string) *ClassificationResult {
	text = strings.TrimSpace(text)

	// Default values
	labels := extractLabels(text)
	title, description := extractTitle(text)
	priority := 5

	// 1. Check for question patterns
	if isQuestion(text) {
		return &ClassificationResult{
			Type:        "question",
			Title:       title,
			Description: text,
			Labels:      labels,
			Priority:    priority,
		}
	}

	// 2. Check for natural-language command requests
	if cmd := detectCommand(text); cmd != "" {
		return &ClassificationResult{
			Type:        "command",
			Title:       title,
			Description: description,
			Labels:      labels,
			Priority:    priority,
			Command:     cmd,
		}
	}

	// 3. Check for goal patterns
	if isGoal(text) {
		return &ClassificationResult{
			Type:        "goal",
			Title:       title,
			Description: description,
			Labels:      labels,
			Priority:    guessPriority(text),
		}
	}

	// 4. Default to note
	return &ClassificationResult{
		Type:        "note",
		Title:       title,
		Description: description,
		Labels:      labels,
		Priority:    priority,
	}
}

// detectCommand checks if the text is a natural-language command that maps to
// a known bot command. Returns the command name or "" if not a command.
func detectCommand(text string) string {
	lower := strings.ToLower(text)

	// Command patterns: list of (keywords, command)
	patterns := []struct {
		keywords []string
		command  string
	}{
		{[]string{"покажи цели", "мои цели", "список целей", "какие цели", "активные цели", "все цели",
			"show goals", "my goals", "list goals", "active goals", "what goals"}, "goals"},
		{[]string{"что делать", "чем заняться", "что дальше", "подскажи", "посоветуй", "следующие шаги",
			"what to do", "next steps", "suggest", "what next", "what now"}, "suggest"},
		{[]string{"покажи контекст", "какой контекст", "текущий контекст", "контекст",
			"show context", "what context", "current context"}, "context"},
		{[]string{"последние события", "что было", "лента", "недавнее", "история", "покажи события",
			"recent events", "what happened", "timeline", "recent activity", "history"}, "timeline"},
		{[]string{"сводка за день", "сводка за неделю", "сводка за месяц", "итоги дня", "дайджест",
			"daily summary", "weekly summary", "monthly summary", "digest", "summary"}, "digest"},
		{[]string{"найди", "поищи", "поиск", "напомни про", "что я знаю о",
			"search for", "find", "remind me about", "what do I know about"}, "search"},
	}

	for _, p := range patterns {
		for _, kw := range p.keywords {
			if strings.HasPrefix(lower, kw) || strings.Contains(lower, " "+kw) || strings.Contains(lower, ", "+kw) {
				return p.command
			}
		}
	}

	return ""
}

// isQuestion checks if the text looks like a question.
func isQuestion(text string) bool {
	// Starts with question words
	prefixes := []string{
		"что", "как", "где", "когда", "почему", "зачем", "кто", "чей", "сколько",
		"какой", "какая", "какие", "какое",
		"what", "how", "where", "when", "why", "who", "whose", "which", "whom",
		"расскажи", "напомни", "найди", "поищи",
		"tell me", "show me", "find", "search", "remind",
	}
	lower := strings.ToLower(text)
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	// Ends with "?"
	if strings.HasSuffix(strings.TrimSpace(text), "?") {
		return true
	}
	return false
}

// isGoal checks if the text looks like a goal / intention.
func isGoal(text string) bool {
	lower := strings.ToLower(text)

	goalPrefixes := []string{
		"хочу", "нужно", "надо", "необходимо", "планирую", "собираюсь",
		"задача", "цель", "todo", "to-do",
		"i want", "i need", "i will", "goal", "objective", "task",
		"сделать", "создать", "написать", "реализовать", "добавить",
		"make", "create", "implement", "add", "fix", "build",
	}
	for _, p := range goalPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}

	// Contains "цель:" or "goal:" marker
	if strings.Contains(lower, "цель:") || strings.Contains(lower, "goal:") {
		return true
	}

	return false
}

// extractTitle extracts a title from the text.
// Returns the first line or first N chars as title.
func extractTitle(text string) (title, description string) {
	// Split by newline
	lines := strings.SplitN(text, "\n", 2)
	if len(lines) > 1 {
		title = strings.TrimSpace(lines[0])
		description = strings.TrimSpace(lines[1])
	} else {
		// Single line: first 80 chars as title
		title = text
		if len([]rune(title)) > 80 {
			title = string([]rune(title)[:80])
		}
		description = text
	}
	if title == "" {
		title = description
	}
	return title, description
}

// extractLabels extracts hashtags and labels from the text.
func extractLabels(text string) []string {
	var labels []string
	seen := make(map[string]bool)

	words := strings.Fields(text)
	for _, word := range words {
		// #hashtag pattern
		if strings.HasPrefix(word, "#") && len(word) > 1 {
			label := strings.TrimFunc(word[1:], func(r rune) bool {
				return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
			})
			label = strings.ToLower(label)
			if label != "" && !seen[label] {
				seen[label] = true
				labels = append(labels, label)
			}
		}
	}

	return labels
}

// guessPriority tries to infer priority from text.
func guessPriority(text string) int {
	lower := strings.ToLower(text)

	highPriorityWords := []string{
		"срочно", "важно", "критично", "немедленно", "asap", "urgent", "important",
		"high priority", "critical", "блокер",
	}
	lowPriorityWords := []string{
		"не срочно", "неважно", "low priority", "когда-нибудь", "потом",
		"некритично", "необязательно",
	}

	for _, w := range highPriorityWords {
		if strings.Contains(lower, w) {
			return 9
		}
	}
	for _, w := range lowPriorityWords {
		if strings.Contains(lower, w) {
			return 2
		}
	}
	return 5 // default
}