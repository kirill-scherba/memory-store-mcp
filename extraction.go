// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"regexp"
	"fmt"
	"log"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Extracted fact
// ---------------------------------------------------------------------------

// ExtractedFact is a single fact extracted from conversation text.
type ExtractedFact struct {
	Content string   `json:"content"`
	Summary string   `json:"summary"`
	Tags    []string `json:"tags,omitempty"`
}

// GraphTriple is a relation between two named entities, extracted from the same
// text and in the same LLM call as the facts.
type GraphTriple struct {
	From     string `json:"from"`
	Relation string `json:"relation"`
	To       string `json:"to"`
	Date     string `json:"date,omitempty"`
}

// ExtractResult is the outcome of one extraction.
type ExtractResult struct {
	Facts   []ExtractedFact `json:"facts"`
	Triples []GraphTriple   `json:"triples"`
}

// extractSystemPrompt returns the system prompt for the extraction LLM call.
func extractSystemPrompt() string {
	return `You are a fact extraction system. Given a conversation text, extract important facts, decisions, intentions, key information — and the relations between the entities named in the text.

Return a JSON object with exactly two keys:

1. "facts" — an array of fact objects:
   - content: the full original text of the fact
   - summary: a one-line summary (max 100 chars)
   - tags: 2-5 relevant tags

2. "triples" — an array of relations between two entities that are BOTH named in the text:
   - from: source entity (person, place, project, image, dish, idea, ...)
   - relation: a short lowercase Russian verb with underscores
     (был_в, заказал, сгенерировал, породил_идею, рассказал_о, работает_над, написал, ...)
   - to: target entity
   - date: YYYY-MM-DD when the text states one, otherwise ""

Only emit a triple when both entities really appear in the text. Never invent entities. If there are no relations, return an empty array.

Return ONLY the JSON object, nothing else. Example:
{"facts":[{"content":"Using Go 1.26 for the project","summary":"Tech stack: Go 1.26","tags":["go","tech-stack"]}],"triples":[{"from":"Кирилл","relation":"был_в","to":"Сварня","date":"2026-09-13"}]}`
}

// trailingComma matches a comma that directly precedes a closing bracket —
// the most common way small models produce invalid JSON.
var trailingComma = regexp.MustCompile(`,\s*([}\]])`)

// sanitizeLLMJSON cleans up what small models emit around valid JSON: Markdown
// code fences and trailing commas before a closing bracket. Both make the
// response unparseable, and both are cheap to fix.
func sanitizeLLMJSON(s string) string {
	s = strings.TrimSpace(s)

	// Markdown code fence: ```json ... ```
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}

	s = trailingComma.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

// ExtractFacts uses the LLM to extract structured facts from conversation text.
// Uses the synchronous chat client (ollamaClient, 120s timeout) and is kept
// for backward compatibility and synchronous callers.
func ExtractFacts(text string) (*ExtractResult, error) {
	return extractFactsWithGenerator(text, generateAnswer)
}

// ExtractFactsAsync extracts facts using the background extraction model and
// no-timeout client. Used by AsyncExtractor so that long-running extractions
// are not cut off by the 120s ollamaClient timeout.
func ExtractFactsAsync(text string) (*ExtractResult, error) {
	return extractFactsWithGenerator(text, generateExtractAnswer)
}

// extractFactsWithGenerator performs the extraction using the provided
// generator function.
func extractFactsWithGenerator(text string, generateFn func([]OllamaChatMessage) (string, error)) (*ExtractResult, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	msg := []OllamaChatMessage{
		{Role: "system", Content: extractSystemPrompt()},
		{Role: "user", Content: text},
	}

	answer, err := generateFn(msg)
	if err != nil {
		return nil, fmt.Errorf("LLM extract failed: %w", err)
	}

	// Parse the JSON response. The current format is an object with "facts" and
	// "triples"; a bare array of facts (the previous format) is still accepted.
	answer = sanitizeLLMJSON(answer)
	cleaned := answer
	if idx := strings.Index(answer, "{"); idx >= 0 {
		if end := strings.LastIndex(answer, "}"); end > idx {
			cleaned = answer[idx : end+1]
		}
	} else if idx := strings.Index(answer, "["); idx >= 0 {
		if end := strings.LastIndex(answer, "]"); end > idx {
			cleaned = answer[idx : end+1]
		}
	}

	var res ExtractResult
	if err := json.Unmarshal([]byte(cleaned), &res); err == nil &&
		(res.Facts != nil || res.Triples != nil) {
		return &res, nil
	}

	var facts []ExtractedFact
	if err := json.Unmarshal([]byte(cleaned), &facts); err == nil {
		return &ExtractResult{Facts: facts}, nil
	}

	return nil, fmt.Errorf("parse extraction JSON: %s", answer)
}

// ---------------------------------------------------------------------------
// Suggest prompt builder
// ---------------------------------------------------------------------------

// suggestSystemPrompt returns the system prompt for the suggestion LLM call.
// The lang parameter controls the output language ("ru" for Russian, "en" for English).
func suggestSystemPrompt(lang string) string {
	var additional string
	if lang == "ru" {
		additional = "\n\nВАЖНОЕ ПРАВИЛО: Все заголовки и описания должны быть на РУССКОМ языке."
	}

	return `You are a proactive assistant that analyses context and goals to suggest next steps.
Return ONLY a JSON array of suggestion objects. Each suggestion has:
- type: one of "reminder", "followup", "goal_next_step", "insight"
- title: short title (max 60 chars)
- description: brief description (max 200 chars)
- priority: integer 0-10

Example:
[{"type":"goal_next_step","title":"Setup CI/CD pipeline","description":"You discussed setting up CI/CD for Cooksy. A good next step would be to define the deployment workflow.","priority":8}]`+additional
}

// SuggestPrompt builds a structured prompt for the suggest LLM call.
func SuggestPrompt(context string) string {
	// The context is already formatted, just ensure it's reasonable
	if len(context) > 4000 {
		context = context[:4000] + "..."
	}
	return context
}

// ---------------------------------------------------------------------------
// Auto-save from conversation
// ---------------------------------------------------------------------------

// ProcessConversation analyses a conversation exchange and queues extraction
// for automatic fact saving. It's called after each user message.
func (s *Storage) ProcessConversation(userMessage, assistantMessage string) error {
	combined := ""
	if userMessage != "" {
		combined += "User: " + userMessage + "\n"
	}
	if assistantMessage != "" {
		combined += "Assistant: " + assistantMessage + "\n"
	}

	if strings.TrimSpace(combined) == "" {
		return nil
	}

	jobID, err := s.SubmitExtract(combined, true)
	if err != nil {
		return fmt.Errorf("auto-extract submit failed: %w", err)
	}

	log.Printf("📝 submitted conversation for auto-extraction (job %s)", jobID)
	return nil
}

// ---------------------------------------------------------------------------
// Memory tool helper: get context for injection
// ---------------------------------------------------------------------------

// GetContextForInjection retrieves relevant context and formats it for
// injection into the system prompt.
func (s *Storage) GetContextForInjection(query string, limit int) (string, error) {
	ctx, err := s.GetContext(query, limit)
	if err != nil {
		return "", err
	}

	if len(ctx.Memories) == 0 && len(ctx.Goals) == 0 {
		return "", nil
	}

	var parts []string

	if len(ctx.Memories) > 0 {
		parts = append(parts, "=== Relevant memories ===")
		for i, mem := range ctx.Memories {
			date := mem.CreatedAt
			if len(date) > 10 {
				date = date[:10]
			}
			parts = append(parts, fmt.Sprintf("%d. [%s] %s (score: %.2f)",
				i+1, date, mem.Value.Content, mem.Score))
			if mem.Value.Summary != "" {
				parts = append(parts, fmt.Sprintf("   Summary: %s", mem.Value.Summary))
			}
			if len(mem.Value.Tags) > 0 {
				parts = append(parts, fmt.Sprintf("   Tags: %s", strings.Join(mem.Value.Tags, ", ")))
			}
			if mem.Value.GoalID != "" {
				parts = append(parts, fmt.Sprintf("   Goal: %s", mem.Value.GoalID))
			}
		}
	}

	// Graph context: entities mentioned in the query and in the retrieved
	// memories, with their immediate connections. Injected automatically — the
	// graph is useless if the agent has to remember to query it.
	var graphText strings.Builder
	graphText.WriteString(query)
	for _, mem := range ctx.Memories {
		graphText.WriteString(" ")
		graphText.WriteString(mem.Value.Content)
		graphText.WriteString(" ")
		graphText.WriteString(mem.Value.Summary)
		graphText.WriteString(" ")
		graphText.WriteString(strings.Join(mem.Value.Tags, " "))
	}
	if items, err := s.graphContextForText(graphText.String(), graphContextEntities); err == nil {
		if section := formatGraphContext(items); section != "" {
			parts = append(parts, "\n"+section)
		}
	}

	if len(ctx.Goals) > 0 {
		parts = append(parts, "\n=== Active goals ===")
		for i, g := range ctx.Goals {
			parts = append(parts, fmt.Sprintf("%d. %s [%d%%] priority=%d deadline=%s",
				i+1, g.Title, g.Progress, g.Priority, g.Deadline))
			if g.Description != "" {
				parts = append(parts, fmt.Sprintf("   %s", g.Description))
			}
		}
	}

	// Upcoming reminders: automatically list all memory/reminder/* entries
	// with a date >= yesterday, so they always appear in context.
	// Uses recursive folder traversal (memory/reminder/YYYY-MM-DD/key).
	const reminderPrefix = "memory/reminder/"
	reminders := s.listReminders(reminderPrefix)
	if len(reminders) > 0 {
		parts = append(parts, "\n=== Upcoming reminders ===")
		parts = append(parts, reminders...)
	}

	return time.Now().UTC().Format("2006-01-02 15:04:05") + "\n" + strings.Join(parts, "\n"), nil
}

// listReminders recursively lists reminder keys under the given prefix,
// filters out entries with dates older than yesterday, and returns formatted strings.
func (s *Storage) listReminders(prefix string) []string {
	keys, err := s.List(prefix)
	if err != nil || len(keys) == 0 {
		return nil
	}

	now := time.Now()
	var reminders []string

	for _, key := range keys {
		if strings.HasSuffix(key, "/") {
			// Folder — recurse into it
			reminders = append(reminders, s.listReminders(key)...)
			continue
		}

		mem, err := s.Get(key)
		if err != nil || mem == nil {
			continue
		}

		// Filter by date from key: memory/reminder/YYYY-MM-DD/name
		include := true
		parts := strings.Split(key, "/")
		if len(parts) >= 4 {
			if d, err := time.Parse("2006-01-02", parts[2]); err == nil {
				if d.Before(now.Add(-24 * time.Hour)) {
					include = false
				}
			}
		}

		if !include {
			continue
		}

		reminders = append(reminders, fmt.Sprintf("- [%s] %s", key, mem.Content))
	}

	return reminders
}
